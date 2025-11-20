package chaintracks

import (
	"context"

	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/bsv-blockchain/go-sdk/transaction/chaintracker"
)

// Chaintracks defines the interface for both embedded ChainManager and remote Client
// This allows applications to seamlessly switch between running chaintracks locally
// or connecting to a remote chaintracks server
type Chaintracks interface {
	// Embed the ChainTracker interface from go-sdk
	chaintracker.ChainTracker

	// Start begins the chaintracks service and returns a channel for block notifications
	Start(ctx context.Context) (<-chan *BlockHeader, error)

	// Stop gracefully shuts down the chaintracks service
	Stop() error

	// GetHeight returns the current blockchain height
	GetHeight() uint32

	// GetTip returns the current chain tip
	GetTip() *BlockHeader

	// GetHeaderByHeight retrieves a block header by its height
	GetHeaderByHeight(height uint32) (*BlockHeader, error)

	// GetHeaderByHash retrieves a block header by its hash
	GetHeaderByHash(hash *chainhash.Hash) (*BlockHeader, error)

	// GetNetwork returns the network name (mainnet, testnet, etc.)
	GetNetwork() (string, error)
}
