package main

import (
	"os"
	"path/filepath"
)

// Config holds application configuration
type Config struct {
	Port             string
	AggregatorDir    string
	PanelsFilePath   string
	StaticDir        string
	VlessSubTestPath string
	SingBoxPath      string
}

// LoadConfig loads configuration from environment variables with defaults
func LoadConfig() Config {
	port := os.Getenv("VLESSPANEL_PORT")
	if port == "" {
		port = "8080"
	}

	aggDir := os.Getenv("VLESSPANEL_AGGREGATOR_DIR")
	if aggDir == "" {
		aggDir = "/home/klem/VlessAggregator"
	}

	panelsFile := os.Getenv("VLESSPANEL_PANELS_FILE")
	if panelsFile == "" {
		panelsFile = "panels.json"
	}

	staticDir := os.Getenv("VLESSPANEL_STATIC_DIR")
	if staticDir == "" {
		staticDir = filepath.Join("..", "frontend", "dist")
	}

	vstPath := os.Getenv("VLESSPANEL_VLESSSUBTEST_PATH")
	if vstPath == "" {
		vstPath = "/home/klem/VlessSubTest/vlesssubtest"
	}

	sbPath := os.Getenv("VLESSPANEL_SINGBOX_PATH")
	if sbPath == "" {
		sbPath = "/home/klem/VlessSubTest/sing-box"
	}

	return Config{
		Port:             port,
		AggregatorDir:    aggDir,
		PanelsFilePath:   panelsFile,
		StaticDir:        staticDir,
		VlessSubTestPath: vstPath,
		SingBoxPath:      sbPath,
	}
}

func (c Config) SubscriptionFilePath(name string) string {
	return filepath.Join(c.AggregatorDir, "config-"+name+".txt")
}
