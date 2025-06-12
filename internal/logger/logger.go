package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sefaphlvn/clustereye-test/internal/config"
	"gopkg.in/natefinch/lumberjack.v2"
)

var Logger zerolog.Logger

// InitLogger initializes the global logger with the given configuration
func InitLogger(cfg config.LogConfig) error {
	// Set log level
	level, err := zerolog.ParseLevel(strings.ToLower(cfg.Level))
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	var writers []io.Writer

	// Console output
	if cfg.Output == "console" || cfg.Output == "both" {
		var consoleWriter io.Writer
		if cfg.Format == "json" {
			consoleWriter = os.Stdout
		} else {
			// Pretty console output
			consoleWriter = zerolog.ConsoleWriter{
				Out:        os.Stdout,
				TimeFormat: time.RFC3339,
				FormatLevel: func(i interface{}) string {
					return strings.ToUpper(fmt.Sprintf("| %-6s|", i))
				},
				FormatMessage: func(i interface{}) string {
					return fmt.Sprintf("***%s****", i)
				},
				FormatFieldName: func(i interface{}) string {
					return fmt.Sprintf("%s:", i)
				},
				FormatFieldValue: func(i interface{}) string {
					return strings.ToUpper(fmt.Sprintf("%s", i))
				},
			}
		}
		writers = append(writers, consoleWriter)
	}

	// File output
	if cfg.Output == "file" || cfg.Output == "both" {
		// Create log directory if it doesn't exist
		logDir := filepath.Dir(cfg.FilePath)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return fmt.Errorf("log dizini oluşturulamadı: %w", err)
		}

		// Configure log rotation
		fileWriter := &lumberjack.Logger{
			Filename:   cfg.FilePath,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
		}
		writers = append(writers, fileWriter)
	}

	if len(writers) == 0 {
		writers = append(writers, os.Stdout)
	}

	// Create multi-writer
	var writer io.Writer
	if len(writers) == 1 {
		writer = writers[0]
	} else {
		writer = io.MultiWriter(writers...)
	}

	// Initialize logger
	Logger = zerolog.New(writer).
		With().
		Timestamp().
		Caller().
		Logger()

	// Set global logger
	log.Logger = Logger

	Logger.Info().
		Str("level", cfg.Level).
		Str("format", cfg.Format).
		Str("output", cfg.Output).
		Str("file_path", cfg.FilePath).
		Msg("Logger initialized")

	return nil
}

// Debug logs a debug message
func Debug() *zerolog.Event {
	return Logger.Debug()
}

// Info logs an info message
func Info() *zerolog.Event {
	return Logger.Info()
}

// Warn logs a warning message
func Warn() *zerolog.Event {
	return Logger.Warn()
}

// Error logs an error message
func Error() *zerolog.Event {
	return Logger.Error()
}

// Fatal logs a fatal message and exits
func Fatal() *zerolog.Event {
	return Logger.Fatal()
}

// Panic logs a panic message and panics
func Panic() *zerolog.Event {
	return Logger.Panic()
}

// WithAgent creates a logger with agent context
func WithAgent(agentID string) zerolog.Logger {
	return Logger.With().Str("agent_id", agentID).Logger()
}

// WithDB creates a logger with database context
func WithDB(operation string) zerolog.Logger {
	return Logger.With().Str("db_operation", operation).Logger()
}

// WithAPI creates a logger with API context
func WithAPI(endpoint string) zerolog.Logger {
	return Logger.With().Str("api_endpoint", endpoint).Logger()
}

// WithMetric creates a logger with metric context
func WithMetric(metricName string) zerolog.Logger {
	return Logger.With().Str("metric_name", metricName).Logger()
}
