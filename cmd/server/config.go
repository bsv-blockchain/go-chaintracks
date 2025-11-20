package main

import (
	"os"
	"path/filepath"
	"strconv"
)

// Config holds the server configuration
type Config struct {
	Port         int
	Network      string
	StoragePath  string
	BootstrapURL string
}

// LoadConfig loads configuration from environment variables with defaults
func LoadConfig() *Config {
	port := 3011
	if portStr := os.Getenv("PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}

	network := "main"
	if net := os.Getenv("CHAIN"); net != "" {
		network = net
	}

	storagePath := getDefaultStoragePath()
	if path := os.Getenv("STORAGE_PATH"); path != "" {
		storagePath = path
	}

	bootstrapURL := os.Getenv("BOOTSTRAP_URL")

	return &Config{
		Port:         port,
		Network:      network,
		StoragePath:  storagePath,
		BootstrapURL: bootstrapURL,
	}
}

// getDefaultStoragePath returns ~/.chaintracks as the default storage path
func getDefaultStoragePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./data/headers"
	}
	return filepath.Join(home, ".chaintracks")
}
