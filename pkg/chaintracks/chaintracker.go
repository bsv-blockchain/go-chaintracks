package chaintracks

import (
	"context"

	"github.com/bsv-blockchain/go-sdk/chainhash"
)

// IsValidRootForHeight implements the ChainTracker interface
// Validates that the given merkle root matches the header at the specified height
func (cm *ChainManager) IsValidRootForHeight(_ context.Context, root *chainhash.Hash, height uint32) (bool, error) {
	// Get the header at the given height
	header, err := cm.GetHeaderByHeight(height)
	if err != nil {
		return false, err
	}

	// Compare the merkle root
	return header.MerkleRoot.IsEqual(root), nil
}

// CurrentHeight implements the ChainTracker interface
// Returns the current height of the blockchain
func (cm *ChainManager) CurrentHeight(_ context.Context) (uint32, error) {
	return cm.GetHeight(), nil
}
