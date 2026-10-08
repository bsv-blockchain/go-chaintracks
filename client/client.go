// Package client provides an HTTP client for connecting to a remote chaintracks server.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

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

	// SSE reconnect backoff bounds
	minBackoff time.Duration
	maxBackoff time.Duration
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
		minBackoff:       defaultMinBackoff,
		maxBackoff:       defaultMaxBackoff,
	}
}

// Subscribe returns a channel that receives tip updates.
// Starts the SSE connection on first subscriber and keeps it alive, reconnecting on failure,
// until the last subscriber leaves. When ctx is canceled, the subscription is removed.
func (c *Client) Subscribe(ctx context.Context) <-chan *chaintracks.BlockHeader {
	ch := make(chan *chaintracks.BlockHeader, 1)

	c.subMu.Lock()
	c.subscribers[ch] = struct{}{}
	if len(c.subscribers) == 1 {
		c.startSSE(ctx)
	}
	c.subMu.Unlock()

	go func() {
		<-ctx.Done()
		c.Unsubscribe(ch)
	}()

	return ch
}

// SubscribeReorg returns a channel that receives reorg events.
// Starts the reorg SSE connection on first subscriber and keeps it alive, reconnecting on failure,
// until the last subscriber leaves. When ctx is canceled, the subscription is removed.
func (c *Client) SubscribeReorg(ctx context.Context) <-chan *chaintracks.ReorgEvent {
	ch := make(chan *chaintracks.ReorgEvent, 1)

	c.reorgSubMu.Lock()
	c.reorgSubscribers[ch] = struct{}{}
	if len(c.reorgSubscribers) == 1 {
		c.startReorgSSE(ctx)
	}
	c.reorgSubMu.Unlock()

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

// startSSE starts the SSE connection and fan-out goroutine. Must be called with subMu held.
// The stream inherits parentCtx's values but not its cancellation, so one subscriber
// leaving cannot kill the stream for the others. It is only canceled by stopSSE
// when the last subscriber leaves.
func (c *Client) startSSE(parentCtx context.Context) {
	msgChan := make(chan *chaintracks.BlockHeader, 1)
	ctx, cancel := context.WithCancel(context.WithoutCancel(parentCtx))
	c.msgChan = msgChan
	c.sseCancel = cancel

	go c.runSSE(ctx, msgChan)
	go c.fanOut(ctx, msgChan)
}

// startReorgSSE starts the reorg SSE connection and fan-out goroutine. Must be called with reorgSubMu held.
// The stream inherits parentCtx's values but not its cancellation, so one subscriber
// leaving cannot kill the stream for the others. It is only canceled by stopReorgSSE
// when the last subscriber leaves.
func (c *Client) startReorgSSE(parentCtx context.Context) {
	msgChan := make(chan *chaintracks.ReorgEvent, 1)
	ctx, cancel := context.WithCancel(context.WithoutCancel(parentCtx))
	c.reorgMsgChan = msgChan
	c.reorgSSECancel = cancel

	go c.runReorgSSE(ctx, msgChan)
	go c.reorgFanOut(ctx, msgChan)
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
func (c *Client) fanOut(ctx context.Context, msgChan <-chan *chaintracks.BlockHeader) {
	for {
		select {
		case <-ctx.Done():
			return
		case header, ok := <-msgChan:
			if !ok {
				return
			}
			c.broadcast(header)
		}
	}
}

// reorgFanOut reads from msgChan and broadcasts to all subscribers.
func (c *Client) reorgFanOut(ctx context.Context, msgChan <-chan *chaintracks.ReorgEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case reorgEvent, ok := <-msgChan:
			if !ok {
				return
			}
			c.reorgBroadcast(reorgEvent)
		}
	}
}

// runSSE keeps the tip stream connected, reconnecting on failure, and forwards
// new tips to msgChan until ctx is canceled.
func (c *Client) runSSE(ctx context.Context, msgChan chan<- *chaintracks.BlockHeader) {
	defer close(msgChan)

	// lastHash survives reconnects so the tip the server resends on connect is not delivered twice.
	var lastHash *chainhash.Hash

	c.runStream(ctx, "/v2/tip/stream", func(data string) {
		var blockHeader chaintracks.BlockHeader
		if err := json.Unmarshal([]byte(data), &blockHeader); err != nil {
			return
		}

		if lastHash != nil && lastHash.IsEqual(&blockHeader.Hash) {
			return
		}
		lastHash = &blockHeader.Hash

		if ctx.Err() != nil {
			return
		}

		c.tipMu.Lock()
		c.currentTip = &blockHeader
		c.tipMu.Unlock()

		select {
		case msgChan <- &blockHeader:
		default:
		}
	})
}

// runReorgSSE keeps the reorg stream connected, reconnecting on failure, and
// forwards reorg events to msgChan until ctx is canceled.
func (c *Client) runReorgSSE(ctx context.Context, msgChan chan<- *chaintracks.ReorgEvent) {
	defer close(msgChan)

	c.runStream(ctx, "/v2/reorg/stream", func(data string) {
		var reorgEvent chaintracks.ReorgEvent
		if err := json.Unmarshal([]byte(data), &reorgEvent); err != nil {
			return
		}

		if reorgEvent.NewTip == nil {
			return
		}

		select {
		case msgChan <- &reorgEvent:
		default:
		}
	})
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
	return c.fetchHeader(ctx, c.baseURL+"/v2/tip")
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

// doGet performs an HTTP GET and returns the response body. Caller must close the body.
func (c *Client) doGet(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%w: status %d", chaintracks.ErrServerRequestFailed, resp.StatusCode)
	}

	return resp, nil
}

// GetHeaders retrieves multiple headers starting from the given height.
func (c *Client) GetHeaders(ctx context.Context, height, count uint32) ([]*chaintracks.BlockHeader, error) {
	url := fmt.Sprintf("%s/v2/headers?height=%d&count=%d", c.baseURL, height, count)
	resp, err := c.doGet(ctx, url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

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
			Height: height + uint32(headerIndex),
			Hash:   h.Hash(),
		})
	}

	return headers, nil
}

// fetchHeader is a helper to fetch and parse a header from the server.
func (c *Client) fetchHeader(ctx context.Context, url string) (*chaintracks.BlockHeader, error) {
	resp, err := c.doGet(ctx, url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var header chaintracks.BlockHeader
	if err := json.NewDecoder(resp.Body).Decode(&header); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &header, nil
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

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: status %d", chaintracks.ErrServerRequestFailed, resp.StatusCode)
	}

	var response struct {
		Network string `json:"network"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if response.Network == "" {
		return "", chaintracks.ErrServerReturnedError
	}

	return response.Network, nil
}
