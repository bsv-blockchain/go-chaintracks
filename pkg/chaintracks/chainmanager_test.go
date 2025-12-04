package chaintracks

import (
	"testing"

	"github.com/bsv-blockchain/go-sdk/block"
	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChainManagerGetTip(t *testing.T) {
	tests := []struct {
		name     string
		setupCM  func() *ChainManager
		expected *BlockHeader
	}{
		{
			name: "ReturnsNilWhenTipIsNil",
			setupCM: func() *ChainManager {
				return &ChainManager{
					tip: nil,
				}
			},
			expected: nil,
		},
		{
			name: "ReturnsTipWhenTipExists",
			setupCM: func() *ChainManager {
				hash := chainhash.Hash{}
				header := &BlockHeader{
					Header: &block.Header{},
					Height: 12345,
					Hash:   hash,
				}
				return &ChainManager{
					tip: header,
				}
			},
			expected: &BlockHeader{
				Header: &block.Header{},
				Height: 12345,
				Hash:   chainhash.Hash{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := tt.setupCM()
			result := cm.GetTip(t.Context())

			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expected.Height, result.Height)
				assert.Equal(t, tt.expected.Hash, result.Hash)
			}
		})
	}
}

func TestChainManagerGetHeight(t *testing.T) {
	tests := []struct {
		name     string
		setupCM  func() *ChainManager
		expected uint32
	}{
		{
			name: "ReturnsZeroWhenTipIsNil",
			setupCM: func() *ChainManager {
				return &ChainManager{
					tip: nil,
				}
			},
			expected: 0,
		},
		{
			name: "ReturnsTipHeightWhenTipExists",
			setupCM: func() *ChainManager {
				return &ChainManager{
					tip: &BlockHeader{
						Header: &block.Header{},
						Height: 12345,
					},
				}
			},
			expected: 12345,
		},
		{
			name: "ReturnsCorrectHeightForGenesisBlock",
			setupCM: func() *ChainManager {
				return &ChainManager{
					tip: &BlockHeader{
						Header: &block.Header{},
						Height: 0,
					},
				}
			},
			expected: 0,
		},
		{
			name: "ReturnsCorrectHeightForHighBlock",
			setupCM: func() *ChainManager {
				return &ChainManager{
					tip: &BlockHeader{
						Header: &block.Header{},
						Height: 800000,
					},
				}
			},
			expected: 800000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := tt.setupCM()
			result := cm.GetHeight(t.Context())
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestChainManagerGetNetwork(t *testing.T) {
	tests := []struct {
		name            string
		network         string
		expectedNetwork string
		expectedError   error
	}{
		{
			name:            "ReturnsMainnetNetwork",
			network:         "mainnet",
			expectedNetwork: "mainnet",
			expectedError:   nil,
		},
		{
			name:            "ReturnsTestnetNetwork",
			network:         "testnet",
			expectedNetwork: "testnet",
			expectedError:   nil,
		},
		{
			name:            "ReturnsEmptyNetwork",
			network:         "",
			expectedNetwork: "",
			expectedError:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &ChainManager{
				network: tt.network,
			}

			result, err := cm.GetNetwork(t.Context())

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedNetwork, result)
			}
		})
	}
}

func TestChainManagerGetHeaderByHeight(t *testing.T) {
	// Create test headers
	hash1 := chainhash.Hash{1}
	hash2 := chainhash.Hash{2}
	hash3 := chainhash.Hash{3}

	header1 := &BlockHeader{
		Header: &block.Header{},
		Height: 0,
		Hash:   hash1,
	}
	header2 := &BlockHeader{
		Header: &block.Header{},
		Height: 1,
		Hash:   hash2,
	}
	header3 := &BlockHeader{
		Header: &block.Header{},
		Height: 2,
		Hash:   hash3,
	}

	tests := []struct {
		name          string
		setupCM       func() *ChainManager
		height        uint32
		expectedHash  chainhash.Hash
		expectedError error
	}{
		{
			name: "ReturnsHeaderForValidHeight",
			setupCM: func() *ChainManager {
				return &ChainManager{
					byHeight: []chainhash.Hash{hash1, hash2, hash3},
					byHash: map[chainhash.Hash]*BlockHeader{
						hash1: header1,
						hash2: header2,
						hash3: header3,
					},
				}
			},
			height:        1,
			expectedHash:  hash2,
			expectedError: nil,
		},
		{
			name: "ReturnsHeaderForGenesisBlock",
			setupCM: func() *ChainManager {
				return &ChainManager{
					byHeight: []chainhash.Hash{hash1, hash2, hash3},
					byHash: map[chainhash.Hash]*BlockHeader{
						hash1: header1,
						hash2: header2,
						hash3: header3,
					},
				}
			},
			height:        0,
			expectedHash:  hash1,
			expectedError: nil,
		},
		{
			name: "ReturnsErrorWhenHeightOutOfRange",
			setupCM: func() *ChainManager {
				return &ChainManager{
					byHeight: []chainhash.Hash{hash1, hash2},
					byHash: map[chainhash.Hash]*BlockHeader{
						hash1: header1,
						hash2: header2,
					},
				}
			},
			height:        10,
			expectedError: ErrHeaderNotFound,
		},
		{
			name: "ReturnsErrorWhenChainIsEmpty",
			setupCM: func() *ChainManager {
				return &ChainManager{
					byHeight: []chainhash.Hash{},
					byHash:   map[chainhash.Hash]*BlockHeader{},
				}
			},
			height:        0,
			expectedError: ErrHeaderNotFound,
		},
		{
			name: "ReturnsErrorWhenHashNotInByHash",
			setupCM: func() *ChainManager {
				return &ChainManager{
					byHeight: []chainhash.Hash{hash1},
					byHash:   map[chainhash.Hash]*BlockHeader{},
				}
			},
			height:        0,
			expectedError: ErrHeaderNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := tt.setupCM()
			result, err := cm.GetHeaderByHeight(t.Context(), tt.height)

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedHash, result.Hash)
			}
		})
	}
}

func TestChainManagerGetHeaderByHash(t *testing.T) {
	// Create test headers
	hash1 := chainhash.Hash{1}
	hash2 := chainhash.Hash{2}
	hash3 := chainhash.Hash{3}
	hashNotFound := chainhash.Hash{99}

	header1 := &BlockHeader{
		Header: &block.Header{},
		Height: 0,
		Hash:   hash1,
	}
	header2 := &BlockHeader{
		Header: &block.Header{},
		Height: 1,
		Hash:   hash2,
	}

	tests := []struct {
		name           string
		setupCM        func() *ChainManager
		hash           *chainhash.Hash
		expectedHeight uint32
		expectedError  error
	}{
		{
			name: "ReturnsHeaderForValidHash",
			setupCM: func() *ChainManager {
				return &ChainManager{
					byHash: map[chainhash.Hash]*BlockHeader{
						hash1: header1,
						hash2: header2,
					},
				}
			},
			hash:           &hash1,
			expectedHeight: 0,
			expectedError:  nil,
		},
		{
			name: "ReturnsHeaderForAnotherValidHash",
			setupCM: func() *ChainManager {
				return &ChainManager{
					byHash: map[chainhash.Hash]*BlockHeader{
						hash1: header1,
						hash2: header2,
					},
				}
			},
			hash:           &hash2,
			expectedHeight: 1,
			expectedError:  nil,
		},
		{
			name: "ReturnsErrorWhenHashNotFound",
			setupCM: func() *ChainManager {
				return &ChainManager{
					byHash: map[chainhash.Hash]*BlockHeader{
						hash1: header1,
						hash2: header2,
					},
				}
			},
			hash:          &hashNotFound,
			expectedError: ErrHeaderNotFound,
		},
		{
			name: "ReturnsErrorWhenByHashIsEmpty",
			setupCM: func() *ChainManager {
				return &ChainManager{
					byHash: map[chainhash.Hash]*BlockHeader{},
				}
			},
			hash:          &hash3,
			expectedError: ErrHeaderNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := tt.setupCM()
			result, err := cm.GetHeaderByHash(t.Context(), tt.hash)

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedHeight, result.Height)
				assert.Equal(t, *tt.hash, result.Hash)
			}
		})
	}
}

func TestChainManagerAddHeader(t *testing.T) {
	tests := []struct {
		name        string
		setupCM     func() *ChainManager
		headerToAdd *BlockHeader
		verifyFunc  func(*testing.T, *ChainManager)
	}{
		{
			name: "AddsHeaderToEmptyChainManager",
			setupCM: func() *ChainManager {
				return &ChainManager{
					byHash: make(map[chainhash.Hash]*BlockHeader),
				}
			},
			headerToAdd: &BlockHeader{
				Header: &block.Header{},
				Height: 0,
				Hash:   chainhash.Hash{1},
			},
			verifyFunc: func(t *testing.T, cm *ChainManager) {
				hash := chainhash.Hash{1}
				header, ok := cm.byHash[hash]
				require.True(t, ok, "Header should be in byHash map")
				assert.Equal(t, uint32(0), header.Height)
				assert.Equal(t, hash, header.Hash)
			},
		},
		{
			name: "AddsHeaderToExistingChainManager",
			setupCM: func() *ChainManager {
				existingHash := chainhash.Hash{1}
				return &ChainManager{
					byHash: map[chainhash.Hash]*BlockHeader{
						existingHash: {
							Header: &block.Header{},
							Height: 0,
							Hash:   existingHash,
						},
					},
				}
			},
			headerToAdd: &BlockHeader{
				Header: &block.Header{},
				Height: 1,
				Hash:   chainhash.Hash{2},
			},
			verifyFunc: func(t *testing.T, cm *ChainManager) {
				assert.Len(t, cm.byHash, 2, "Should have 2 headers")
				newHash := chainhash.Hash{2}
				header, ok := cm.byHash[newHash]
				require.True(t, ok, "New header should be in byHash map")
				assert.Equal(t, uint32(1), header.Height)
				assert.Equal(t, newHash, header.Hash)
			},
		},
		{
			name: "OverwritesExistingHeaderWithSameHash",
			setupCM: func() *ChainManager {
				hash := chainhash.Hash{1}
				return &ChainManager{
					byHash: map[chainhash.Hash]*BlockHeader{
						hash: {
							Header: &block.Header{},
							Height: 0,
							Hash:   hash,
						},
					},
				}
			},
			headerToAdd: &BlockHeader{
				Header: &block.Header{},
				Height: 999,
				Hash:   chainhash.Hash{1},
			},
			verifyFunc: func(t *testing.T, cm *ChainManager) {
				assert.Len(t, cm.byHash, 1, "Should still have 1 header")
				hash := chainhash.Hash{1}
				header, ok := cm.byHash[hash]
				require.True(t, ok, "Header should be in byHash map")
				assert.Equal(t, uint32(999), header.Height, "Height should be updated")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := tt.setupCM()
			err := cm.AddHeader(tt.headerToAdd)
			require.NoError(t, err)
			tt.verifyFunc(t, cm)
		})
	}
}

func TestChainManagerPruneOrphans(t *testing.T) {
	tests := []struct {
		name       string
		setupCM    func() *ChainManager
		verifyFunc func(t *testing.T, cm *ChainManager)
	}{
		{
			name: "NilTipReturnsEarly",
			setupCM: func() *ChainManager {
				return &ChainManager{
					tip:      nil,
					byHash:   make(map[chainhash.Hash]*BlockHeader),
					byHeight: []chainhash.Hash{},
				}
			},
			verifyFunc: func(t *testing.T, cm *ChainManager) {
				t.Helper()
				// Should not panic or error
				// byHash should remain empty
				assert.Empty(t, cm.byHash)
			},
		},
		{
			name: "TipHeightLessThan100NoPruning",
			setupCM: func() *ChainManager {
				cm := &ChainManager{
					byHash:   make(map[chainhash.Hash]*BlockHeader),
					byHeight: make([]chainhash.Hash, 51),
				}

				// Create tip at height 50
				tipHash := chainhash.Hash{0x01}
				tip := &BlockHeader{
					Header: &block.Header{},
					Height: 50,
					Hash:   tipHash,
				}
				cm.tip = tip
				cm.byHash[tipHash] = tip
				cm.byHeight[50] = tipHash

				// Create an orphan at height 10 (should NOT be pruned)
				orphanHash := chainhash.Hash{0x02}
				orphan := &BlockHeader{
					Header: &block.Header{},
					Height: 10,
					Hash:   orphanHash,
				}
				cm.byHash[orphanHash] = orphan

				return cm
			},
			verifyFunc: func(t *testing.T, cm *ChainManager) {
				t.Helper()
				// Orphan should still exist (no pruning when tip < 100)
				_, exists := cm.byHash[chainhash.Hash{0x02}]
				assert.True(t, exists, "Orphan should not be pruned when tip height < 100")
				assert.Len(t, cm.byHash, 2, "Should have 2 headers (tip + orphan)")
			},
		},
		{
			name: "PrunesOldOrphansAboveHeight100",
			setupCM: func() *ChainManager {
				cm := &ChainManager{
					byHash:   make(map[chainhash.Hash]*BlockHeader),
					byHeight: make([]chainhash.Hash, 201),
				}

				// Create tip at height 200
				tipHash := chainhash.Hash{0x01}
				tip := &BlockHeader{
					Header: &block.Header{},
					Height: 200,
					Hash:   tipHash,
				}
				cm.tip = tip
				cm.byHash[tipHash] = tip
				cm.byHeight[200] = tipHash

				// Create old orphan at height 50 (should be pruned, 200-100=100)
				oldOrphanHash := chainhash.Hash{0x02}
				oldOrphan := &BlockHeader{
					Header: &block.Header{},
					Height: 50,
					Hash:   oldOrphanHash,
				}
				cm.byHash[oldOrphanHash] = oldOrphan

				// Create recent orphan at height 150 (should NOT be pruned)
				recentOrphanHash := chainhash.Hash{0x03}
				recentOrphan := &BlockHeader{
					Header: &block.Header{},
					Height: 150,
					Hash:   recentOrphanHash,
				}
				cm.byHash[recentOrphanHash] = recentOrphan

				return cm
			},
			verifyFunc: func(t *testing.T, cm *ChainManager) {
				t.Helper()
				// Old orphan should be pruned
				_, oldExists := cm.byHash[chainhash.Hash{0x02}]
				assert.False(t, oldExists, "Old orphan (height 50) should be pruned")

				// Recent orphan should still exist
				_, recentExists := cm.byHash[chainhash.Hash{0x03}]
				assert.True(t, recentExists, "Recent orphan (height 150) should not be pruned")

				// Tip should still exist
				_, tipExists := cm.byHash[chainhash.Hash{0x01}]
				assert.True(t, tipExists, "Tip should not be pruned")

				assert.Len(t, cm.byHash, 2, "Should have 2 headers (tip + recent orphan)")
			},
		},
		{
			name: "PreservesMainChainHeaders",
			setupCM: func() *ChainManager {
				cm := &ChainManager{
					byHash:   make(map[chainhash.Hash]*BlockHeader),
					byHeight: make([]chainhash.Hash, 201),
				}

				// Create tip at height 200
				tipHash := chainhash.Hash{0x01}
				tip := &BlockHeader{
					Header: &block.Header{},
					Height: 200,
					Hash:   tipHash,
				}
				cm.tip = tip
				cm.byHash[tipHash] = tip
				cm.byHeight[200] = tipHash

				// Create main chain header at height 50 (should NOT be pruned)
				mainChainHash := chainhash.Hash{0x02}
				mainChain := &BlockHeader{
					Header: &block.Header{},
					Height: 50,
					Hash:   mainChainHash,
				}
				cm.byHash[mainChainHash] = mainChain
				cm.byHeight[50] = mainChainHash

				// Create orphan at height 50 with different hash (should be pruned)
				orphanHash := chainhash.Hash{0x03}
				orphan := &BlockHeader{
					Header: &block.Header{},
					Height: 50,
					Hash:   orphanHash,
				}
				cm.byHash[orphanHash] = orphan

				return cm
			},
			verifyFunc: func(t *testing.T, cm *ChainManager) {
				t.Helper()
				// Main chain header should be preserved
				_, mainExists := cm.byHash[chainhash.Hash{0x02}]
				assert.True(t, mainExists, "Main chain header should be preserved")

				// Orphan should be pruned
				_, orphanExists := cm.byHash[chainhash.Hash{0x03}]
				assert.False(t, orphanExists, "Orphan should be pruned")

				assert.Len(t, cm.byHash, 2, "Should have 2 headers (tip + main chain header)")
			},
		},
		{
			name: "HandlesIntegerOverflowProtection",
			setupCM: func() *ChainManager {
				cm := &ChainManager{
					byHash:   make(map[chainhash.Hash]*BlockHeader),
					byHeight: make([]chainhash.Hash, 0),
				}

				// Create tip at height 200
				tipHash := chainhash.Hash{0x01}
				tip := &BlockHeader{
					Header: &block.Header{},
					Height: 200,
					Hash:   tipHash,
				}
				cm.tip = tip
				cm.byHash[tipHash] = tip

				// Create orphan that would trigger overflow check
				// chainLen (0) <= 0xFFFFFFFF is true, but header.Height (50) < uint32(chainLen) (0) is false
				orphanHash := chainhash.Hash{0x02}
				orphan := &BlockHeader{
					Header: &block.Header{},
					Height: 50,
					Hash:   orphanHash,
				}
				cm.byHash[orphanHash] = orphan

				return cm
			},
			verifyFunc: func(t *testing.T, cm *ChainManager) {
				t.Helper()
				// Orphan should be pruned (height 50 < pruneHeight 100)
				_, exists := cm.byHash[chainhash.Hash{0x02}]
				assert.False(t, exists, "Orphan should be pruned")
				assert.Len(t, cm.byHash, 1, "Should have 1 header (tip only)")
			},
		},
		{
			name: "PrunesMultipleOldOrphans",
			setupCM: func() *ChainManager {
				cm := &ChainManager{
					byHash:   make(map[chainhash.Hash]*BlockHeader),
					byHeight: make([]chainhash.Hash, 301),
				}

				// Create tip at height 300
				tipHash := chainhash.Hash{0x01, 0x00}
				tip := &BlockHeader{
					Header: &block.Header{},
					Height: 300,
					Hash:   tipHash,
				}
				cm.tip = tip
				cm.byHash[tipHash] = tip
				cm.byHeight[300] = tipHash

				// Create multiple old orphans (all should be pruned)
				// Use two bytes to avoid collision
				for i := uint32(50); i < 100; i++ {
					hash := chainhash.Hash{0x02, byte(i)}
					orphan := &BlockHeader{
						Header: &block.Header{},
						Height: i,
						Hash:   hash,
					}
					cm.byHash[hash] = orphan
				}

				// Create multiple recent orphans (none should be pruned)
				for i := uint32(250); i < 260; i++ {
					hash := chainhash.Hash{0x03, byte(i)}
					orphan := &BlockHeader{
						Header: &block.Header{},
						Height: i,
						Hash:   hash,
					}
					cm.byHash[hash] = orphan
				}

				return cm
			},
			verifyFunc: func(t *testing.T, cm *ChainManager) {
				t.Helper()
				// Should have tip + 10 recent orphans = 11
				assert.Len(t, cm.byHash, 11, "Should have 11 headers (tip + 10 recent orphans)")

				// Verify old orphans are gone
				for i := uint32(50); i < 100; i++ {
					hash := chainhash.Hash{0x02, byte(i)}
					_, exists := cm.byHash[hash]
					assert.False(t, exists, "Old orphan at height %d should be pruned", i)
				}

				// Verify recent orphans remain
				for i := uint32(250); i < 260; i++ {
					hash := chainhash.Hash{0x03, byte(i)}
					_, exists := cm.byHash[hash]
					assert.True(t, exists, "Recent orphan at height %d should be preserved", i)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := tt.setupCM()
			cm.pruneOrphans()
			tt.verifyFunc(t, cm)
		})
	}
}
