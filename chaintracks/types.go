package chaintracks

import (
	"math/big"

	"github.com/bsv-blockchain/go-sdk/block"
	"github.com/bsv-blockchain/go-sdk/chainhash"
)

// BlockHeader extends the base block.Header with additional chain-specific metadata
type BlockHeader struct {
	*block.Header

	Height    uint32         `json:"height"` // Block height in the chain
	Hash      chainhash.Hash `json:"hash"`
	ChainWork *big.Int       `json:"-"` // Cumulative chain work up to and including this block
}

// CDNMetadata represents the JSON metadata file structure
type CDNMetadata struct {
	RootFolder     string         `json:"rootFolder"`
	JSONFilename   string         `json:"jsonFilename"`
	HeadersPerFile int            `json:"headersPerFile"`
	Files          []CDNFileEntry `json:"files"`
}

// CDNFileEntry represents a single file entry in the metadata
type CDNFileEntry struct {
	Chain         string         `json:"chain"`
	Count         int            `json:"count"`
	FileHash      string         `json:"fileHash"`
	FileName      string         `json:"fileName"`
	FirstHeight   uint32         `json:"firstHeight"`
	LastChainWork string         `json:"lastChainWork"`
	LastHash      chainhash.Hash `json:"lastHash"`
	PrevChainWork string         `json:"prevChainWork"`
	PrevHash      chainhash.Hash `json:"prevHash"`
	SourceURL     string         `json:"sourceUrl"`
}

// ReorgEvent represents a chain reorganization event.
// The primary payload is OrphanedHashes - a slice of block hashes that are no longer on the main chain.
// Consumers should use this to invalidate any data (merkle paths, etc.) referencing these blocks.
type ReorgEvent struct {
	// OrphanedHashes lists the hashes of blocks no longer on the main chain.
	// These are the blocks that were replaced during the reorg.
	OrphanedHashes []chainhash.Hash `json:"orphanedHashes"`

	// CommonAncestor is the fork point where the chains diverged.
	CommonAncestor *BlockHeader `json:"commonAncestor"`

	// NewTip is the chain tip after the reorg.
	NewTip *BlockHeader `json:"newTip"`

	// Depth is the number of blocks that were replaced (length of OrphanedHashes).
	Depth uint32 `json:"depth"`
}
