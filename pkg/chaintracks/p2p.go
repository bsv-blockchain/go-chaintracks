package chaintracks

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"

	p2p "github.com/bsv-blockchain/go-p2p-message-bus"
	"github.com/bsv-blockchain/go-sdk/block"
	"github.com/libp2p/go-libp2p/core/crypto"
)

// Start initializes and starts the P2P listener for block announcements
// Returns a channel that consumers can use to receive tip change notifications
func (cm *ChainManager) Start(ctx context.Context) (<-chan *BlockHeader, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.p2pClient != nil {
		return nil, ErrP2PAlreadyStarted
	}

	// Load or generate private key
	privKey, err := loadOrGeneratePrivateKey(cm.localStoragePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key: %w", err)
	}

	// Create P2P client
	client, err := p2p.NewClient(p2p.Config{
		Name:          "go-chaintracks",
		Logger:        &p2p.DefaultLogger{},
		PrivateKey:    privKey,
		Port:          0, // Random port
		PeerCacheFile: filepath.Join(cm.localStoragePath, "peer_cache.json"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create P2P client: %w", err)
	}

	cm.p2pClient = client
	cm.msgChan = make(chan *BlockHeader, 1) // Buffered channel (size 1) for latest tip only

	// Subscribe to block topic
	topic := fmt.Sprintf("teranode/bitcoin/1.0.0/%snet-block", cm.network)
	log.Printf("Subscribing to P2P topic: %s", topic)

	msgChan := client.Subscribe(topic)

	// Start message handler goroutine
	go func() {
		for {
			select {
			case <-ctx.Done():
				close(cm.msgChan)
				return
			case msg := <-msgChan:
				if err := cm.handleBlockMessage(ctx, msg.Data); err != nil {
					log.Printf("Error handling block message: %v", err)
				}
			}
		}
	}()

	return cm.msgChan, nil
}

// Stop stops the P2P listener if it's running
func (cm *ChainManager) Stop() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.p2pClient == nil {
		return nil
	}

	err := cm.p2pClient.Close()
	cm.p2pClient = nil
	return err
}

// GetPeers returns information about connected P2P peers
// Returns empty slice if P2P is not running
func (cm *ChainManager) GetPeers() []PeerInfo {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.p2pClient == nil {
		return []PeerInfo{}
	}

	p2pPeers := cm.p2pClient.GetPeers()
	peers := make([]PeerInfo, len(p2pPeers))
	for i, p := range p2pPeers {
		peers[i] = PeerInfo{
			ID:    p.ID,
			Name:  p.Name,
			Addrs: p.Addrs,
		}
	}
	return peers
}

// handleBlockMessage processes a received block message
func (cm *ChainManager) handleBlockMessage(ctx context.Context, data []byte) error {
	log.Printf("Raw block message: %s", string(data))

	var blockMsg BlockMessage
	if err := json.Unmarshal(data, &blockMsg); err != nil {
		return fmt.Errorf("failed to unmarshal block message: %w", err)
	}

	log.Printf("Received block: height=%d hash=%s from=%s datahub=%s", blockMsg.Height, blockMsg.Hash, blockMsg.PeerID, blockMsg.DataHubURL)

	// Decode header from hex
	headerBytes, err := hex.DecodeString(blockMsg.Header)
	if err != nil {
		return fmt.Errorf("failed to decode header hex: %w", err)
	}

	if len(headerBytes) != 80 {
		return fmt.Errorf("%w: %d bytes", ErrInvalidHeaderSize, len(headerBytes))
	}

	header, err := block.NewHeaderFromBytes(headerBytes)
	if err != nil {
		return fmt.Errorf("failed to parse header: %w", err)
	}

	// Check if parent exists in our chain
	parentHash := header.PrevHash
	_, err = cm.GetHeaderByHash(&parentHash)
	if err == nil {
		// Parent exists - simple case
		return cm.addBlockToChain(header, blockMsg.Height)
	}

	// Parent doesn't exist - need to crawl back
	log.Printf("Parent not found for block %s, crawling back...", blockMsg.Hash)
	return cm.crawlBackAndMerge(ctx, header, blockMsg.Height, blockMsg.DataHubURL)
}

// addBlockToChain processes a block and evaluates if it becomes the new chain tip
func (cm *ChainManager) addBlockToChain(header *block.Header, height uint32) error {
	// Get parent to calculate chainwork
	parentHash := header.PrevHash
	parentHeader, err := cm.GetHeaderByHash(&parentHash)
	if err != nil {
		return fmt.Errorf("failed to get parent header: %w", err)
	}

	// Calculate chainwork
	work := CalculateWork(header.Bits)
	chainWork := new(big.Int).Add(parentHeader.ChainWork, work)

	// Create BlockHeader
	blockHeader := &BlockHeader{
		Header:    header,
		Height:    height,
		Hash:      header.Hash(),
		ChainWork: chainWork,
	}

	// Always add the header to byHash first
	if err := cm.AddHeader(blockHeader); err != nil {
		return fmt.Errorf("failed to add header: %w", err)
	}

	// Check if this is the new tip
	currentTip := cm.GetTip()
	if currentTip == nil || blockHeader.ChainWork.Cmp(currentTip.ChainWork) > 0 {
		log.Printf("New tip: height=%d chainwork=%s", blockHeader.Height, blockHeader.ChainWork.String())
		return cm.SetChainTip([]*BlockHeader{blockHeader})
	}

	log.Printf("Block added as orphan/alternate chain: height=%d", blockHeader.Height)
	return nil
}

// crawlBackAndMerge fetches missing parents until we find a connection to our chain
func (cm *ChainManager) crawlBackAndMerge(_ context.Context, header *block.Header, _ uint32, dataHubURL string) error {
	// Use the shared sync logic to walk backwards and find common ancestor
	blockHash := header.Hash()
	return cm.SyncFromRemoteTip(blockHash, dataHubURL)
}

// loadOrGeneratePrivateKey loads a private key from file or generates a new one
func loadOrGeneratePrivateKey(storagePath string) (crypto.PrivKey, error) {
	keyPath := filepath.Join(storagePath, "p2p_key.hex")

	// Try to load existing key
	if _, err := os.Stat(keyPath); err == nil {
		data, err := os.ReadFile(keyPath) //nolint:gosec // Path is constructed internally, not from user input
		if err != nil {
			return nil, fmt.Errorf("failed to read key file: %w", err)
		}

		keyHex := string(data)
		privKey, err := p2p.PrivateKeyFromHex(keyHex)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}

		log.Printf("Loaded P2P private key from %s", keyPath)
		return privKey, nil
	}

	// Generate new key
	privKey, err := p2p.GeneratePrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Save to file
	keyHex, err := p2p.PrivateKeyToHex(privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encode private key: %w", err)
	}

	if err := os.MkdirAll(storagePath, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	if err := os.WriteFile(keyPath, []byte(keyHex), 0o600); err != nil {
		return nil, fmt.Errorf("failed to write key file: %w", err)
	}

	log.Printf("Generated new P2P private key: %s", keyPath)
	return privKey, nil
}
