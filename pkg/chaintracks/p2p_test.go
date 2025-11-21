package chaintracks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChainManagerGetPeers(t *testing.T) {
	tests := []struct {
		name        string
		setupCM     func() *ChainManager
		expectEmpty bool
	}{
		{
			name: "NilP2PClientReturnsEmptySlice",
			setupCM: func() *ChainManager {
				return &ChainManager{
					p2pClient: nil,
				}
			},
			expectEmpty: true,
		},
		{
			name: "P2PClientNotInitializedReturnsEmpty",
			setupCM: func() *ChainManager {
				// P2P client not initialized
				return &ChainManager{
					p2pClient: nil,
				}
			},
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := tt.setupCM()
			peers := cm.GetPeers()

			if tt.expectEmpty {
				assert.Empty(t, peers, "Expected empty peer list")
			} else {
				assert.NotEmpty(t, peers, "Expected non-empty peer list")
			}
		})
	}
}
