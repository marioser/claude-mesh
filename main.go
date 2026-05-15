package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"claude-mesh/internal/anthropicapi"
	"claude-mesh/internal/bridge"
	"claude-mesh/internal/config"
	"claude-mesh/internal/contextusage"
	"claude-mesh/internal/installer"
	"claude-mesh/internal/lock"
	"claude-mesh/internal/logging"
	mqttclient "claude-mesh/internal/mqtt"
	"claude-mesh/internal/publisher"
	"claude-mesh/internal/statusline"
	"claude-mesh/internal/store"
	"claude-mesh/internal/usagestats"

	"github.com/redis/go-redis/v9"
)

// anthropicAPICfg holds optional Anthropic API credentials for the usage primary source.
// When orgID and cookie are both non-empty, the daemon uses the API instead of JSONL parsing.
// baseURL is used in tests to override the real endpoint; in production it should be "".
type anthropicAPICfg struct {
	orgID   string
	cookie  string
	baseURL string // empty → uses real claude.ai endpoint
}

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
	case "statusline":
		return runStatusline(remaining[1:])
	case "help", "--help", "-h":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command: %s", remaining[0])
	}
}

// buildSubscriberClientID returns a unique MQTT client_id for the bridge subscriber.
// Format: <base>-sub-<hostname>-<pid>
// Using hostname+pid prevents "session taken over" disconnects when 2+ bridge
// instances run simultaneously (e.g. launchd KeepAlive + manual invocation).
func buildSubscriberClientID(base string) string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	return fmt.Sprintf("%s-sub-%s-%d", base, hostname, os.Getpid())
}

func runBridge(_ []string) error {
	// --- Instance guard (FIX 2) ---
	// Prevent multiple bridge daemons from competing over the same MQTT
	// subscriber client_id and Redis session state. If another live bridge
	// process is detected, exit 0 silently — launchd KeepAlive will not
	// re-trigger an immediate restart.
	lockPath := envOrDefault("CLAUDE_MESH_LOCK_PATH", "/tmp/claude-mesh-bridge.pid")
	if err := lock.Acquire(lockPath); err != nil {
		if errors.Is(err, lock.ErrAlreadyHeld) {
			// Another daemon is alive — exit cleanly without printing anything
			// to avoid polluting logs with spurious restarts.
			return nil
		}
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer func() { _ = lock.Release(lockPath) }()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	log, err := logging.New(cfg)
	if err != nil {
		return fmt.Errorf("logger: %w", err)
	}
	defer func() { _ = log.Sync() }()

	// --- Unique subscriber client_id (FIX 1) ---
	clientID := buildSubscriberClientID(cfg.MQTTClientID)
	broker := fmt.Sprintf("tcp://%s:%d", cfg.MQTTHost, cfg.MQTTPort)

	// --- Verbose startup logging (FIX 3) ---
	log.Info("mqtt connecting",
		zap.String("broker", broker),
		zap.String("client_id", clientID),
	)

	// Bug 2 fix: create the Subscriber first so the OnConnectHandler closure can
	// capture it and re-issue Subscribe on every paho reconnect. The handler runs on
	// paho's internal network goroutine — it MUST NOT block, so it spawns a goroutine.
	var sub *mqttclient.Subscriber // forward declaration; populated after mqttClient is created

	onConnect := func() {
		log.Info("mqtt reconnected, resubscribing", zap.String("client_id", clientID))
		// IMPORTANT: This callback runs on paho's internal network goroutine.
		// We MUST NOT block here — spawn a goroutine for anything that can block.
		go func() {
			if sub == nil {
				return // bridge not yet constructed (extremely unlikely race)
			}
			sub.SetConnected(true)
			ctx := context.Background()
			if err := sub.ResubscribeOnReconnect(ctx); err != nil {
				log.Warn("mqtt resubscribe after reconnect failed", zap.Error(err))
			} else {
				log.Info("mqtt resubscribed successfully after reconnect", zap.String("client_id", clientID))
			}
		}()
	}

	mqttClient := mqttclient.NewPahoClient(broker, clientID, cfg.MQTTUsername, cfg.MQTTPassword, onConnect)

	if err := mqttClient.Connect(context.Background()); err != nil {
		log.Warn("mqtt connect failed (will retry via AutoReconnect)", zap.Error(err))
	} else {
		log.Info("mqtt connected", zap.String("client_id", clientID))
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

	// Bug 3 fix: touch all existing active sessions so the sweep ticker does not
	// evict them in the 90s window following a restart.
	nowMs := float64(time.Now().UnixMilli())
	if n, err := s.TouchActiveSessions(context.Background(), nowMs); err != nil {
		log.Warn("bridge: session rehydration failed", zap.Error(err))
	} else if n > 0 {
		log.Info("bridge: rehydrated active sessions", zap.Int("count", n))
	}

	sub = mqttclient.NewSubscriber(mqttClient)
	b := bridge.New(sub, s, log)

	log.Info("bridge ready", zap.String("client_id", clientID), zap.String("lock", lockPath))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start the usage tracking poll goroutine.
	// It scans ~/.claude/projects/ every 60s and writes stats to Redis.
	// If CLAUDE_MESH_ANTHROPIC_ORG_ID + CLAUDE_MESH_ANTHROPIC_COOKIE are set,
	// the Anthropic usage API is used as primary source instead of JSONL parsing.
	projectsDir := claudeProjectsDir()
	plan := usagestats.ResolveFromEnv()
	apiCfg := anthropicAPICfg{
		orgID:  cfg.AnthropicOrgID,
		cookie: cfg.AnthropicCookie,
	}
	go runUsagePoll(ctx, redisClient, projectsDir, plan, log, apiCfg)

	b.Run(ctx)
	return nil
}

// claudeProjectsDir returns the path to ~/.claude/projects/.
func claudeProjectsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

// runUsagePoll runs the usage tracking loop. It polls every 60s, scans JSONL transcripts
// (or uses the Anthropic API if configured), and writes stats to Redis with a 120s TTL.
// Errors are logged but never crash the daemon.
func runUsagePoll(ctx context.Context, client *redis.Client, projectsDir string, plan usagestats.Plan, log *zap.Logger, apiCfg anthropicAPICfg) {
	pollUsageOnce(ctx, client, projectsDir, plan, log, apiCfg)

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pollUsageOnce(ctx, client, projectsDir, plan, log, apiCfg)
		}
	}
}

// buildHTTPDoer returns the appropriate HTTPDoer for the given baseURL.
//
// When baseURL is non-empty (test paths pointing at a local httptest server),
// a plain *http.Client is returned — no TLS impersonation needed since the test
// server uses plain HTTP. When baseURL is empty (production, real claude.ai
// endpoint), a Python subprocess client using curl_cffi is returned to bypass
// Cloudflare's TLS fingerprint check. Returns nil on creation failure (caller
// falls back to JSONL).
func buildHTTPDoer(baseURL string, log *zap.Logger) anthropicapi.HTTPDoer {
	if baseURL != "" {
		// Test path: plain standard http.Client works against httptest servers.
		return &http.Client{Timeout: 10 * time.Second}
	}
	// Production path: Python subprocess + curl_cffi (Chrome120 impersonation).
	c, err := anthropicapi.NewPythonClient()
	if err != nil {
		log.Warn("usage poll: failed to create Python client (python3/curl_cffi missing?), falling back to JSONL", zap.Error(err))
		return nil
	}
	return c
}

// pollUsageOnce executes one usage scan+compute+write cycle.
//
// When apiCfg.orgID and apiCfg.cookie are both non-empty, it first attempts to
// fetch usage from the Anthropic API. On success the percentages come directly
// from the API's "utilization" fields (already 0-100) and the source key is
// written as "anthropic". On 401/any error it falls through to the JSONL path.
// When the API is not configured, or after any fallback, source is "local".
func pollUsageOnce(ctx context.Context, client *redis.Client, projectsDir string, plan usagestats.Plan, log *zap.Logger, apiCfg anthropicAPICfg) {
	const ttl = 120 * time.Second

	// --- Primary source: Anthropic API ---
	if apiCfg.orgID != "" && apiCfg.cookie != "" {
		httpClient := buildHTTPDoer(apiCfg.baseURL, log)
		if httpClient != nil {
			usage, err := anthropicapi.FetchUsage(ctx, httpClient, apiCfg.orgID, apiCfg.cookie, apiCfg.baseURL)
			if err == nil {
				// API succeeded: write utilization values directly.
				writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				defer cancel()

				pipe := client.Pipeline()
				pipe.Set(writeCtx, "claude:mesh:usage:pct:5h", fmt.Sprintf("%.4f", usage.FiveHour.Utilization), ttl)
				pipe.Set(writeCtx, "claude:mesh:usage:pct:week", fmt.Sprintf("%.4f", usage.SevenDay.Utilization), ttl)
				pipe.Set(writeCtx, "claude:mesh:usage:plan", plan.Tier, ttl)
				pipe.Set(writeCtx, "claude:mesh:usage:source", "anthropic", ttl)
				pipe.Set(writeCtx, "claude:mesh:usage:resets_at:5h", usage.FiveHour.ResetsAt.Format(time.RFC3339), ttl)
				pipe.Set(writeCtx, "claude:mesh:usage:resets_at:week", usage.SevenDay.ResetsAt.Format(time.RFC3339), ttl)

				if _, err := pipe.Exec(writeCtx); err != nil {
					log.Warn("usage poll: redis write (anthropic) failed", zap.Error(err))
					return
				}

				log.Debug("usage poll: anthropic api ok",
					zap.Float64("pct_5h", usage.FiveHour.Utilization),
					zap.Float64("pct_week", usage.SevenDay.Utilization),
				)
				return
			}

			// API failed — log and fall through to JSONL.
			if errors.Is(err, anthropicapi.ErrAuthFailed) {
				log.Warn("usage poll: anthropic api auth failed (cookie expired?), falling back to JSONL")
			} else {
				log.Warn("usage poll: anthropic api error, falling back to JSONL", zap.Error(err))
			}
		}
	}

	// --- Fallback source: local JSONL parsing ---
	since := time.Now().Add(-7 * 24 * time.Hour)
	entries, err := usagestats.ScanProjects(projectsDir, since)
	if err != nil {
		log.Warn("usage poll: scan failed", zap.Error(err))
		return
	}

	now := time.Now()
	tokens5h := usagestats.FiveHourTokens(entries, now)
	tokensWeek := usagestats.WeekTokens(entries, now)

	var pct5h, pctWeek float64
	if plan.Limit5h > 0 {
		pct5h = float64(tokens5h) / float64(plan.Limit5h) * 100.0
		if pct5h > 100.0 {
			pct5h = 100.0
		}
	}
	if plan.LimitWeek > 0 {
		pctWeek = float64(tokensWeek) / float64(plan.LimitWeek) * 100.0
		if pctWeek > 100.0 {
			pctWeek = 100.0
		}
	}

	writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	pipe := client.Pipeline()
	pipe.Set(writeCtx, "claude:mesh:usage:tokens:5h", tokens5h, ttl)
	pipe.Set(writeCtx, "claude:mesh:usage:tokens:week", tokensWeek, ttl)
	pipe.Set(writeCtx, "claude:mesh:usage:pct:5h", fmt.Sprintf("%.4f", pct5h), ttl)
	pipe.Set(writeCtx, "claude:mesh:usage:pct:week", fmt.Sprintf("%.4f", pctWeek), ttl)
	pipe.Set(writeCtx, "claude:mesh:usage:limit:5h", plan.Limit5h, ttl)
	pipe.Set(writeCtx, "claude:mesh:usage:limit:week", plan.LimitWeek, ttl)
	pipe.Set(writeCtx, "claude:mesh:usage:plan", plan.Tier, ttl)
	pipe.Set(writeCtx, "claude:mesh:usage:source", "local", ttl)

	if _, err := pipe.Exec(writeCtx); err != nil {
		log.Warn("usage poll: redis write failed", zap.Error(err))
		return
	}

	log.Debug("usage poll: ok (local)",
		zap.Int("tokens_5h", tokens5h),
		zap.Int("tokens_week", tokensWeek),
		zap.Float64("pct_5h", pct5h),
		zap.Float64("pct_week", pctWeek),
		zap.String("plan", plan.Tier),
	)
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
	client := mqttclient.NewPahoClient(broker, clientID, cfg.MQTTUsername, cfg.MQTTPassword, nil)

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

	// MQTT health (read from Redis hash written by the bridge each sweep tick).
	mqttHealth, found, err := s.ReadMQTTHealth(context.Background())
	if err != nil || !found {
		status["mqtt"] = map[string]any{"connected": false}
	} else {
		status["mqtt"] = map[string]any{
			"connected":    mqttHealth.Connected,
			"subscribed":   mqttHealth.Subscribed,
			"msg_count":    mqttHealth.MsgCount,
			"last_msg_ms":  mqttHealth.LastMsgMs,
			"started_at":   mqttHealth.StartedAtMs,
			"updated_at":   mqttHealth.UpdatedAtMs,
		}
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

// runStatusline prints a single-line status string suitable for Claude Code's statusLine config.
// Claude Code passes a JSON payload on stdin with session_id, transcript_path, cwd, model, etc.
//
// Performance budget: <100ms p95. Git calls, transcript parse, and Redis query all run
// concurrently. Any failure returns a minimal fallback line silently — no stderr
// output (Claude Code would display it to the user).
func runStatusline(_ []string) error {
	cfg, err := config.Load()
	if err != nil {
		// Config load failure is not fatal — print minimal line and exit 0.
		fmt.Println("🌳 - │ ⚪ daemon down")
		return nil
	}

	// Decode stdin JSON from Claude Code (non-fatal if stdin is empty or malformed).
	var in statusline.Input
	dec := json.NewDecoder(os.Stdin)
	_ = dec.Decode(&in) // ignore error — graceful degrade to zero Input

	// Run git + transcript parse concurrently (both CPU-bound or fast I/O).
	type gitResult struct {
		branch  string
		changes int
	}
	gitCh := make(chan gitResult, 1)
	usageCh := make(chan contextusage.Usage, 1)

	go func() {
		b, c := gitBranchAndChanges()
		gitCh <- gitResult{branch: b, changes: c}
	}()
	go func() {
		limit := statusline.ContextLimitForModel(in.Model)
		usageCh <- contextusage.Parse(in.TranscriptPath, limit)
	}()

	// Total budget 800ms — covers slow `git status` on large monorepos (~250ms)
	// plus the 50ms Redis sub-timeout enforced inside statusline.Render.
	// The Redis call itself remains capped at 50ms; this ceiling only protects
	// against runaway git/transcript reads.
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer func() { _ = redisClient.Close() }()

	gr := <-gitCh
	u := <-usageCh
	in.Changes = gr.changes

	s := store.NewRedisStore(redisClient, storeConfig(cfg))
	line := statusline.Render(ctx, s, gr.branch, in, u)
	fmt.Println(line)
	return nil
}

// gitBranchAndChanges returns the current git branch and the count of dirty files.
// Both operations use a 100ms timeout and fail silently to "" / 0.
// Callers should invoke this in a goroutine when concurrency is desired.
func gitBranchAndChanges() (branch string, changes int) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Branch.
	branchCmd := exec.CommandContext(ctx, "git", "branch", "--show-current")
	if out, err := branchCmd.Output(); err == nil {
		branch = strings.TrimSpace(string(out))
	}

	// Dirty-file count via `git status --porcelain`.
	statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	if out, err := statusCmd.Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.TrimSpace(line) != "" {
				changes++
			}
		}
	}

	return branch, changes
}

func usage() {
	fmt.Fprint(os.Stderr, `claude-mesh-bridge - MIOBOX Claude Mesh observability daemon

Usage:
  claude-mesh-bridge run                         Start the bridge daemon
  claude-mesh-bridge publish <event-type>        Publish a single event (reads JSON from stdin)
  claude-mesh-bridge install                     Install hooks, launchd agent, and MCP entry
  claude-mesh-bridge uninstall                   Remove all Claude Mesh installations
  claude-mesh-bridge status [--json]             Print daemon/Redis/MQTT health
  claude-mesh-bridge statusline                  Print one-line status for Claude Code statusLine
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
