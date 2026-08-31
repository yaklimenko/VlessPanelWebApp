package main

import (
	"log"
	"os"
	"path/filepath"
	"time"
)

// Config holds application configuration
type Config struct {
	Port                  string
	AggregatorDir         string
	PanelsFilePath        string
	StaticDir             string
	VlessSubTestDaemonURL string
	DataDir               string
	PublicURL             string // публичный базовый URL агрегатора (для ссылок на подписки)
	SyncScript            string // rsync script to push configs to the aggregator
	AdminToken            string // master token; empty = auth disabled

	// HTTP-сервер: таймауты и время на graceful shutdown (см. main.go).
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration

	// MetricsDBPath — SQLite раздела статистики (по умолчанию /data/metrics.db).
	MetricsDBPath string
}

// LoadConfig loads configuration from environment variables with defaults
func LoadConfig() Config {
	port := os.Getenv("VLESSPANEL_PORT")
	if port == "" {
		port = "8080"
	}

	aggDir := os.Getenv("VLESSPANEL_AGGREGATOR_DIR")
	if aggDir == "" {
		aggDir = "/opt/vless-aggregator"
	}

	panelsFile := os.Getenv("VLESSPANEL_PANELS_FILE")
	if panelsFile == "" {
		panelsFile = "panels.json"
	}

	staticDir := os.Getenv("VLESSPANEL_STATIC_DIR")
	if staticDir == "" {
		staticDir = filepath.Join("..", "frontend", "dist")
	}

	daemonURL := os.Getenv("VLESSPANEL_VLESSSUBTEST_DAEMON_URL")
	if daemonURL == "" {
		daemonURL = "http://vlesssubtest:7070"
	}

	dataDir := os.Getenv("VLESSPANEL_DATA_DIR")
	if dataDir == "" {
		dataDir = filepath.Dir(panelsFile)
	}

	metricsDBPath := os.Getenv("VLESSPANEL_METRICS_DB")
	if metricsDBPath == "" {
		metricsDBPath = filepath.Join(dataDir, "metrics.db")
	}

	publicURL := os.Getenv("VLESSPANEL_PUBLIC_URL")
	if publicURL == "" {
		publicURL = "https://example.com"
	}

	syncScript := os.Getenv("VLESSPANEL_SYNC_SCRIPT")
	if syncScript == "" {
		syncScript = "/opt/aggregator-configs/sync-configs.sh"
	}

	adminToken := os.Getenv("VLESSPANEL_ADMIN_TOKEN")

	return Config{
		Port:                  port,
		AggregatorDir:         aggDir,
		PanelsFilePath:        panelsFile,
		StaticDir:             staticDir,
		VlessSubTestDaemonURL: daemonURL,
		DataDir:               dataDir,
		PublicURL:             publicURL,
		SyncScript:            syncScript,
		AdminToken:            adminToken,
		MetricsDBPath:         metricsDBPath,
		ReadHeaderTimeout:     envDuration("VLESSPANEL_READ_HEADER_TIMEOUT", 10*time.Second),
		ReadTimeout:           envDuration("VLESSPANEL_READ_TIMEOUT", 30*time.Second),
		WriteTimeout:          envDuration("VLESSPANEL_WRITE_TIMEOUT", 2*time.Minute),
		IdleTimeout:           envDuration("VLESSPANEL_IDLE_TIMEOUT", 2*time.Minute),
		ShutdownTimeout:       envDuration("VLESSPANEL_SHUTDOWN_TIMEOUT", 8*time.Second),
	}
}

// envDuration читает duration из env-переменной с дефолтом def. При невалидном
// значении логирует предупреждение и возвращает def.
func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Printf("config: invalid duration for %s (%q), using %s", key, v, def)
		return def
	}
	return d
}
