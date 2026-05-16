// Package logging builds the shared zap logger for claude-mesh binaries.
// Log path defaults to ~/Library/Logs/claude-mesh-bridge.log.
// Level is parsed from cfg.LogLevel; unrecognised values default to INFO.
package logging

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/marioser/claude-mesh/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New creates a zap.Logger that writes to the file specified in cfg.LogPath.
// It also writes to stderr so launchd captures output.
func New(cfg config.EnvOptions) (*zap.Logger, error) {
	level := parseLevel(cfg.LogLevel)

	logPath, err := expandHome(cfg.LogPath)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, err
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "time"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	fileEncoder := zapcore.NewJSONEncoder(encoderCfg)
	consoleEncoder := zapcore.NewConsoleEncoder(encoderCfg)

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	fileSink := zapcore.AddSync(file)
	stderrSink := zapcore.AddSync(os.Stderr)

	core := zapcore.NewTee(
		zapcore.NewCore(fileEncoder, fileSink, level),
		zapcore.NewCore(consoleEncoder, stderrSink, level),
	)

	return zap.New(core, zap.AddCaller()), nil
}

// parseLevel converts a string log level to a zapcore.Level.
// Unknown values default to INFO.
func parseLevel(s string) zapcore.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return zapcore.DebugLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// expandHome replaces a leading "~" with the user's home directory.
func expandHome(path string) (string, error) {
	if !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[2:]), nil
}
