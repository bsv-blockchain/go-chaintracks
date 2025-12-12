// Package config provides configuration and initialization for chaintracks.
package config

import (
	p2p "github.com/bsv-blockchain/go-teranode-p2p-client"
)

// Mode specifies which chaintracks implementation to use.
type Mode string

const (
	ModeEmbedded Mode = "embedded" // Run chainmanager locally
	ModeRemote   Mode = "remote"   // Connect to remote chaintracks server
)

// Config holds chaintracks configuration.
type Config struct {
	Mode         Mode       `mapstructure:"mode"`          // "embedded" or "remote"
	URL          string     `mapstructure:"url"`           // Remote server URL (required for remote mode)
	StoragePath  string     `mapstructure:"storage_path"`  // Local storage path (for embedded mode)
	BootstrapURL string     `mapstructure:"bootstrap_url"` // Bootstrap URL for initial sync (for embedded mode)
	P2P          p2p.Config `mapstructure:"p2p"`           // P2P configuration (for embedded mode)
}
