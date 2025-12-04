package chaintracks

import (
	"testing"

	"github.com/bsv-blockchain/go-sdk/block"
	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChainManagerIsValidRootForHeight(t *testing.T) {
	// Create test merkle roots
	validRoot := chainhash.Hash{1, 2, 3, 4, 5}
	invalidRoot := chainhash.Hash{9, 9, 9, 9, 9}
	hash1 := chainhash.Hash{1}
	hash2 := chainhash.Hash{2}

	tests := []struct {
		name          string
		setupCM       func() *ChainManager
		root          *chainhash.Hash
		height        uint32
		expectedValid bool
		expectedError error
	}{
		{
			name: "ReturnsTrueForValidMerkleRoot",
			setupCM: func() *ChainManager {
				header1 := &BlockHeader{
					Header: &block.Header{
						MerkleRoot: validRoot,
					},
					Height: 100,
					Hash:   hash1,
				}
				return &ChainManager{
					byHeight: []chainhash.Hash{hash1},
					byHash: map[chainhash.Hash]*BlockHeader{
						hash1: header1,
					},
				}
			},
			root:          &validRoot,
			height:        0,
			expectedValid: true,
			expectedError: nil,
		},
		{
			name: "ReturnsFalseForInvalidMerkleRoot",
			setupCM: func() *ChainManager {
				header1 := &BlockHeader{
					Header: &block.Header{
						MerkleRoot: validRoot,
					},
					Height: 100,
					Hash:   hash1,
				}
				return &ChainManager{
					byHeight: []chainhash.Hash{hash1},
					byHash: map[chainhash.Hash]*BlockHeader{
						hash1: header1,
					},
				}
			},
			root:          &invalidRoot,
			height:        0,
			expectedValid: false,
			expectedError: nil,
		},
		{
			name: "ReturnsErrorWhenHeaderNotFound",
			setupCM: func() *ChainManager {
				return &ChainManager{
					byHeight: []chainhash.Hash{},
					byHash:   map[chainhash.Hash]*BlockHeader{},
				}
			},
			root:          &validRoot,
			height:        0,
			expectedValid: false,
			expectedError: ErrHeaderNotFound,
		},
		{
			name: "ReturnsErrorWhenHeightOutOfRange",
			setupCM: func() *ChainManager {
				header1 := &BlockHeader{
					Header: &block.Header{
						MerkleRoot: validRoot,
					},
					Height: 0,
					Hash:   hash1,
				}
				return &ChainManager{
					byHeight: []chainhash.Hash{hash1},
					byHash: map[chainhash.Hash]*BlockHeader{
						hash1: header1,
					},
				}
			},
			root:          &validRoot,
			height:        100,
			expectedValid: false,
			expectedError: ErrHeaderNotFound,
		},
		{
			name: "ReturnsTrueForMultipleHeadersWithValidRoot",
			setupCM: func() *ChainManager {
				header1 := &BlockHeader{
					Header: &block.Header{
						MerkleRoot: validRoot,
					},
					Height: 0,
					Hash:   hash1,
				}
				header2 := &BlockHeader{
					Header: &block.Header{
						MerkleRoot: invalidRoot,
					},
					Height: 1,
					Hash:   hash2,
				}
				return &ChainManager{
					byHeight: []chainhash.Hash{hash1, hash2},
					byHash: map[chainhash.Hash]*BlockHeader{
						hash1: header1,
						hash2: header2,
					},
				}
			},
			root:          &validRoot,
			height:        0,
			expectedValid: true,
			expectedError: nil,
		},
		{
			name: "ReturnsFalseForSecondHeaderWithWrongRoot",
			setupCM: func() *ChainManager {
				header1 := &BlockHeader{
					Header: &block.Header{
						MerkleRoot: validRoot,
					},
					Height: 0,
					Hash:   hash1,
				}
				header2 := &BlockHeader{
					Header: &block.Header{
						MerkleRoot: invalidRoot,
					},
					Height: 1,
					Hash:   hash2,
				}
				return &ChainManager{
					byHeight: []chainhash.Hash{hash1, hash2},
					byHash: map[chainhash.Hash]*BlockHeader{
						hash1: header1,
						hash2: header2,
					},
				}
			},
			root:          &validRoot,
			height:        1,
			expectedValid: false,
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := tt.setupCM()

			valid, err := cm.IsValidRootForHeight(t.Context(), tt.root, tt.height)

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
				assert.False(t, valid)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedValid, valid)
			}
		})
	}
}

func TestChainManagerCurrentHeight(t *testing.T) {
	tests := []struct {
		name           string
		setupCM        func() *ChainManager
		expectedHeight uint32
		expectedError  error
	}{
		{
			name: "ReturnsZeroWhenTipIsNil",
			setupCM: func() *ChainManager {
				return &ChainManager{
					tip: nil,
				}
			},
			expectedHeight: 0,
			expectedError:  nil,
		},
		{
			name: "ReturnsCorrectHeightWhenTipExists",
			setupCM: func() *ChainManager {
				return &ChainManager{
					tip: &BlockHeader{
						Header: &block.Header{},
						Height: 12345,
					},
				}
			},
			expectedHeight: 12345,
			expectedError:  nil,
		},
		{
			name: "ReturnsZeroForGenesisBlock",
			setupCM: func() *ChainManager {
				return &ChainManager{
					tip: &BlockHeader{
						Header: &block.Header{},
						Height: 0,
					},
				}
			},
			expectedHeight: 0,
			expectedError:  nil,
		},
		{
			name: "ReturnsHighBlockHeight",
			setupCM: func() *ChainManager {
				return &ChainManager{
					tip: &BlockHeader{
						Header: &block.Header{},
						Height: 800000,
					},
				}
			},
			expectedHeight: 800000,
			expectedError:  nil,
		},
		{
			name: "ReturnsMaxUint32Height",
			setupCM: func() *ChainManager {
				return &ChainManager{
					tip: &BlockHeader{
						Header: &block.Header{},
						Height: 4294967295, // Max uint32
					},
				}
			},
			expectedHeight: 4294967295,
			expectedError:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := tt.setupCM()

			height, err := cm.CurrentHeight(t.Context())

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedHeight, height)
			}
		})
	}
}
