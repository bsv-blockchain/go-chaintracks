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
			result := cm.GetTip()

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
			result := cm.GetHeight()
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

			result, err := cm.GetNetwork()

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
			result, err := cm.GetHeaderByHeight(tt.height)

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
			result, err := cm.GetHeaderByHash(tt.hash)

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
