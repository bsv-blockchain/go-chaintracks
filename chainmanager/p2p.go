package chainmanager

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"

	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
	"github.com/bsv-blockchain/go-sdk/block"
	teranode "github.com/bsv-blockchain/teranode/services/p2p"
)

// startBlockSubscription starts listening for P2P block announcements.
// The subscription will automatically stop when ctx is cancelled.
func (cm *ChainManager) startBlockSubscription(ctx context.Context) {
	log.Printf("Subscribing to P2P blocks for network: %s", cm.P2PClient.GetNetwork())

	blockChan := cm.P2PClient.SubscribeBlocks(ctx)

	go func() {
		for {
			select {
			case <-ctx.Done():
				close(cm.msgChan)
				return
			case blockMsg, ok := <-blockChan:
				if !ok {
					close(cm.msgChan)
					return
				}
				if err := cm.processBlockMessage(ctx, blockMsg); err != nil {
					log.Printf("Error handling block message: %v", err)
				}
			}
		}
	}()
}

// processBlockMessage processes a received block message.
func (cm *ChainManager) processBlockMessage(ctx context.Context, blockMsg teranode.BlockMessage) error {
	log.Printf("Received block: height=%d hash=%s from=%s datahub=%s", blockMsg.Height, blockMsg.Hash, blockMsg.PeerID, blockMsg.DataHubURL)

	// Decode header from hex
	headerBytes, err := hex.DecodeString(blockMsg.Header)
	if err != nil {
		return fmt.Errorf("failed to decode header hex: %w", err)
	}

	if len(headerBytes) != 80 {
		return fmt.Errorf("%w: %d bytes", chaintracks.ErrInvalidHeaderSize, len(headerBytes))
	}

	header, err := block.NewHeaderFromBytes(headerBytes)
	if err != nil {
		return fmt.Errorf("failed to parse header: %w", err)
	}

	// Check if we already have this block
	blockHash := header.Hash()
	if _, existsErr := cm.GetHeaderByHash(ctx, &blockHash); existsErr == nil {
		return nil
	}

	// Check if parent exists in our main chain (not just as an orphan)
	parentHash := header.PrevHash
	parentHeader, err := cm.GetHeaderByHash(ctx, &parentHash)
	if err == nil {
		// Parent exists in byHash - verify it's in the main chain (byHeight)
		mainChainHeader, err := cm.GetHeaderByHeight(ctx, parentHeader.Height)
		if err == nil && mainChainHeader.Hash == parentHash {
			// Parent is in main chain - simple case
			return cm.addBlockToChain(ctx, header, blockMsg.Height)
		}
		// Parent exists but is an orphan - need to crawl back to find common ancestor
		log.Printf("Parent %s exists but is not in main chain at height %d, crawling back...", parentHash, parentHeader.Height)
	} else {
		log.Printf("Parent not found for block %s, crawling back...", blockMsg.Hash)
	}
	return cm.crawlBackAndMerge(ctx, header, blockMsg.Height, blockMsg.DataHubURL)
}

// addBlockToChain processes a block and evaluates if it becomes the new chain tip.
func (cm *ChainManager) addBlockToChain(ctx context.Context, header *block.Header, height uint32) error {
	// Get parent to calculate chainwork
	parentHash := header.PrevHash
	parentHeader, err := cm.GetHeaderByHash(ctx, &parentHash)
	if err != nil {
		return fmt.Errorf("failed to get parent header: %w", err)
	}

	// Calculate chainwork
	work := CalculateWork(header.Bits)
	chainWork := new(big.Int).Add(parentHeader.ChainWork, work)

	// Create BlockHeader
	blockHeader := &chaintracks.BlockHeader{
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
	currentTip := cm.GetTip(ctx)
	if currentTip == nil || blockHeader.ChainWork.Cmp(currentTip.ChainWork) > 0 {
		log.Printf("New tip: height=%d chainwork=%s", blockHeader.Height, blockHeader.ChainWork.String())
		return cm.SetChainTip(ctx, []*chaintracks.BlockHeader{blockHeader})
	}

	log.Printf("Block added as orphan/alternate chain: height=%d", blockHeader.Height)
	return nil
}

// crawlBackAndMerge fetches missing parents until we find a connection to our chain.
func (cm *ChainManager) crawlBackAndMerge(ctx context.Context, header *block.Header, _ uint32, dataHubURL string) error {
	// Use the shared sync logic to walk backwards and find common ancestor
	blockHash := header.Hash()
	return cm.SyncFromRemoteTip(ctx, blockHash, dataHubURL)
}
