package main

import (
	"log"
	"net/http"
	"os"

	"github.com/davidsilvasanmartin/playlists-go/internal/api"
	"github.com/davidsilvasanmartin/playlists-go/internal/musicbrainz"
	"github.com/davidsilvasanmartin/playlists-go/internal/search"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// version is set at build time via -ldflags="-X main.version=<value>".
// It defaults to "dev" for local builds that omit the flag.
var version = "dev"

func main() {
	// -- config ----------------
	// Load committed defaults first, then let .env override personal values
	// Both files are optional - errors are silently discarded
	// In Docker neither file exists; env vars are injected by the container runtime
	_ = godotenv.Load(".development.env")
	_ = godotenv.Overload(".env")

	// TODO we are setting defaults here which we may forget about; maybe it's better to just use Viper
	port := getEnv("PLAYLISTS_PORT", "8080")
	mbBaseURL := getEnv("PLAYLISTS_MB_BASE_URL", "https://musicbrainz.org")
	mbUserAgent := mustGetEnv("PLAYLISTS_MB_USER_AGENT")
	logLevel := getEnv("PLAYLISTS_LOG_LEVEL", "info")
	logFormat := getEnv("PLAYLISTS_LOG_FORMAT", "json")

	// -- logger ----------------
	logger, err := buildLogger(logLevel, logFormat)
	if err != nil {
		log.Fatalf("failed to build logger: %v", err)
	}
	// Flush any buffered log entries before the process exits.
	// This is important in production to avoid losing the last few log lines
	defer logger.Sync()
	logger.Info("starting server",
		zap.String("port", port),
		zap.String("logLevel", logLevel),
		zap.String("logFormat", logFormat),
		zap.String("version", version),
	)

	// -- dependencies ----------------
	mbClient := musicbrainz.NewClient(mbBaseURL, mbUserAgent, logger)
	searchService := search.NewService(mbClient, logger)
	searchHandler := search.NewHandler(searchService, logger)

	// -- routing ----------------
	mux := api.NewRouter(logger, searchHandler, version)

	// -- server ----------------
	addr := ":" + port
	logger.Info("server ready", zap.String("addr", addr))
	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Fatal("server error", zap.Error(err))
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// mustGetEnv returns the value of an environment variable. If the variable is
// not set, or if it is set but empty, it will cause the program to crash
func mustGetEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required environment variable %q is not set", key)
	}
	return v
}

// buildLogger creates a *zap.Logger configured from environment variables
//
//	PLAYLISTS_LOG_LEVEL - one of: debug, info, warn, error (default: info)
//	PLAYLISTS_LOG_FORMAT - one of: dev, json (default: json)
//
// "dev" format uses a coloured, human-readable encoder that prints to stdout.
// "json" format uses a compact JSON encoder suited for log-aggregation pipelines.
func buildLogger(level string, format string) (*zap.Logger, error) {
	// zapcore.Level is an integer type that represents log severity.
	// UnmarshalText parses strings like "debug", "info", "warn", "error".
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	// zap.NewAtomicLevelAt wraps the level in a struct that can be changed at
	// runtime (useful for dynamic log-level endpoints — not needed yet, but
	// it is the idiomatic way to set a level in Zap configs).
	atomicLevel := zap.NewAtomicLevelAt(zapLevel)

	var cfg zap.Config
	if format == "dev" {
		// NewDevelopmentConfig returns a Config that uses the console encoder
		// (coloured, human-readable). Stack traces are enabled on Warn+.
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		// NewProductionConfig returns a Config that uses the JSON encoder and
		// writes to stdout. This is the right choice for container environments.
		cfg = zap.NewProductionConfig()
	}
	cfg.Level = atomicLevel

	// Build() compiles the Config into a *zap.Logger. The only realistic error
	// here is an invalid output path, so we treat it as fatal in main().
	return cfg.Build()
}
