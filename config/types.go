// Package config provides configuration and initialization for chaintracks.
package config

import (
	p2p "github.com/bsv-blockchain/go-teranode-p2p-client"
)

// Mode specifies which chaintracks implementation to use.
type Mode string

// Chaintracks mode constants.
const (
	ModeEmbedded Mode = "embedded" // Run chainmanager locally
	ModeRemote   Mode = "remote"   // Connect to remote chaintracks server
)

// BootstrapMode specifies how to bootstrap the chain.
type BootstrapMode string

// Bootstrap mode constants.
const (
	BootstrapModeAPI BootstrapMode = "api" // Gorillanode-style binary API (default)
	BootstrapModeCDN BootstrapMode = "cdn" // TypeScript CDN-style static files
)

// Config holds chaintracks configuration.
type Config struct {
	Mode          Mode          `mapstructure:"mode"`           // "embedded" or "remote"
	URL           string        `mapstructure:"url"`            // Remote server URL (required for remote mode)
	StoragePath   string        `mapstructure:"storage_path"`   // Local storage path (for embedded mode)
	BootstrapURL  string        `mapstructure:"bootstrap_url"`  // Bootstrap URL for initial sync (for embedded mode)
	BootstrapMode BootstrapMode `mapstructure:"bootstrap_mode"` // "api" or "cdn" (default: api)
	P2P           p2p.Config    `mapstructure:"p2p"`            // P2P configuration (for embedded mode)
}
