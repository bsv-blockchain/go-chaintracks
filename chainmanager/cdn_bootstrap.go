package chainmanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/big"
	"net/http"
	"time"

	"github.com/bsv-blockchain/go-sdk/block"

	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
)

const (
	cdnHeadersPerFile = 100000
	cdnHeaderSize     = 80
)

// Sentinel errors for CDN bootstrap operations.
var (
	ErrCDNMetadataFetchFailed = errors.New("metadata fetch failed")
	ErrCDNFileFetchFailed     = errors.New("file fetch failed")
	ErrCDNInvalidFileSize     = errors.New("invalid file size")
)

// CDNBootstrapper handles bootstrap from CDN-format header files.
type CDNBootstrapper struct {
	baseURL    string
	network    string
	httpClient *http.Client
}

// NewCDNBootstrapper creates a new CDN bootstrapper.
func NewCDNBootstrapper(baseURL, network string) *CDNBootstrapper {
	return &CDNBootstrapper{
		baseURL: baseURL,
		network: network,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // Large files need longer timeout
		},
	}
}

// FetchMetadata downloads and parses the CDN metadata JSON.
// Expected URL format: {baseURL}/{network}NetBlockHeaders.json
func (b *CDNBootstrapper) FetchMetadata(ctx context.Context) (*chaintracks.CDNMetadata, error) {
	url := fmt.Sprintf("%s/%sNetBlockHeaders.json", b.baseURL, b.network)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create metadata request: %w", err)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metadata: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrCDNMetadataFetchFailed, resp.StatusCode)
	}

	var metadata chaintracks.CDNMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	return &metadata, nil
}

// FetchHeadersFile downloads a single .headers file.
// Expected URL format: {baseURL}/{fileName}
func (b *CDNBootstrapper) FetchHeadersFile(ctx context.Context, fileName string) ([]byte, error) {
	url := fmt.Sprintf("%s/%s", b.baseURL, fileName)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create file request: %w", err)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch file %s: %w", fileName, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w for %s: status %d", ErrCDNFileFetchFailed, fileName, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", fileName, err)
	}

	// Validate file size
	if len(data)%cdnHeaderSize != 0 {
		return nil, fmt.Errorf("%w for %s: %d bytes (not multiple of %d)",
			ErrCDNInvalidFileSize, fileName, len(data), cdnHeaderSize)
	}

	return data, nil
}

// Bootstrap performs full CDN bootstrap, downloading all header files.
func (b *CDNBootstrapper) Bootstrap(ctx context.Context, cm *ChainManager) error {
	log.Printf("Starting CDN bootstrap from %s", b.baseURL)

	// 1. Fetch metadata
	metadata, err := b.FetchMetadata(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch CDN metadata: %w", err)
	}

	log.Printf("CDN metadata: %d files, %d headers per file",
		len(metadata.Files), metadata.HeadersPerFile)

	// 2. Determine which files we need (skip already-loaded ones)
	currentHeight := cm.GetHeight(ctx)
	startFileIndex := currentHeight / cdnHeadersPerFile

	log.Printf("Current height: %d, starting from file index %d", currentHeight, startFileIndex)

	// 3. Download and import each file
	if err := b.processFiles(ctx, cm, metadata, startFileIndex); err != nil {
		return err
	}

	tip := cm.GetTip(ctx)
	if tip != nil {
		log.Printf("CDN bootstrap complete. Chain tip: height=%d hash=%s", tip.Height, tip.Hash.String())
	}

	return nil
}

// processFiles downloads and imports each file from the CDN metadata.
func (b *CDNBootstrapper) processFiles(ctx context.Context, cm *ChainManager,
	metadata *chaintracks.CDNMetadata, startFileIndex uint32,
) error {
	for i, fileEntry := range metadata.Files {
		if i > math.MaxUint32 || uint32(i) < startFileIndex { //nolint:gosec // overflow checked
			continue // Already have this file's headers
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := b.processFileEntry(ctx, cm, fileEntry, i, len(metadata.Files)); err != nil {
			return err
		}
	}
	return nil
}

// processFileEntry downloads and imports a single header file.
func (b *CDNBootstrapper) processFileEntry(ctx context.Context, cm *ChainManager,
	fileEntry chaintracks.CDNFileEntry, index, total int,
) error {
	log.Printf("Downloading %s (file %d/%d)", fileEntry.FileName, index+1, total)

	data, err := b.FetchHeadersFile(ctx, fileEntry.FileName)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", fileEntry.FileName, err)
	}

	// Parse headers from binary data
	headers, err := parseHeadersFromBytes(data)
	if err != nil {
		return fmt.Errorf("failed to parse headers from %s: %w", fileEntry.FileName, err)
	}

	// Convert to BlockHeaders with heights and chainwork
	blockHeaders, err := b.convertToBlockHeaders(ctx, cm, headers, fileEntry.FirstHeight)
	if err != nil {
		return fmt.Errorf("failed to convert headers from %s: %w", fileEntry.FileName, err)
	}

	// Import into chain manager
	if err := cm.SetChainTip(ctx, blockHeaders); err != nil {
		return fmt.Errorf("failed to set chain tip from %s: %w", fileEntry.FileName, err)
	}

	headerCount := len(blockHeaders)
	if headerCount > math.MaxUint32 {
		headerCount = math.MaxUint32
	}
	log.Printf("Imported %d headers from %s (height %d-%d)",
		headerCount, fileEntry.FileName,
		fileEntry.FirstHeight, fileEntry.FirstHeight+uint32(headerCount)-1) //nolint:gosec // overflow checked

	return nil
}

// parseHeadersFromBytes parses raw bytes into block headers.
func parseHeadersFromBytes(data []byte) ([]*block.Header, error) {
	headerCount := len(data) / cdnHeaderSize
	headers := make([]*block.Header, 0, headerCount)

	for i := 0; i < headerCount; i++ {
		headerBytes := data[i*cdnHeaderSize : (i+1)*cdnHeaderSize]
		header, err := block.NewHeaderFromBytes(headerBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse header at index %d: %w", i, err)
		}
		headers = append(headers, header)
	}

	return headers, nil
}

// convertToBlockHeaders converts raw headers to BlockHeaders with height and chainwork.
func (b *CDNBootstrapper) convertToBlockHeaders(ctx context.Context, cm *ChainManager,
	headers []*block.Header, firstHeight uint32,
) ([]*chaintracks.BlockHeader, error) {
	blockHeaders := make([]*chaintracks.BlockHeader, 0, len(headers))

	var prevChainWork *big.Int
	if firstHeight == 0 {
		prevChainWork = big.NewInt(0)
	} else {
		prevHeader, err := cm.GetHeaderByHeight(ctx, firstHeight-1)
		if err != nil {
			return nil, fmt.Errorf("failed to get previous header at height %d: %w", firstHeight-1, err)
		}
		prevChainWork = prevHeader.ChainWork
	}

	for i, header := range headers {
		if i > math.MaxUint32 {
			break // Prevent overflow
		}
		height := firstHeight + uint32(i) //nolint:gosec // overflow checked

		var chainWork *big.Int
		if height == 0 {
			chainWork = big.NewInt(0)
		} else {
			work := CalculateWork(header.Bits)
			chainWork = new(big.Int).Add(prevChainWork, work)
			prevChainWork = chainWork
		}

		blockHeader := &chaintracks.BlockHeader{
			Header:    header,
			Height:    height,
			Hash:      header.Hash(),
			ChainWork: chainWork,
		}

		blockHeaders = append(blockHeaders, blockHeader)
	}

	return blockHeaders, nil
}

// runCDNBootstrap performs initial sync from a CDN endpoint.
func (cm *ChainManager) runCDNBootstrap(ctx context.Context, url string) {
	log.Printf("CDN Bootstrap URL configured: %s", url)

	bootstrapper := NewCDNBootstrapper(url, cm.network)
	if err := bootstrapper.Bootstrap(ctx, cm); err != nil {
		log.Printf("CDN bootstrap failed: %v (will continue with P2P sync)", err)
		return
	}

	if tip := cm.GetTip(ctx); tip != nil {
		log.Printf("Chain tip after CDN bootstrap: %s at height %d", tip.Header.Hash().String(), tip.Height)
	}
}
