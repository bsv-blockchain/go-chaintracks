package chainmanager

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/bsv-blockchain/go-sdk/block"
	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
)

func TestChainManagerGetTip(t *testing.T) {
	tests := []struct {
		name     string
		setupCM  func() *ChainManager
		expected *chaintracks.BlockHeader
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
				header := &chaintracks.BlockHeader{
					Header: &block.Header{},
					Height: 12345,
					Hash:   hash,
				}
				return &ChainManager{
					tip: header,
				}
			},
			expected: &chaintracks.BlockHeader{
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
					tip: &chaintracks.BlockHeader{
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
					tip: &chaintracks.BlockHeader{
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
					tip: &chaintracks.BlockHeader{
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

	header1 := &chaintracks.BlockHeader{
		Header: &block.Header{},
		Height: 0,
		Hash:   hash1,
	}
	header2 := &chaintracks.BlockHeader{
		Header: &block.Header{},
		Height: 1,
		Hash:   hash2,
	}
	header3 := &chaintracks.BlockHeader{
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
					byHash: map[chainhash.Hash]*chaintracks.BlockHeader{
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
					byHash: map[chainhash.Hash]*chaintracks.BlockHeader{
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
					byHash: map[chainhash.Hash]*chaintracks.BlockHeader{
						hash1: header1,
						hash2: header2,
					},
				}
			},
			height:        10,
			expectedError: chaintracks.ErrHeaderNotFound,
		},
		{
			name: "ReturnsErrorWhenChainIsEmpty",
			setupCM: func() *ChainManager {
				return &ChainManager{
					byHeight: []chainhash.Hash{},
					byHash:   map[chainhash.Hash]*chaintracks.BlockHeader{},
				}
			},
			height:        0,
			expectedError: chaintracks.ErrHeaderNotFound,
		},
		{
			name: "ReturnsErrorWhenHashNotInByHash",
			setupCM: func() *ChainManager {
				return &ChainManager{
					byHeight: []chainhash.Hash{hash1},
					byHash:   map[chainhash.Hash]*chaintracks.BlockHeader{},
				}
			},
			height:        0,
			expectedError: chaintracks.ErrHeaderNotFound,
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

	header1 := &chaintracks.BlockHeader{
		Header: &block.Header{},
		Height: 0,
		Hash:   hash1,
	}
	header2 := &chaintracks.BlockHeader{
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
					byHash: map[chainhash.Hash]*chaintracks.BlockHeader{
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
					byHash: map[chainhash.Hash]*chaintracks.BlockHeader{
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
					byHash: map[chainhash.Hash]*chaintracks.BlockHeader{
						hash1: header1,
						hash2: header2,
					},
				}
			},
			hash:          &hashNotFound,
			expectedError: chaintracks.ErrHeaderNotFound,
		},
		{
			name: "ReturnsErrorWhenByHashIsEmpty",
			setupCM: func() *ChainManager {
				return &ChainManager{
					byHash: map[chainhash.Hash]*chaintracks.BlockHeader{},
				}
			},
			hash:          &hash3,
			expectedError: chaintracks.ErrHeaderNotFound,
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
		headerToAdd *chaintracks.BlockHeader
		verifyFunc  func(*testing.T, *ChainManager)
	}{
		{
			name: "AddsHeaderToEmptyChainManager",
			setupCM: func() *ChainManager {
				return &ChainManager{
					byHash: make(map[chainhash.Hash]*chaintracks.BlockHeader),
				}
			},
			headerToAdd: &chaintracks.BlockHeader{
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
					byHash: map[chainhash.Hash]*chaintracks.BlockHeader{
						existingHash: {
							Header: &block.Header{},
							Height: 0,
							Hash:   existingHash,
						},
					},
				}
			},
			headerToAdd: &chaintracks.BlockHeader{
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
					byHash: map[chainhash.Hash]*chaintracks.BlockHeader{
						hash: {
							Header: &block.Header{},
							Height: 0,
							Hash:   hash,
						},
					},
				}
			},
			headerToAdd: &chaintracks.BlockHeader{
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
					byHash:   make(map[chainhash.Hash]*chaintracks.BlockHeader),
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
					byHash:   make(map[chainhash.Hash]*chaintracks.BlockHeader),
					byHeight: make([]chainhash.Hash, 51),
				}

				// Create tip at height 50
				tipHash := chainhash.Hash{0x01}
				tip := &chaintracks.BlockHeader{
					Header: &block.Header{},
					Height: 50,
					Hash:   tipHash,
				}
				cm.tip = tip
				cm.byHash[tipHash] = tip
				cm.byHeight[50] = tipHash

				// Create an orphan at height 10 (should NOT be pruned)
				orphanHash := chainhash.Hash{0x02}
				orphan := &chaintracks.BlockHeader{
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
					byHash:   make(map[chainhash.Hash]*chaintracks.BlockHeader),
					byHeight: make([]chainhash.Hash, 201),
				}

				// Create tip at height 200
				tipHash := chainhash.Hash{0x01}
				tip := &chaintracks.BlockHeader{
					Header: &block.Header{},
					Height: 200,
					Hash:   tipHash,
				}
				cm.tip = tip
				cm.byHash[tipHash] = tip
				cm.byHeight[200] = tipHash

				// Create old orphan at height 50 (should be pruned, 200-100=100)
				oldOrphanHash := chainhash.Hash{0x02}
				oldOrphan := &chaintracks.BlockHeader{
					Header: &block.Header{},
					Height: 50,
					Hash:   oldOrphanHash,
				}
				cm.byHash[oldOrphanHash] = oldOrphan

				// Create recent orphan at height 150 (should NOT be pruned)
				recentOrphanHash := chainhash.Hash{0x03}
				recentOrphan := &chaintracks.BlockHeader{
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
					byHash:   make(map[chainhash.Hash]*chaintracks.BlockHeader),
					byHeight: make([]chainhash.Hash, 201),
				}

				// Create tip at height 200
				tipHash := chainhash.Hash{0x01}
				tip := &chaintracks.BlockHeader{
					Header: &block.Header{},
					Height: 200,
					Hash:   tipHash,
				}
				cm.tip = tip
				cm.byHash[tipHash] = tip
				cm.byHeight[200] = tipHash

				// Create main chain header at height 50 (should NOT be pruned)
				mainChainHash := chainhash.Hash{0x02}
				mainChain := &chaintracks.BlockHeader{
					Header: &block.Header{},
					Height: 50,
					Hash:   mainChainHash,
				}
				cm.byHash[mainChainHash] = mainChain
				cm.byHeight[50] = mainChainHash

				// Create orphan at height 50 with different hash (should be pruned)
				orphanHash := chainhash.Hash{0x03}
				orphan := &chaintracks.BlockHeader{
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
					byHash:   make(map[chainhash.Hash]*chaintracks.BlockHeader),
					byHeight: make([]chainhash.Hash, 0),
				}

				// Create tip at height 200
				tipHash := chainhash.Hash{0x01}
				tip := &chaintracks.BlockHeader{
					Header: &block.Header{},
					Height: 200,
					Hash:   tipHash,
				}
				cm.tip = tip
				cm.byHash[tipHash] = tip

				// Create orphan that would trigger overflow check
				// chainLen (0) <= 0xFFFFFFFF is true, but header.Height (50) < uint32(chainLen) (0) is false
				orphanHash := chainhash.Hash{0x02}
				orphan := &chaintracks.BlockHeader{
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
					byHash:   make(map[chainhash.Hash]*chaintracks.BlockHeader),
					byHeight: make([]chainhash.Hash, 301),
				}

				// Create tip at height 300
				tipHash := chainhash.Hash{0x01, 0x00}
				tip := &chaintracks.BlockHeader{
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
					orphan := &chaintracks.BlockHeader{
						Header: &block.Header{},
						Height: i,
						Hash:   hash,
					}
					cm.byHash[hash] = orphan
				}

				// Create multiple recent orphans (none should be pruned)
				for i := uint32(250); i < 260; i++ {
					hash := chainhash.Hash{0x03, byte(i)} //nolint:gosec // i is bounded by loop range, safe for byte
					orphan := &chaintracks.BlockHeader{
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
					hash := chainhash.Hash{0x03, byte(i)} //nolint:gosec // i is bounded by loop range, safe for byte
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

func TestSetChainTipWithReorg(t *testing.T) {
	// Create test hashes
	hash1 := chainhash.Hash{1}
	hash2 := chainhash.Hash{2}
	hash3 := chainhash.Hash{3}
	orphanHash1 := chainhash.Hash{0xA1}
	orphanHash2 := chainhash.Hash{0xA2}

	tests := []struct {
		name           string
		setupCM        func() *ChainManager
		branchHeaders  []*chaintracks.BlockHeader
		commonAncestor *chaintracks.BlockHeader
		orphanedHashes []chainhash.Hash
		expectedError  error
		verifyFunc     func(t *testing.T, cm *ChainManager, reorgChan chan *chaintracks.ReorgEvent)
	}{
		{
			name: "PublishesReorgEventToChannel",
			setupCM: func() *ChainManager {
				return &ChainManager{
					byHash:       make(map[chainhash.Hash]*chaintracks.BlockHeader),
					byHeight:     make([]chainhash.Hash, 0),
					reorgMsgChan: make(chan *chaintracks.ReorgEvent, 1),
				}
			},
			branchHeaders: []*chaintracks.BlockHeader{
				{Header: &block.Header{}, Height: 101, Hash: hash2},
				{Header: &block.Header{}, Height: 102, Hash: hash3},
			},
			commonAncestor: &chaintracks.BlockHeader{Header: &block.Header{}, Height: 100, Hash: hash1},
			orphanedHashes: []chainhash.Hash{orphanHash1, orphanHash2},
			expectedError:  nil,
			verifyFunc: func(t *testing.T, _ *ChainManager, reorgChan chan *chaintracks.ReorgEvent) {
				select {
				case event := <-reorgChan:
					require.NotNil(t, event)
					assert.Equal(t, uint32(2), event.Depth)
					assert.Len(t, event.OrphanedHashes, 2)
					assert.Equal(t, orphanHash1, event.OrphanedHashes[0])
					assert.Equal(t, orphanHash2, event.OrphanedHashes[1])
					assert.Equal(t, hash3, event.NewTip.Hash) // last header in branch
					assert.Equal(t, hash1, event.CommonAncestor.Hash)
				default:
					t.Fatal("Expected reorg event on channel")
				}
			},
		},
		{
			name: "HandlesNilReorgChannel",
			setupCM: func() *ChainManager {
				return &ChainManager{
					byHash:       make(map[chainhash.Hash]*chaintracks.BlockHeader),
					byHeight:     make([]chainhash.Hash, 0),
					reorgMsgChan: nil,
				}
			},
			branchHeaders: []*chaintracks.BlockHeader{
				{Header: &block.Header{}, Height: 101, Hash: hash2},
			},
			commonAncestor: &chaintracks.BlockHeader{Header: &block.Header{}, Height: 100, Hash: hash1},
			orphanedHashes: []chainhash.Hash{orphanHash1},
			expectedError:  nil,
			verifyFunc: func(t *testing.T, cm *ChainManager, _ chan *chaintracks.ReorgEvent) {
				// since channel is nil, we only need to assert that tip was set
				assert.Equal(t, hash2, cm.tip.Hash)
			},
		},
		{
			name: "SkipsWhenChannelFull",
			setupCM: func() *ChainManager {
				ch := make(chan *chaintracks.ReorgEvent, 1)
				ch <- &chaintracks.ReorgEvent{Depth: 999}
				return &ChainManager{
					byHash:       make(map[chainhash.Hash]*chaintracks.BlockHeader),
					byHeight:     make([]chainhash.Hash, 0),
					reorgMsgChan: ch,
				}
			},
			branchHeaders: []*chaintracks.BlockHeader{
				{Header: &block.Header{}, Height: 101, Hash: hash2},
			},
			commonAncestor: &chaintracks.BlockHeader{Header: &block.Header{}, Height: 100, Hash: hash1},
			orphanedHashes: []chainhash.Hash{orphanHash1},
			expectedError:  nil,
			verifyFunc: func(t *testing.T, _ *ChainManager, reorgChan chan *chaintracks.ReorgEvent) {
				event := <-reorgChan
				assert.Equal(t, uint32(999), event.Depth) // The event.Depth should be the old one
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := tt.setupCM()
			reorgChan := cm.reorgMsgChan

			err := cm.SetChainTipWithReorg(t.Context(), tt.branchHeaders, tt.commonAncestor, tt.orphanedHashes)

			if tt.expectedError != nil {
				require.ErrorIs(t, err, tt.expectedError)
			} else {
				require.NoError(t, err)
			}

			if tt.verifyFunc != nil {
				tt.verifyFunc(t, cm, reorgChan)
			}
		})
	}
}

func TestSubscribeReorg_MultipleSubscribersAllowed(t *testing.T) {
	// Create test hashes
	hash1 := chainhash.Hash{1}
	hash2 := chainhash.Hash{2}
	hash3 := chainhash.Hash{3}
	orphanHash1 := chainhash.Hash{0xA1}
	orphanHash2 := chainhash.Hash{0xA2}

	// setup chain manager and reorg data
	cm := &ChainManager{
		byHash:           make(map[chainhash.Hash]*chaintracks.BlockHeader),
		byHeight:         make([]chainhash.Hash, 0),
		reorgMsgChan:     make(chan *chaintracks.ReorgEvent, 1),
		reorgSubscribers: make(map[chan *chaintracks.ReorgEvent]struct{}),
	}
	branchHeaders := []*chaintracks.BlockHeader{
		{Header: &block.Header{}, Height: 101, Hash: hash2},
		{Header: &block.Header{}, Height: 102, Hash: hash3},
	}
	commonAncestor := &chaintracks.BlockHeader{Header: &block.Header{}, Height: 100, Hash: hash1}
	orphanedHashes := []chainhash.Hash{orphanHash1, orphanHash2}

	// create subscribers
	ch1 := cm.SubscribeReorg(t.Context())
	ch2 := cm.SubscribeReorg(t.Context())

	assert.Len(t, cm.reorgSubscribers, 2)
	assert.NotEqual(t, ch1, ch2)
	// reorg event
	err := cm.SetChainTipWithReorg(t.Context(), branchHeaders, commonAncestor, orphanedHashes)
	require.NoError(t, err)
}

func TestSubscribeReorg_AutoUnsubscribesOnContextCancel(t *testing.T) {
	cm := &ChainManager{
		reorgSubscribers: make(map[chan *chaintracks.ReorgEvent]struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch := cm.SubscribeReorg(ctx)

	// Verify subscribed
	cm.reorgSubMu.RLock()
	subscriberCount := len(cm.reorgSubscribers)
	cm.reorgSubMu.RUnlock()
	assert.Equal(t, 1, subscriberCount)

	// Cancel context
	cancel()

	// Wait for the goroutine to unsubscribe (give it a moment)
	require.Eventually(t, func() bool {
		cm.reorgSubMu.RLock()
		defer cm.reorgSubMu.RUnlock()
		return len(cm.reorgSubscribers) == 0
	}, time.Second, 10*time.Millisecond, "Subscriber should be removed after context cancel")

	// Channel should be closed
	_, ok := <-ch
	assert.False(t, ok, "Channel should be closed")
}

func TestUnsubscribeReorg_AutoUnsubscribesOnContextCancel(t *testing.T) {
	tests := []struct {
		name       string
		setupCM    func() (*ChainManager, chan *chaintracks.ReorgEvent)
		verifyFunc func(t *testing.T, cm *ChainManager, ch chan *chaintracks.ReorgEvent)
	}{
		{
			name: "RemovesSubscriberAndClosesChannel",
			setupCM: func() (*ChainManager, chan *chaintracks.ReorgEvent) {
				ch := make(chan *chaintracks.ReorgEvent, 1)
				cm := &ChainManager{
					reorgSubscribers: map[chan *chaintracks.ReorgEvent]struct{}{
						ch: {},
					},
				}

				return cm, ch
			},
			verifyFunc: func(t *testing.T, cm *ChainManager, ch chan *chaintracks.ReorgEvent) {
				assert.Empty(t, cm.reorgSubscribers, "Subscriber should be removed")

				_, ok := <-ch
				assert.False(t, ok, "Channel should be closed")
			},
		},
		{
			name: "RemovesOnlySpecifiedSubscriberAndClosesChannel",
			setupCM: func() (*ChainManager, chan *chaintracks.ReorgEvent) {
				ch1 := make(chan *chaintracks.ReorgEvent, 1)
				ch2 := make(chan *chaintracks.ReorgEvent, 1)
				cm := &ChainManager{
					reorgSubscribers: map[chan *chaintracks.ReorgEvent]struct{}{
						ch1: {},
						ch2: {},
					},
				}

				return cm, ch1
			},
			verifyFunc: func(t *testing.T, cm *ChainManager, ch chan *chaintracks.ReorgEvent) {
				assert.Len(t, cm.reorgSubscribers, 1, "Only one subscriber should be removed")

				_, ok := <-ch
				assert.False(t, ok, "Channel should be closed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm, ch := tt.setupCM()

			cm.UnsubscribeReorg(ch)

			tt.verifyFunc(t, cm, ch)
		})
	}
}

func TestReorgBroadcast(t *testing.T) {
	tests := []struct {
		name       string
		setupCM    func() (*ChainManager, []chan *chaintracks.ReorgEvent)
		event      *chaintracks.ReorgEvent
		verifyFunc func(t *testing.T, channels []chan *chaintracks.ReorgEvent, event *chaintracks.ReorgEvent)
	}{
		{
			name: "BroadcastsToAllSubscribers",
			setupCM: func() (*ChainManager, []chan *chaintracks.ReorgEvent) {
				ch1 := make(chan *chaintracks.ReorgEvent, 1)
				ch2 := make(chan *chaintracks.ReorgEvent, 1)
				ch3 := make(chan *chaintracks.ReorgEvent, 1)
				cm := &ChainManager{
					reorgSubscribers: map[chan *chaintracks.ReorgEvent]struct{}{
						ch1: {},
						ch2: {},
						ch3: {},
					},
				}

				return cm, []chan *chaintracks.ReorgEvent{ch1, ch2, ch3}
			},
			event: &chaintracks.ReorgEvent{
				Depth:          3,
				OrphanedHashes: []chainhash.Hash{{0x01}, {0x02}, {0x03}},
				CommonAncestor: &chaintracks.BlockHeader{Height: 100},
				NewTip:         &chaintracks.BlockHeader{Height: 100},
			},
			verifyFunc: func(t *testing.T, channels []chan *chaintracks.ReorgEvent, event *chaintracks.ReorgEvent) {
				for i, ch := range channels {
					select {
					case received := <-ch:
						assert.Equal(t, event.Depth, received.Depth, "Channel %d: Depth mismatch", i)
						assert.Equal(t, event.OrphanedHashes, received.OrphanedHashes, "Channel %d: OrphanedHashes mismatch", i)
						assert.Equal(t, event.CommonAncestor.Height, received.CommonAncestor.Height, "Channel %d: CommonAncestor mismatch", i)
						assert.Equal(t, event.NewTip.Height, received.NewTip.Height, "Channel %d: NewTip mismatch", i)
					default:
						t.Errorf("Channel %d: expected event but got none", i)
					}
				}
			},
		},
		{
			name: "HandlesNoSubscribers",
			setupCM: func() (*ChainManager, []chan *chaintracks.ReorgEvent) {
				cm := &ChainManager{
					reorgSubscribers: make(map[chan *chaintracks.ReorgEvent]struct{}),
				}
				return cm, nil
			},
			event: &chaintracks.ReorgEvent{Depth: 1},
			verifyFunc: func(_ *testing.T, _ []chan *chaintracks.ReorgEvent, _ *chaintracks.ReorgEvent) {
				// Should not panic - nothing to verify
			},
		},
	}

	for _, tt := range tests {
		cm, channels := tt.setupCM()

		cm.reorgBroadcast(tt.event)

		tt.verifyFunc(t, channels, tt.event)
	}
}

func TestReorgFanOut_BroadcastEventsFromChannelToAllSubscribers(t *testing.T) {
	ch1 := make(chan *chaintracks.ReorgEvent, 1)
	ch2 := make(chan *chaintracks.ReorgEvent, 1)
	cm := &ChainManager{
		reorgMsgChan: make(chan *chaintracks.ReorgEvent, 1),
		reorgSubscribers: map[chan *chaintracks.ReorgEvent]struct{}{
			ch1: {},
			ch2: {},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Start the fan-out goroutine
	go cm.reorgFanOut(ctx)
	// Send event to the internal channel
	event := &chaintracks.ReorgEvent{
		Depth:          2,
		OrphanedHashes: []chainhash.Hash{{0xAA}, {0xBB}},
	}
	cm.reorgMsgChan <- event
	// Both subscribers should receive it
	require.Eventually(t, func() bool {
		select {
		case received := <-ch1:
			return received.Depth == 2
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond, "ch1 should receive event")
	require.Eventually(t, func() bool {
		select {
		case received := <-ch2:
			return received.Depth == 2
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond, "ch2 should receive event")
}

func TestIsOnMainChain(t *testing.T) {
	hashMain := chainhash.Hash{0x01}
	hashOrphan := chainhash.Hash{0x02}

	mainHeader := &chaintracks.BlockHeader{Header: &block.Header{}, Height: 5, Hash: hashMain}
	orphanHeader := &chaintracks.BlockHeader{Header: &block.Header{}, Height: 5, Hash: hashOrphan}

	cm := &ChainManager{
		byHeight: make([]chainhash.Hash, 6),
		byHash: map[chainhash.Hash]*chaintracks.BlockHeader{
			hashMain:   mainHeader,
			hashOrphan: orphanHeader,
		},
	}
	cm.byHeight[5] = hashMain

	assert.True(t, cm.isOnMainChain(t.Context(), mainHeader), "main chain block should return true")
	assert.False(t, cm.isOnMainChain(t.Context(), orphanHeader), "orphan block should return false")
}

// TestReorgOrphanAtSameHeight reproduces the bug where a competing block at the
// same height as the tip is stored as an orphan in byHash, then when the next
// block arrives building on the orphan, the orphan's height in byHeight is never
// updated because the common ancestor search finds the orphan and treats it as
// the fork point (commonAncestor.Height == oldTip.Height, so reorg is skipped).
//
// The fix ensures that only main-chain blocks are treated as common ancestors
// during sync, so orphan blocks at the same height as the tip trigger a proper
// reorg back to the real common ancestor (the parent of both competing blocks).
func TestReorgOrphanAtSameHeight(t *testing.T) {
	ctx := t.Context()
	tmpDir := t.TempDir()

	cm, err := NewForTesting(ctx, "test", tmpDir)
	require.NoError(t, err)

	// Build a chain: genesis(0) → block1(1) → block2(2) → blockA(3)
	// blockA is the first block at height 3 (becomes tip)
	genesis := &chaintracks.BlockHeader{
		Header:    &block.Header{Version: 1, Bits: 0x1d00ffff},
		Height:    0,
		Hash:      chainhash.Hash{0x10},
		ChainWork: CalculateWork(0x1d00ffff),
	}
	block1 := &chaintracks.BlockHeader{
		Header:    &block.Header{Version: 1, Bits: 0x1d00ffff, PrevHash: genesis.Hash},
		Height:    1,
		Hash:      chainhash.Hash{0x11},
		ChainWork: new(big.Int).Mul(CalculateWork(0x1d00ffff), big.NewInt(2)),
	}
	block2 := &chaintracks.BlockHeader{
		Header:    &block.Header{Version: 1, Bits: 0x1d00ffff, PrevHash: block1.Hash},
		Height:    2,
		Hash:      chainhash.Hash{0x12},
		ChainWork: new(big.Int).Mul(CalculateWork(0x1d00ffff), big.NewInt(3)),
	}
	blockA := &chaintracks.BlockHeader{
		Header:    &block.Header{Version: 1, Bits: 0x1d00ffff, PrevHash: block2.Hash},
		Height:    3,
		Hash:      chainhash.Hash{0xA0}, // Stale block
		ChainWork: new(big.Int).Mul(CalculateWork(0x1d00ffff), big.NewInt(4)),
	}

	err = cm.SetChainTip(ctx, []*chaintracks.BlockHeader{genesis, block1, block2, blockA})
	require.NoError(t, err)
	assert.Equal(t, uint32(3), cm.GetHeight(ctx))
	assert.Equal(t, blockA.Hash, cm.GetTip(ctx).Hash)

	// Competing blockB at height 3 (same parent as blockA, same chainwork)
	// Added to byHash as orphan — NOT in byHeight
	blockB := &chaintracks.BlockHeader{
		Header:    &block.Header{Version: 2, Bits: 0x1d00ffff, PrevHash: block2.Hash},
		Height:    3,
		Hash:      chainhash.Hash{0xB0}, // Canonical block
		ChainWork: new(big.Int).Mul(CalculateWork(0x1d00ffff), big.NewInt(4)),
	}
	err = cm.AddHeader(blockB)
	require.NoError(t, err)

	// Verify blockB is an orphan (in byHash but not in byHeight)
	assert.False(t, cm.isOnMainChain(ctx, blockB))
	assert.True(t, cm.isOnMainChain(ctx, blockA))

	// Now block4 arrives building on blockB (the orphan).
	// This simulates what SyncFromRemoteTip does AFTER the fix:
	// it finds the real common ancestor (block2 at height 2) and includes
	// blockB in the branch, triggering a proper reorg.
	block4 := &chaintracks.BlockHeader{
		Header:    &block.Header{Version: 1, Bits: 0x1d00ffff, PrevHash: blockB.Hash},
		Height:    4,
		Hash:      chainhash.Hash{0x14},
		ChainWork: new(big.Int).Mul(CalculateWork(0x1d00ffff), big.NewInt(5)),
	}

	// The branch includes blockB (replacing blockA at height 3) and block4
	err = cm.SetChainTipWithReorg(ctx, []*chaintracks.BlockHeader{blockB, block4}, block2, []chainhash.Hash{blockA.Hash})
	require.NoError(t, err)

	// Verify: tip should be block4
	assert.Equal(t, uint32(4), cm.GetHeight(ctx))
	assert.Equal(t, block4.Hash, cm.GetTip(ctx).Hash)

	// Critical check: byHeight[3] should now be blockB, NOT blockA
	headerAt3, err := cm.GetHeaderByHeight(ctx, 3)
	require.NoError(t, err)
	assert.Equal(t, blockB.Hash, headerAt3.Hash, "byHeight[3] should be blockB (canonical), not blockA (stale)")

	// Verify blockB is now on main chain
	assert.True(t, cm.isOnMainChain(ctx, blockB))
	assert.False(t, cm.isOnMainChain(ctx, blockA))
}
