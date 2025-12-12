package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name         string
		envVars      map[string]string
		expectedPort int
	}{
		{
			name:         "LoadsDefaultValues",
			envVars:      nil,
			expectedPort: 3011,
		},
		{
			name:         "LoadsPortFromEnvironment",
			envVars:      map[string]string{"CHAINTRACKS_PORT": "8080"},
			expectedPort: 8080,
		},
		{
			name:         "UsesDefaultPortWhenPortIsEmpty",
			envVars:      map[string]string{"CHAINTRACKS_PORT": ""},
			expectedPort: 3011,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := withEnvVars(t, tt.envVars)
			defer cleanup()

			config, err := Load()

			require.NoError(t, err)
			require.NotNil(t, config)
			assert.Equal(t, tt.expectedPort, config.Port)
		})
	}
}

