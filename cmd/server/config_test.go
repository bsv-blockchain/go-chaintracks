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
		envVars        map[string]string
		expectedPort   int
		expectedNet    string
		expectedPath   string
		expectedBootst string
	}{
		{
			name:           "LoadsDefaultValues",
			envVars:        nil,
			expectedPort:   3011,
			expectedNet:    "main",
			expectedPath:   getDefaultStoragePath(),
			expectedBootst: "",
		},
		{
			name:           "LoadsPortFromEnvironment",
			envVars:        map[string]string{"PORT": "8080"},
			expectedPort:   8080,
			expectedNet:    "main",
			expectedPath:   getDefaultStoragePath(),
			expectedBootst: "",
		},
		{
			name:           "LoadsNetworkFromEnvironment",
			envVars:        map[string]string{"CHAIN": "testnet"},
			expectedPort:   3011,
			expectedNet:    "testnet",
			expectedPath:   getDefaultStoragePath(),
			expectedBootst: "",
		},
		{
			name:           "LoadsStoragePathFromEnvironment",
			envVars:        map[string]string{"STORAGE_PATH": "/custom/path"},
			expectedPort:   3011,
			expectedNet:    "main",
			expectedPath:   "/custom/path",
			expectedBootst: "",
		},
		{
			name:           "LoadsBootstrapURLFromEnvironment",
			envVars:        map[string]string{"BOOTSTRAP_URL": "http://example.com"},
			expectedPort:   3011,
			expectedNet:    "main",
			expectedPath:   getDefaultStoragePath(),
			expectedBootst: "http://example.com",
		},
		{
			name: "LoadsAllValuesFromEnvironment",
			envVars: map[string]string{
				"PORT":          "9999",
				"CHAIN":         "regtest",
				"STORAGE_PATH":  "/full/custom/path",
				"BOOTSTRAP_URL": "http://bootstrap.example.com",
			},
			expectedPort:   9999,
			expectedNet:    "regtest",
			expectedPath:   "/full/custom/path",
			expectedBootst: "http://bootstrap.example.com",
		},
		{
			name:           "UsesDefaultPortWhenPortIsInvalid",
			envVars:        map[string]string{"PORT": "invalid"},
			expectedPort:   3011,
			expectedNet:    "main",
			expectedPath:   getDefaultStoragePath(),
			expectedBootst: "",
		},
		{
			name:           "UsesDefaultPortWhenPortIsEmpty",
			envVars:        map[string]string{"PORT": ""},
			expectedPort:   3011,
			expectedNet:    "main",
			expectedPath:   getDefaultStoragePath(),
			expectedBootst: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := withEnvVars(t, tt.envVars)
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
