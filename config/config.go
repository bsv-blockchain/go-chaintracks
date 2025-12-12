// Package config provides configuration and initialization for chaintracks.
package config

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/bsv-blockchain/go-chaintracks/chainmanager"
	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
	"github.com/bsv-blockchain/go-chaintracks/client"
	p2p "github.com/bsv-blockchain/go-teranode-p2p-client"
	"github.com/spf13/viper"
)

// SetDefaults sets viper defaults for chaintracks configuration when used as an embedded library.
func (c *Config) SetDefaults(v *viper.Viper, configPath string) {
	prefix := ""
	if configPath != "" {
		prefix = configPath + "."
	}
	v.SetDefault(prefix+"mode", "embedded")
	v.SetDefault(prefix+"storage_path", "~/.chaintracks")
	c.P2P.SetDefaults(v, prefix+"p2p")
}

// Initialize creates and returns the appropriate chaintracks implementation.
// Name identifies this client on the P2P network (ignored if p2pClient is provided).
// If p2pClient is non-nil in embedded mode, it will be used instead of creating a new one.
func (c *Config) Initialize(ctx context.Context, name string, p2pClient *p2p.Client) (chaintracks.Chaintracks, error) {
	switch c.Mode {
	case ModeRemote:
		if c.URL == "" {
			return nil, fmt.Errorf("chaintracks URL required for remote mode")
		}
		return client.New(c.URL), nil

	case ModeEmbedded, "":
		// Expand ~ in storage path
		if len(c.StoragePath) >= 2 && c.StoragePath[:2] == "~/" {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("failed to resolve home directory: %w", err)
			}
			c.StoragePath = path.Join(homeDir, c.StoragePath[2:])
		}

		// Use provided P2P client or create a new one
		if p2pClient == nil {
			// Set P2P storage path to match chaintracks if not specified
			if c.P2P.StoragePath == "" {
				c.P2P.StoragePath = c.StoragePath
			}
			// Set P2P network from chaintracks config if not specified
			if c.P2P.Network == "" {
				c.P2P.Network = "main"
			}

			var err error
			p2pClient, err = c.P2P.Initialize(ctx, name)
			if err != nil {
				return nil, fmt.Errorf("failed to create P2P client: %w", err)
			}
		}

		return chainmanager.New(ctx, p2pClient.GetNetwork(), c.StoragePath, p2pClient, c.BootstrapURL)

	default:
		return nil, fmt.Errorf("unknown chaintracks mode: %s", c.Mode)
	}
}
