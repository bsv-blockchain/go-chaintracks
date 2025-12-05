package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"
)

// Config holds the server configuration
type Config struct {
	Port           int
	Network        string
	StoragePath    string
	BootstrapURL   string
	BootstrapPeers []string
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

	bootstrapPeers := loadBootstrapPeers(network)

	return &Config{
		Port:           port,
		Network:        network,
		StoragePath:    storagePath,
		BootstrapURL:   bootstrapURL,
		BootstrapPeers: bootstrapPeers,
	}
}

// loadBootstrapPeers loads P2P bootstrap peers from data/bootstrap_peers.json
func loadBootstrapPeers(network string) []string {
	data, err := os.ReadFile("data/bootstrap_peers.json")
	if err != nil {
		log.Printf("Warning: could not load bootstrap_peers.json: %v", err)
		return nil
	}

	var peers map[string][]string
	if err := json.Unmarshal(data, &peers); err != nil {
		log.Printf("Warning: could not parse bootstrap_peers.json: %v", err)
		return nil
	}

	if networkPeers, ok := peers[network]; ok {
		log.Printf("Loaded %d bootstrap peers for network %s", len(networkPeers), network)
		return networkPeers
	}

	log.Printf("Warning: no bootstrap peers found for network %s", network)
	return nil
}

// getDefaultStoragePath returns ~/.chaintracks as the default storage path
func getDefaultStoragePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "./data/headers"
	}
	return filepath.Join(home, ".chaintracks")
}
