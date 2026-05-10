package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"claude-mesh/internal/bridge"
	"claude-mesh/internal/config"
	"claude-mesh/internal/installer"
	"claude-mesh/internal/logging"
	mqttclient "claude-mesh/internal/mqtt"
	"claude-mesh/internal/publisher"
	"claude-mesh/internal/store"

	"github.com/redis/go-redis/v9"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "claude-mesh-bridge: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	// Global --ensure-running flag: check if daemon is running, kickstart if absent.
	fs := flag.NewFlagSet("claude-mesh-bridge", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	ensureRunning := fs.Bool("ensure-running", false, "kickstart daemon via launchctl if not running")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if *ensureRunning {
		return runEnsureRunning()
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		usage()
		return errors.New("missing command")
	}

	switch remaining[0] {
	case "run":
		return runBridge(remaining[1:])
	case "publish":
		if len(remaining) < 2 {
			return errors.New("publish requires an event type: session-open, activity, or session-close")
		}
		return runPublish(remaining[1], remaining[2:])
	case "install":
		return runInstall(remaining[1:])
	case "uninstall":
		return runUninstall(remaining[1:])
	case "status":
		return runStatus(remaining[1:])
	case "help", "--help", "-h":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command: %s", remaining[0])
	}
}

func runBridge(_ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	log, err := logging.New(cfg)
	if err != nil {
		return fmt.Errorf("logger: %w", err)
	}
	defer func() { _ = log.Sync() }()

	broker := fmt.Sprintf("tcp://%s:%d", cfg.MQTTHost, cfg.MQTTPort)
	mqttClient := mqttclient.NewPahoClient(broker, cfg.MQTTClientID+"-sub", cfg.MQTTUsername, cfg.MQTTPassword)

	if err := mqttClient.Connect(context.Background()); err != nil {
		log.Warn("mqtt connect failed (will retry via AutoReconnect)", zap.Error(err))
	}
	defer mqttClient.Disconnect(500)

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer func() { _ = redisClient.Close() }()

	s := store.NewRedisStore(redisClient, storeConfig(cfg))
	if err := s.HealthCheck(context.Background()); err != nil {
		return fmt.Errorf("redis unreachable: %w", err)
	}

	sub := mqttclient.NewSubscriber(mqttClient)
	b := bridge.New(sub, s, log)

	log.Info("bridge ready")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	b.Run(ctx)
	return nil
}

func runPublish(eventType string, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	broker := fmt.Sprintf("tcp://%s:%d", cfg.MQTTHost, cfg.MQTTPort)
	// Use pid-suffixed client ID to avoid conflict with the daemon.
	clientID := cfg.MQTTClientID + "-pub-" + strconv.Itoa(os.Getpid())
	client := mqttclient.NewPahoClient(broker, clientID, cfg.MQTTUsername, cfg.MQTTPassword)

	if err := client.Connect(ctx); err != nil {
		// Non-fatal: log and exit 0 (hooks must never block).
		fmt.Fprintf(os.Stderr, "claude-mesh-bridge publish: mqtt connect: %v\n", err)
		return nil
	}
	defer client.Disconnect(200)

	if err := publisher.PublishCmd(ctx, eventType, os.Stdin, client, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "claude-mesh-bridge publish: %v\n", err)
		// Return nil so hooks exit 0.
		return nil
	}
	return nil
}

func runInstall(_ []string) error {
	return installer.Install(installer.DefaultPaths())
}

func runUninstall(_ []string) error {
	return installer.Uninstall(installer.DefaultPaths())
}

func runStatus(_ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	status := map[string]any{
		"daemon": daemonRunning(),
	}

	// Redis health.
	redisClient := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer func() { _ = redisClient.Close() }()
	s := store.NewRedisStore(redisClient, storeConfig(cfg))
	if err := s.HealthCheck(context.Background()); err != nil {
		status["redis"] = "unreachable"
	} else {
		status["redis"] = "ok"
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(status)
}

func runEnsureRunning() error {
	if daemonRunning() {
		return nil
	}
	// Kickstart via launchctl (budget 200ms).
	uid := strconv.Itoa(os.Getuid())
	cmd := exec.Command("launchctl", "kickstart", "-k",
		"gui/"+uid+"/com.miobox.claude-mesh-bridge")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "claude-mesh-bridge --ensure-running: %v\n", err)
	}
	return nil
}

func daemonRunning() bool {
	cmd := exec.Command("pgrep", "-f", "claude-mesh-bridge run")
	return cmd.Run() == nil
}

func storeConfig(cfg config.EnvOptions) store.StoreConfig {
	return store.StoreConfig{
		SessionTTL:         cfg.SessionTTL,
		ActivityPerSessTTL: cfg.ActivityPerSessTTL,
		ActivityGlobalTTL:  cfg.ActivityGlobalTTL,
		ActivityRingSize:   cfg.ActivityRingSize,
		GlobalRingSize:     cfg.GlobalRingSize,
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `claude-mesh-bridge - MIOBOX Claude Mesh observability daemon

Usage:
  claude-mesh-bridge run                         Start the bridge daemon
  claude-mesh-bridge publish <event-type>        Publish a single event (reads JSON from stdin)
  claude-mesh-bridge install                     Install hooks, launchd agent, and MCP entry
  claude-mesh-bridge uninstall                   Remove all Claude Mesh installations
  claude-mesh-bridge status [--json]             Print daemon/Redis/MQTT health
  claude-mesh-bridge --ensure-running            Kickstart daemon if not running

Event types for publish:
  session-open    session-close    activity

Environment:
  CLAUDE_MESH_MQTT_HOST     MQTT broker host (default: localhost)
  CLAUDE_MESH_MQTT_PORT     MQTT broker port (default: 1883)
  CLAUDE_MESH_REDIS_ADDR    Redis address (default: localhost:6379)
  CLAUDE_MESH_LOG_LEVEL     Log level: debug|info|warn|error (default: info)
`)
}

// envOrDefault reads an env var with a fallback.
func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
