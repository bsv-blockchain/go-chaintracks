// Package chainmanager provides blockchain header tracking and management functionality.
package chainmanager

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
	p2p "github.com/bsv-blockchain/go-teranode-p2p-client"
	"github.com/bsv-blockchain/go-sdk/chainhash"
)

// ChainManager is the main orchestrator for chain management.
type ChainManager struct {
	mu sync.RWMutex

	byHeight []chainhash.Hash                            // Main chain hashes indexed by height
	byHash   map[chainhash.Hash]*chaintracks.BlockHeader // Hash → Header (all headers: main + orphans)
	tip      *chaintracks.BlockHeader                    // Current chain tip

	localStoragePath string
	network          string

	// P2P fields
	P2PClient   *p2p.Client                            // P2P client for network communication
	msgChan     chan *chaintracks.BlockHeader          // Internal channel for tip updates
	subscribers map[chan *chaintracks.BlockHeader]struct{}
	subMu       sync.RWMutex
}

// New creates a new ChainManager, restores from local files, and starts the P2P subscription.
// The subscription will automatically stop when ctx is cancelled.
// If bootstrapURL is provided, it will sync from a remote teranode before returning.
func New(ctx context.Context, network, localStoragePath string, p2pClient *p2p.Client, bootstrapURL ...string) (*ChainManager, error) {
	if p2pClient == nil {
		return nil, fmt.Errorf("p2pClient is required")
	}

	cm := &ChainManager{
		byHeight:         make([]chainhash.Hash, 0, 1000000),
		byHash:           make(map[chainhash.Hash]*chaintracks.BlockHeader),
		network:          network,
		localStoragePath: localStoragePath,
		P2PClient:        p2pClient,
		msgChan:          make(chan *chaintracks.BlockHeader, 1),
		subscribers:      make(map[chan *chaintracks.BlockHeader]struct{}),
	}

	log.Printf("ChainManager initializing: network=%s, path=%s", network, localStoragePath)

	// Auto-restore from local files if they exist
	if err := cm.loadFromLocalFiles(ctx); err != nil {
		return nil, fmt.Errorf("failed to load checkpoint files: %w", err)
	}

	// Run bootstrap sync if configured (optional parameter)
	if len(bootstrapURL) > 0 && bootstrapURL[0] != "" {
		cm.runBootstrapSync(ctx, bootstrapURL[0])
	}

	// Start P2P block subscription
	cm.startBlockSubscription(ctx)

	// Start fan-out goroutine to broadcast tip updates to subscribers
	go cm.fanOut(ctx)

	return cm, nil
}

// NewForTesting creates a ChainManager without P2P for unit testing.
func NewForTesting(ctx context.Context, network, localStoragePath string) (*ChainManager, error) {
	cm := &ChainManager{
		byHeight:         make([]chainhash.Hash, 0, 1000000),
		byHash:           make(map[chainhash.Hash]*chaintracks.BlockHeader),
		network:          network,
		localStoragePath: localStoragePath,
		msgChan:          make(chan *chaintracks.BlockHeader, 1),
		subscribers:      make(map[chan *chaintracks.BlockHeader]struct{}),
	}

	if err := cm.loadFromLocalFiles(ctx); err != nil {
		return nil, fmt.Errorf("failed to load checkpoint files: %w", err)
	}

	go cm.fanOut(ctx)

	return cm, nil
}

// fanOut reads from msgChan and broadcasts to all subscribers.
func (cm *ChainManager) fanOut(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case header, ok := <-cm.msgChan:
			if !ok {
				return
			}
			cm.broadcast(header)
		}
	}
}

// runBootstrapSync performs initial sync from a bootstrap node.
func (cm *ChainManager) runBootstrapSync(ctx context.Context, url string) {
	log.Printf("Bootstrap URL configured: %s", url)

	// Get the latest block hash from the bootstrap node
	remoteTipHash, err := FetchLatestBlock(ctx, url)
	if err != nil {
		log.Printf("Failed to get bootstrap node tip: %v (will continue with P2P sync)", err)
		return
	}

	log.Printf("Bootstrap node tip: %s", remoteTipHash.String())
	if err := cm.SyncFromRemoteTip(ctx, remoteTipHash, url); err != nil {
		log.Printf("Bootstrap sync failed: %v (will continue with P2P sync)", err)
		return
	}

	// Log updated chain state after bootstrap
	if tip := cm.GetTip(ctx); tip != nil {
		log.Printf("Chain tip after bootstrap: %s at height %d", tip.Header.Hash().String(), tip.Height)
	}
}

// GetHeaderByHeight retrieves a header by height.
func (cm *ChainManager) GetHeaderByHeight(_ context.Context, height uint32) (*chaintracks.BlockHeader, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if len(cm.byHeight) > 0xFFFFFFFF {
		return nil, chaintracks.ErrIntegerOverflow
	}
	if height >= uint32(len(cm.byHeight)) { //nolint:gosec // Overflow check performed above
		return nil, chaintracks.ErrHeaderNotFound
	}

	hash := cm.byHeight[height]
	header, ok := cm.byHash[hash]
	if !ok {
		return nil, chaintracks.ErrHeaderNotFound
	}

	return header, nil
}

// GetHeaderByHash retrieves a header by hash.
func (cm *ChainManager) GetHeaderByHash(_ context.Context, hash *chainhash.Hash) (*chaintracks.BlockHeader, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	header, ok := cm.byHash[*hash]
	if !ok {
		return nil, chaintracks.ErrHeaderNotFound
	}

	return header, nil
}

// GetHeaders retrieves multiple headers starting from the given height.
func (cm *ChainManager) GetHeaders(ctx context.Context, height, count uint32) ([]*chaintracks.BlockHeader, error) {
	headers := make([]*chaintracks.BlockHeader, 0, count)
	for i := uint32(0); i < count; i++ {
		header, err := cm.GetHeaderByHeight(ctx, height+i)
		if err != nil {
			break
		}
		headers = append(headers, header)
	}
	return headers, nil
}

// GetTip returns the current chain tip.
func (cm *ChainManager) GetTip(_ context.Context) *chaintracks.BlockHeader {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.tip
}

// GetHeight returns the current chain height.
func (cm *ChainManager) GetHeight(_ context.Context) uint32 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if cm.tip == nil {
		return 0
	}
	return cm.tip.Height
}

// AddHeader adds a header to byHash for lookups without modifying the chain tip.
func (cm *ChainManager) AddHeader(header *chaintracks.BlockHeader) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.byHash[header.Hash] = header

	return nil
}

// GetNetwork returns the network name.
func (cm *ChainManager) GetNetwork(_ context.Context) (string, error) {
	return cm.network, nil
}

// Subscribe returns a channel that receives tip updates.
// When ctx is cancelled, the subscription is automatically removed.
func (cm *ChainManager) Subscribe(ctx context.Context) <-chan *chaintracks.BlockHeader {
	ch := make(chan *chaintracks.BlockHeader, 1)
	cm.subMu.Lock()
	cm.subscribers[ch] = struct{}{}
	cm.subMu.Unlock()

	go func() {
		<-ctx.Done()
		cm.Unsubscribe(ch)
	}()

	return ch
}

// Unsubscribe removes a subscriber channel.
func (cm *ChainManager) Unsubscribe(ch <-chan *chaintracks.BlockHeader) {
	cm.subMu.Lock()
	defer cm.subMu.Unlock()
	for sub := range cm.subscribers {
		if sub == ch {
			delete(cm.subscribers, sub)
			close(sub)
			return
		}
	}
}

// broadcast sends a tip update to all subscribers.
func (cm *ChainManager) broadcast(header *chaintracks.BlockHeader) {
	cm.subMu.RLock()
	defer cm.subMu.RUnlock()
	for ch := range cm.subscribers {
		select {
		case ch <- header:
		default:
		}
	}
}

// pruneOrphans removes old orphaned headers (must be called with lock held).
func (cm *ChainManager) pruneOrphans() {
	if cm.tip == nil {
		return
	}

	pruneHeight := uint32(0)
	if cm.tip.Height > 100 {
		pruneHeight = cm.tip.Height - 100
	}

	// Remove headers that are not in byHeight (orphans) and too old
	for hash, header := range cm.byHash {
		// Check if it's in the main chain
		chainLen := len(cm.byHeight)
		if chainLen <= 0xFFFFFFFF && header.Height < uint32(chainLen) && cm.byHeight[header.Height] == hash {
			continue
		}
		// It's an orphan, check if too old
		if header.Height < pruneHeight {
			delete(cm.byHash, hash)
		}
	}
}
