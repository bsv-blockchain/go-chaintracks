package chaintracks

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/bsv-blockchain/go-sdk/chainhash"
)

// Client is an HTTP client for chaintracks server with SSE support
type Client struct {
	baseURL    string
	httpClient *http.Client
	currentTip *BlockHeader
	tipMu      sync.RWMutex
	msgChan    chan *BlockHeader
	cancelFunc context.CancelFunc
}

// NewClient creates a new HTTP client for chaintracks server
func NewClient(baseURL string) *Client {
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

// Start connects to the SSE stream and returns a channel for tip updates
func (cc *Client) Start(ctx context.Context) (<-chan *BlockHeader, error) {
	cc.msgChan = make(chan *BlockHeader, 1)

	childCtx, cancel := context.WithCancel(ctx)
	cc.cancelFunc = cancel

	req, err := http.NewRequestWithContext(childCtx, "GET", cc.baseURL+"/v2/tip/stream", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSE request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")

	resp, err := cc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SSE stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%w: status %d", ErrSSEStreamFailed, resp.StatusCode)
	}

	go cc.readSSE(childCtx, resp.Body)

	return cc.msgChan, nil
}

// readSSE reads Server-Sent Events from the response body
//
//nolint:gocyclo // Inherent complexity of SSE parsing logic
func (cc *Client) readSSE(ctx context.Context, body io.ReadCloser) {
	defer func() { _ = body.Close() }()
	defer close(cc.msgChan)

	reader := bufio.NewReader(body)
	var lastHash *chainhash.Hash

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				return
			}
			return
		}

		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "" {
			continue
		}

		var blockHeader BlockHeader
		if err := json.Unmarshal([]byte(data), &blockHeader); err != nil {
			continue
		}

		if lastHash != nil && lastHash.IsEqual(&blockHeader.Hash) {
			continue
		}

		lastHash = &blockHeader.Hash

		cc.tipMu.Lock()
		cc.currentTip = &blockHeader
		cc.tipMu.Unlock()

		select {
		case cc.msgChan <- &blockHeader:
		case <-ctx.Done():
			return
		default:
		}
	}
}

// Stop closes the SSE connection
func (cc *Client) Stop() error {
	if cc.cancelFunc != nil {
		cc.cancelFunc()
	}
	return nil
}

// GetTip returns the current chain tip
func (cc *Client) GetTip() *BlockHeader {
	cc.tipMu.RLock()
	defer cc.tipMu.RUnlock()
	return cc.currentTip
}

// GetHeight returns the current chain height
func (cc *Client) GetHeight() uint32 {
	cc.tipMu.RLock()
	defer cc.tipMu.RUnlock()
	if cc.currentTip == nil {
		return 0
	}
	return cc.currentTip.Height
}

// GetHeaderByHeight retrieves a header by height from the server
func (cc *Client) GetHeaderByHeight(height uint32) (*BlockHeader, error) {
	url := fmt.Sprintf("%s/v2/header/height/%d", cc.baseURL, height)
	return cc.fetchHeader(url)
}

// GetHeaderByHash retrieves a header by hash from the server
func (cc *Client) GetHeaderByHash(hash *chainhash.Hash) (*BlockHeader, error) {
	url := fmt.Sprintf("%s/v2/header/hash/%s", cc.baseURL, hash.String())
	return cc.fetchHeader(url)
}

// fetchHeader is a helper to fetch and parse a header from the server
func (cc *Client) fetchHeader(url string) (*BlockHeader, error) {
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := cc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch header: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrServerRequestFailed, resp.StatusCode)
	}

	var response struct {
		Status string       `json:"status"`
		Value  *BlockHeader `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if response.Status != "success" || response.Value == nil {
		return nil, ErrHeaderNotFound
	}

	return response.Value, nil
}

// IsValidRootForHeight implements the ChainTracker interface
func (cc *Client) IsValidRootForHeight(_ context.Context, root *chainhash.Hash, height uint32) (bool, error) {
	header, err := cc.GetHeaderByHeight(height) //nolint:contextcheck // Interface doesn't support context
	if err != nil {
		return false, err
	}
	return header.MerkleRoot.IsEqual(root), nil
}

// CurrentHeight implements the ChainTracker interface
func (cc *Client) CurrentHeight(_ context.Context) (uint32, error) {
	return cc.GetHeight(), nil
}

// GetNetwork returns the network name from the server
func (cc *Client) GetNetwork() (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), "GET", cc.baseURL+"/v2/network", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := cc.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch network: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var response struct {
		Status string `json:"status"`
		Value  string `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if response.Status != "success" {
		return "", ErrServerReturnedError
	}

	return response.Value, nil
}
