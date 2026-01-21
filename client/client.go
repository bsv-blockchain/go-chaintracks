// Package client provides an HTTP client for connecting to a remote chaintracks server.
package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/bsv-blockchain/go-sdk/block"
	"github.com/bsv-blockchain/go-sdk/chainhash"

	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
)

// Client is an HTTP client for chaintracks server with SSE support.
type Client struct {
	baseURL    string
	httpClient *http.Client

	// SSE state
	currentTip *chaintracks.BlockHeader
	tipMu      sync.RWMutex
	msgChan    chan *chaintracks.BlockHeader
	sseCancel  context.CancelFunc

	// Subscriber fan-out
	subscribers map[chan *chaintracks.BlockHeader]struct{}
	subMu       sync.Mutex

	// Reorg SSE state
	reorgMsgChan     chan *chaintracks.ReorgEvent
	reorgSSECancel   context.CancelFunc
	reorgSubscribers map[chan *chaintracks.ReorgEvent]struct{}
	reorgSubMu       sync.Mutex
}

// New creates a new HTTP client for chaintracks server.
func New(baseURL string) *Client {
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &Client{
		baseURL:          baseURL,
		httpClient:       &http.Client{},
		subscribers:      make(map[chan *chaintracks.BlockHeader]struct{}),
		reorgSubscribers: make(map[chan *chaintracks.ReorgEvent]struct{}),
	}
}

// Subscribe returns a channel that receives tip updates.
// Starts SSE connection on first subscriber. When ctx is canceled, the subscription is removed.
func (c *Client) Subscribe(ctx context.Context) <-chan *chaintracks.BlockHeader {
	ch := make(chan *chaintracks.BlockHeader, 1)

	c.subMu.Lock()
	c.subscribers[ch] = struct{}{}
	firstSubscriber := len(c.subscribers) == 1
	c.subMu.Unlock()

	if firstSubscriber {
		c.startSSE(ctx)
	}

	go func() {
		<-ctx.Done()
		c.Unsubscribe(ch)
	}()

	return ch
}

// SubscribeReorg returns a channel that receives reorg events.
// Starts reorg SSE connection on first subscriber. When ctx is canceled, the subscription is removed.
func (c *Client) SubscribeReorg(ctx context.Context) <-chan *chaintracks.ReorgEvent {
	ch := make(chan *chaintracks.ReorgEvent, 1)

	c.reorgSubMu.Lock()
	c.reorgSubscribers[ch] = struct{}{}
	firstSubscriber := len(c.reorgSubscribers) == 1
	c.reorgSubMu.Unlock()

	if firstSubscriber {
		c.startReorgSSE(ctx)
	}

	go func() {
		<-ctx.Done()
		c.UnsubscribeReorg(ch)
	}()

	return ch
}

// Unsubscribe removes a subscriber channel.
// Stops SSE and clears tip cache when last subscriber leaves.
func (c *Client) Unsubscribe(ch <-chan *chaintracks.BlockHeader) {
	c.subMu.Lock()
	defer c.subMu.Unlock()

	for sub := range c.subscribers {
		if sub == ch {
			delete(c.subscribers, sub)
			close(sub)
			break
		}
	}

	if len(c.subscribers) == 0 {
		c.stopSSE()
	}
}

// UnsubscribeReorg removes a reorg subscriber channel.
// Stops reorgSSE.
func (c *Client) UnsubscribeReorg(ch <-chan *chaintracks.ReorgEvent) {
	c.reorgSubMu.Lock()
	defer c.reorgSubMu.Unlock()

	for sub := range c.reorgSubscribers {
		if sub == ch {
			delete(c.reorgSubscribers, sub)
			close(sub)
			break
		}
	}

	if len(c.reorgSubscribers) == 0 {
		c.stopReorgSSE()
	}
}

// startSSE starts the SSE connection and fan-out goroutine.
func (c *Client) startSSE(parentCtx context.Context) {
	c.msgChan = make(chan *chaintracks.BlockHeader, 1)
	ctx, cancel := context.WithCancel(parentCtx)
	c.sseCancel = cancel

	go c.runSSE(ctx)
	go c.fanOut(ctx)
}

// startReorgSSE starts the reorg SSE connection and fan-out goroutine.
func (c *Client) startReorgSSE(parentCtx context.Context) {
	c.reorgMsgChan = make(chan *chaintracks.ReorgEvent, 1)
	ctx, cancel := context.WithCancel(parentCtx)
	c.reorgSSECancel = cancel

	go c.runReorgSSE(ctx)
	go c.reorgFanOut(ctx)
}

// stopSSE stops the SSE connection and clears the tip cache.
func (c *Client) stopSSE() {
	if c.sseCancel != nil {
		c.sseCancel()
		c.sseCancel = nil
	}

	c.tipMu.Lock()
	c.currentTip = nil
	c.tipMu.Unlock()
}

// stopReorgSSE stops the reorg SSE connection.
func (c *Client) stopReorgSSE() {
	if c.reorgSSECancel != nil {
		c.reorgSSECancel()
		c.reorgSSECancel = nil
	}
}

// fanOut reads from msgChan and broadcasts to all subscribers.
func (c *Client) fanOut(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case header, ok := <-c.msgChan:
			if !ok {
				return
			}
			c.broadcast(header)
		}
	}
}

// reorgFanOut reads from reorgMsgChan and broadcasts to all subscribers.
func (c *Client) reorgFanOut(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case reorgEvent, ok := <-c.reorgMsgChan:
			if !ok {
				return
			}
			c.reorgBroadcast(reorgEvent)
		}
	}
}

// runSSE connects to the SSE stream and reads events.
func (c *Client) runSSE(ctx context.Context) {
	defer close(c.msgChan)

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v2/tip/stream", nil)
	if err != nil {
		return
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return
	}

	c.readSSE(ctx, resp.Body)
}

// runReorgSSE connects to the reorg SSE stream and reads events.
func (c *Client) runReorgSSE(ctx context.Context) {
	defer close(c.reorgMsgChan)

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v2/reorg/stream", nil)
	if err != nil {
		return
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return
	}

	c.readReorgSSE(ctx, resp.Body)
}

// readSSE reads Server-Sent Events from the response body.
//
//nolint:gocyclo // Inherent complexity of SSE parsing logic
func (c *Client) readSSE(ctx context.Context, body io.ReadCloser) {
	defer func() { _ = body.Close() }()

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

		var blockHeader chaintracks.BlockHeader
		if err := json.Unmarshal([]byte(data), &blockHeader); err != nil {
			continue
		}

		if lastHash != nil && lastHash.IsEqual(&blockHeader.Hash) {
			continue
		}

		lastHash = &blockHeader.Hash

		c.tipMu.Lock()
		c.currentTip = &blockHeader
		c.tipMu.Unlock()

		select {
		case c.msgChan <- &blockHeader:
		case <-ctx.Done():
			return
		default:
		}
	}
}

// readReorgSSE reads Server-Sent Events from the response body.
//
// Inherent complexity of SSE parsing logic
func (c *Client) readReorgSSE(ctx context.Context, body io.ReadCloser) {
	defer func() { _ = body.Close() }()

	reader := bufio.NewReader(body)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
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

		var reorgEvent chaintracks.ReorgEvent
		if err := json.Unmarshal([]byte(data), &reorgEvent); err != nil {
			continue
		}

		if reorgEvent.NewTip == nil {
			continue
		}

		select {
		case c.reorgMsgChan <- &reorgEvent:
		case <-ctx.Done():
			return
		default:
		}
	}
}

// broadcast sends a tip update to all subscribers.
func (c *Client) broadcast(header *chaintracks.BlockHeader) {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	for ch := range c.subscribers {
		select {
		case ch <- header:
		default:
		}
	}
}

// reorgBroadcast sends a reorg eventto all subscribers.
func (c *Client) reorgBroadcast(reorgEvent *chaintracks.ReorgEvent) {
	c.reorgSubMu.Lock()
	defer c.reorgSubMu.Unlock()
	for ch := range c.reorgSubscribers {
		select {
		case ch <- reorgEvent:
		default:
		}
	}
}

// GetTip returns the current chain tip.
// If there are active subscribers, returns cached tip. Otherwise makes a REST call.
func (c *Client) GetTip(ctx context.Context) *chaintracks.BlockHeader {
	c.tipMu.RLock()
	tip := c.currentTip
	c.tipMu.RUnlock()

	if tip != nil {
		return tip
	}

	// No cached tip, fetch via REST
	header, err := c.fetchTip(ctx)
	if err != nil {
		return nil
	}
	return header
}

// GetHeight returns the current chain height.
// If there are active subscribers, returns cached height. Otherwise makes a REST call.
func (c *Client) GetHeight(ctx context.Context) uint32 {
	tip := c.GetTip(ctx)
	if tip == nil {
		return 0
	}
	return tip.Height
}

// fetchTip fetches the current tip via REST.
func (c *Client) fetchTip(ctx context.Context) (*chaintracks.BlockHeader, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v2/tip", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tip: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", chaintracks.ErrServerRequestFailed, resp.StatusCode)
	}

	var response struct {
		Status string                   `json:"status"`
		Value  *chaintracks.BlockHeader `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if response.Status != "success" || response.Value == nil {
		return nil, chaintracks.ErrHeaderNotFound
	}

	return response.Value, nil
}

// GetHeaderByHeight retrieves a header by height from the server.
func (c *Client) GetHeaderByHeight(ctx context.Context, height uint32) (*chaintracks.BlockHeader, error) {
	url := fmt.Sprintf("%s/v2/header/height/%d", c.baseURL, height)
	return c.fetchHeader(ctx, url)
}

// GetHeaderByHash retrieves a header by hash from the server.
func (c *Client) GetHeaderByHash(ctx context.Context, hash *chainhash.Hash) (*chaintracks.BlockHeader, error) {
	url := fmt.Sprintf("%s/v2/header/hash/%s", c.baseURL, hash.String())
	return c.fetchHeader(ctx, url)
}

// GetHeaders retrieves multiple headers starting from the given height.
func (c *Client) GetHeaders(ctx context.Context, height, count uint32) ([]*chaintracks.BlockHeader, error) {
	url := fmt.Sprintf("%s/v2/headers?height=%d&count=%d", c.baseURL, height, count)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch headers: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", chaintracks.ErrServerRequestFailed, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if len(data)%80 != 0 {
		return nil, fmt.Errorf("%w: %d bytes", chaintracks.ErrInvalidResponseLength, len(data))
	}

	var headers []*chaintracks.BlockHeader
	for i := 0; i < len(data); i += 80 {
		h, err := block.NewHeaderFromBytes(data[i : i+80])
		if err != nil {
			return nil, fmt.Errorf("failed to parse header at offset %d: %w", i, err)
		}
		headerIndex := i / 80
		headers = append(headers, &chaintracks.BlockHeader{
			Header: h,
			Height: height + uint32(headerIndex), //nolint:gosec // headerIndex is bounded by response size
			Hash:   h.Hash(),
		})
	}

	return headers, nil
}

// fetchHeader is a helper to fetch and parse a header from the server.
func (c *Client) fetchHeader(ctx context.Context, url string) (*chaintracks.BlockHeader, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch header: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", chaintracks.ErrServerRequestFailed, resp.StatusCode)
	}

	var response struct {
		Status string                   `json:"status"`
		Value  *chaintracks.BlockHeader `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if response.Status != "success" || response.Value == nil {
		return nil, chaintracks.ErrHeaderNotFound
	}

	return response.Value, nil
}

// IsValidRootForHeight implements the ChainTracker interface.
func (c *Client) IsValidRootForHeight(ctx context.Context, root *chainhash.Hash, height uint32) (bool, error) {
	header, err := c.GetHeaderByHeight(ctx, height)
	if err != nil {
		return false, err
	}
	return header.MerkleRoot.IsEqual(root), nil
}

// CurrentHeight implements the ChainTracker interface.
func (c *Client) CurrentHeight(ctx context.Context) (uint32, error) {
	return c.GetHeight(ctx), nil
}

// GetNetwork returns the network name from the server.
func (c *Client) GetNetwork(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v2/network", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch network: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var response struct {
		Status string `json:"status"`
		Value  string `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if response.Status != "success" {
		return "", chaintracks.ErrServerReturnedError
	}

	return response.Value, nil
}
