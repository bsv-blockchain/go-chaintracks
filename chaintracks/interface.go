package chaintracks

import (
	"context"

	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/bsv-blockchain/go-sdk/transaction/chaintracker"
)

// Chaintracks defines the interface for both embedded ChainManager and remote Client.
// Both implementations start their subscriptions automatically when created via config.Initialize().
// Cleanup happens automatically when the context passed to Initialize is cancelled.
type Chaintracks interface {
	// Embed the ChainTracker interface from go-sdk
	chaintracker.ChainTracker

	// GetHeight returns the current blockchain height
	GetHeight(ctx context.Context) uint32

	// GetTip returns the current chain tip
	GetTip(ctx context.Context) *BlockHeader

	// GetHeaderByHeight retrieves a block header by its height
	GetHeaderByHeight(ctx context.Context, height uint32) (*BlockHeader, error)

	// GetHeaderByHash retrieves a block header by its hash
	GetHeaderByHash(ctx context.Context, hash *chainhash.Hash) (*BlockHeader, error)

	// GetHeaders retrieves multiple headers starting from the given height
	GetHeaders(ctx context.Context, height, count uint32) ([]*BlockHeader, error)

	// GetNetwork returns the network name (mainnet, testnet, etc.)
	GetNetwork(ctx context.Context) (string, error)

	// Subscribe returns a channel that receives tip updates.
	// For Client: starts SSE connection on first subscriber, enables tip caching.
	// For ChainManager: returns a channel fed by the always-running P2P subscription.
	Subscribe(ctx context.Context) <-chan *BlockHeader

	// Unsubscribe removes a subscriber channel.
	// For Client: stops SSE and clears tip cache when last subscriber leaves.
	// For ChainManager: removes the channel from fan-out (P2P keeps running).
	Unsubscribe(ch <-chan *BlockHeader)
}
