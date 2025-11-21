package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name           string
		setupEnv       func() func()
		expectedPort   int
		expectedNet    string
		expectedPath   string
		expectedBootst string
	}{
		{
			name: "LoadsDefaultValues",
			setupEnv: func() func() {
				// Clear all environment variables
				oldPort := os.Getenv("PORT")
				oldChain := os.Getenv("CHAIN")
				oldStorage := os.Getenv("STORAGE_PATH")
				oldBootstrap := os.Getenv("BOOTSTRAP_URL")

				_ = os.Unsetenv("PORT")
				_ = os.Unsetenv("CHAIN")
				_ = os.Unsetenv("STORAGE_PATH")
				_ = os.Unsetenv("BOOTSTRAP_URL")

				return func() {
					_ = os.Setenv("PORT", oldPort)
					_ = os.Setenv("CHAIN", oldChain)
					_ = os.Setenv("STORAGE_PATH", oldStorage)
					_ = os.Setenv("BOOTSTRAP_URL", oldBootstrap)
				}
			},
			expectedPort:   3011,
			expectedNet:    "main",
			expectedPath:   getDefaultStoragePath(),
			expectedBootst: "",
		},
		{
			name: "LoadsPortFromEnvironment",
			setupEnv: func() func() {
				oldPort := os.Getenv("PORT")
				oldChain := os.Getenv("CHAIN")
				oldStorage := os.Getenv("STORAGE_PATH")
				oldBootstrap := os.Getenv("BOOTSTRAP_URL")

				_ = os.Setenv("PORT", "8080")
				_ = os.Unsetenv("CHAIN")
				_ = os.Unsetenv("STORAGE_PATH")
				_ = os.Unsetenv("BOOTSTRAP_URL")

				return func() {
					_ = os.Setenv("PORT", oldPort)
					_ = os.Setenv("CHAIN", oldChain)
					_ = os.Setenv("STORAGE_PATH", oldStorage)
					_ = os.Setenv("BOOTSTRAP_URL", oldBootstrap)
				}
			},
			expectedPort:   8080,
			expectedNet:    "main",
			expectedPath:   getDefaultStoragePath(),
			expectedBootst: "",
		},
		{
			name: "LoadsNetworkFromEnvironment",
			setupEnv: func() func() {
				oldPort := os.Getenv("PORT")
				oldChain := os.Getenv("CHAIN")
				oldStorage := os.Getenv("STORAGE_PATH")
				oldBootstrap := os.Getenv("BOOTSTRAP_URL")

				_ = os.Unsetenv("PORT")
				_ = os.Setenv("CHAIN", "testnet")
				_ = os.Unsetenv("STORAGE_PATH")
				_ = os.Unsetenv("BOOTSTRAP_URL")

				return func() {
					_ = os.Setenv("PORT", oldPort)
					_ = os.Setenv("CHAIN", oldChain)
					_ = os.Setenv("STORAGE_PATH", oldStorage)
					_ = os.Setenv("BOOTSTRAP_URL", oldBootstrap)
				}
			},
			expectedPort:   3011,
			expectedNet:    "testnet",
			expectedPath:   getDefaultStoragePath(),
			expectedBootst: "",
		},
		{
			name: "LoadsStoragePathFromEnvironment",
			setupEnv: func() func() {
				oldPort := os.Getenv("PORT")
				oldChain := os.Getenv("CHAIN")
				oldStorage := os.Getenv("STORAGE_PATH")
				oldBootstrap := os.Getenv("BOOTSTRAP_URL")

				_ = os.Unsetenv("PORT")
				_ = os.Unsetenv("CHAIN")
				_ = os.Setenv("STORAGE_PATH", "/custom/path")
				_ = os.Unsetenv("BOOTSTRAP_URL")

				return func() {
					_ = os.Setenv("PORT", oldPort)
					_ = os.Setenv("CHAIN", oldChain)
					_ = os.Setenv("STORAGE_PATH", oldStorage)
					_ = os.Setenv("BOOTSTRAP_URL", oldBootstrap)
				}
			},
			expectedPort:   3011,
			expectedNet:    "main",
			expectedPath:   "/custom/path",
			expectedBootst: "",
		},
		{
			name: "LoadsBootstrapURLFromEnvironment",
			setupEnv: func() func() {
				oldPort := os.Getenv("PORT")
				oldChain := os.Getenv("CHAIN")
				oldStorage := os.Getenv("STORAGE_PATH")
				oldBootstrap := os.Getenv("BOOTSTRAP_URL")

				_ = os.Unsetenv("PORT")
				_ = os.Unsetenv("CHAIN")
				_ = os.Unsetenv("STORAGE_PATH")
				_ = os.Setenv("BOOTSTRAP_URL", "http://example.com")

				return func() {
					_ = os.Setenv("PORT", oldPort)
					_ = os.Setenv("CHAIN", oldChain)
					_ = os.Setenv("STORAGE_PATH", oldStorage)
					_ = os.Setenv("BOOTSTRAP_URL", oldBootstrap)
				}
			},
			expectedPort:   3011,
			expectedNet:    "main",
			expectedPath:   getDefaultStoragePath(),
			expectedBootst: "http://example.com",
		},
		{
			name: "LoadsAllValuesFromEnvironment",
			setupEnv: func() func() {
				oldPort := os.Getenv("PORT")
				oldChain := os.Getenv("CHAIN")
				oldStorage := os.Getenv("STORAGE_PATH")
				oldBootstrap := os.Getenv("BOOTSTRAP_URL")

				_ = os.Setenv("PORT", "9999")
				_ = os.Setenv("CHAIN", "regtest")
				_ = os.Setenv("STORAGE_PATH", "/full/custom/path")
				_ = os.Setenv("BOOTSTRAP_URL", "http://bootstrap.example.com")

				return func() {
					_ = os.Setenv("PORT", oldPort)
					_ = os.Setenv("CHAIN", oldChain)
					_ = os.Setenv("STORAGE_PATH", oldStorage)
					_ = os.Setenv("BOOTSTRAP_URL", oldBootstrap)
				}
			},
			expectedPort:   9999,
			expectedNet:    "regtest",
			expectedPath:   "/full/custom/path",
			expectedBootst: "http://bootstrap.example.com",
		},
		{
			name: "UsesDefaultPortWhenPortIsInvalid",
			setupEnv: func() func() {
				oldPort := os.Getenv("PORT")
				oldChain := os.Getenv("CHAIN")
				oldStorage := os.Getenv("STORAGE_PATH")
				oldBootstrap := os.Getenv("BOOTSTRAP_URL")

				_ = os.Setenv("PORT", "invalid")
				_ = os.Unsetenv("CHAIN")
				_ = os.Unsetenv("STORAGE_PATH")
				_ = os.Unsetenv("BOOTSTRAP_URL")

				return func() {
					_ = os.Setenv("PORT", oldPort)
					_ = os.Setenv("CHAIN", oldChain)
					_ = os.Setenv("STORAGE_PATH", oldStorage)
					_ = os.Setenv("BOOTSTRAP_URL", oldBootstrap)
				}
			},
			expectedPort:   3011,
			expectedNet:    "main",
			expectedPath:   getDefaultStoragePath(),
			expectedBootst: "",
		},
		{
			name: "UsesDefaultPortWhenPortIsEmpty",
			setupEnv: func() func() {
				oldPort := os.Getenv("PORT")
				oldChain := os.Getenv("CHAIN")
				oldStorage := os.Getenv("STORAGE_PATH")
				oldBootstrap := os.Getenv("BOOTSTRAP_URL")

				_ = os.Setenv("PORT", "")
				_ = os.Unsetenv("CHAIN")
				_ = os.Unsetenv("STORAGE_PATH")
				_ = os.Unsetenv("BOOTSTRAP_URL")

				return func() {
					_ = os.Setenv("PORT", oldPort)
					_ = os.Setenv("CHAIN", oldChain)
					_ = os.Setenv("STORAGE_PATH", oldStorage)
					_ = os.Setenv("BOOTSTRAP_URL", oldBootstrap)
				}
			},
			expectedPort:   3011,
			expectedNet:    "main",
			expectedPath:   getDefaultStoragePath(),
			expectedBootst: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := tt.setupEnv()
			defer cleanup()

			config := LoadConfig()

			require.NotNil(t, config)
			assert.Equal(t, tt.expectedPort, config.Port)
			assert.Equal(t, tt.expectedNet, config.Network)
			assert.Equal(t, tt.expectedPath, config.StoragePath)
			assert.Equal(t, tt.expectedBootst, config.BootstrapURL)
		})
	}
}

func TestGetDefaultStoragePath(t *testing.T) {
	tests := []struct {
		name         string
		expectedPath string
	}{
		{
			name: "ReturnsHomeDirectoryPlusChaintracks",
			expectedPath: func() string {
				home, err := os.UserHomeDir()
				if err != nil {
					return "./data/headers"
				}
				return filepath.Join(home, ".chaintracks")
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getDefaultStoragePath()
			assert.Equal(t, tt.expectedPath, result)
		})
	}
}
