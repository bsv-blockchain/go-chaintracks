package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
	"github.com/bsv-blockchain/go-chaintracks/config"
	"github.com/spf13/viper"
)

// AppConfig holds all configuration for the server application.
type AppConfig struct {
	Port        int           `mapstructure:"port"`
	Chaintracks config.Config `mapstructure:"chaintracks"`
}

// Load reads configuration from file and environment variables.
func Load() (*AppConfig, error) {
	v := viper.New()

	cfg := &AppConfig{}

	// Set defaults
	v.SetDefault("port", 3011)
	cfg.Chaintracks.SetDefaults(v, "chaintracks")

	// Config file settings
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("$HOME/.chaintracks")
	v.AddConfigPath("/etc/chaintracks")

	// Environment variable settings
	v.SetEnvPrefix("CHAINTRACKS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file (optional - env vars can provide everything)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return cfg, nil
}

// Initialize creates and returns the chaintracks service.
func (c *AppConfig) Initialize(ctx context.Context) (chaintracks.Chaintracks, error) {
	return c.Chaintracks.Initialize(ctx, "chaintracks", nil)
}
