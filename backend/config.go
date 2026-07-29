package main

import (
	"os"
	"path/filepath"
)

// Config holds application configuration
type Config struct {
	Port                  string
	AggregatorDir         string
	PanelsFilePath        string
	StaticDir             string
	VlessSubTestDaemonURL string
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

	return Config{
		Port:                  port,
		AggregatorDir:         aggDir,
		PanelsFilePath:        panelsFile,
		StaticDir:             staticDir,
		VlessSubTestDaemonURL: daemonURL,
	}
}

func (c Config) SubscriptionFilePath(name string) string {
	return filepath.Join(c.AggregatorDir, "configs-"+name+".txt")
}
