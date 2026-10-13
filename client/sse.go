package client

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
)

const (
	defaultMinBackoff = time.Second
	defaultMaxBackoff = 30 * time.Second

	// defaultIdleTimeout is three missed server keepalives (sent every 15s).
	defaultIdleTimeout = 45 * time.Second

	sseDataPrefix = "data: "
)

// errIdleTimeout is returned when a stream receives no line within the idle timeout.
var errIdleTimeout = errors.New("stream idle timeout")

// runStream keeps an SSE stream at path connected until ctx is canceled.
// onData is called with the payload of every non-empty "data: " line.
// After a connect failure, a non-200 response or a read error it reconnects
// with exponential backoff and jitter. The backoff resets once the server
// sends an event or a keepalive.
func (c *Client) runStream(ctx context.Context, path string, onData func(data string)) {
	minBackoff, maxBackoff := c.backoffBounds()
	backoff := minBackoff

	for ctx.Err() == nil {
		err := c.streamOnce(ctx, path, onData, func() { backoff = minBackoff })
		if ctx.Err() != nil {
			return
		}

		delay := jitter(backoff)
		log.Printf("chaintracks client: %s stream ended: %v; reconnecting in %v", path, err, delay)

		if !sleepCtx(ctx, delay) {
			return
		}

		backoff = min(backoff*2, maxBackoff)
	}
}

// backoffBounds returns the configured reconnect bounds, falling back to the
// defaults so a zero-value Client never reconnects in a tight loop.
func (c *Client) backoffBounds() (minBackoff, maxBackoff time.Duration) {
	minBackoff, maxBackoff = c.minBackoff, c.maxBackoff
	if minBackoff <= 0 {
		minBackoff = defaultMinBackoff
	}
	if maxBackoff < minBackoff {
		maxBackoff = max(minBackoff, defaultMaxBackoff)
	}
	return minBackoff, maxBackoff
}

// idleTimeoutOrDefault returns the configured idle timeout, falling back to the
// default so a zero-value Client still detects stalled streams.
func (c *Client) idleTimeoutOrDefault() time.Duration {
	if c.idleTimeout <= 0 {
		return defaultIdleTimeout
	}
	return c.idleTimeout
}

// streamOnce opens one SSE connection and reads it until it fails, goes idle or ctx is canceled.
// Any line, including ": keepalive" comments, counts as proof of life. If none arrives within
// the idle timeout the body is closed, which unblocks the read and returns errIdleTimeout.
// http.Client.Timeout is deliberately not used: it limits the whole request and would cut
// off healthy long-lived streams.
// onActivity is called for every non-empty line received.
func (c *Client) streamOnce(ctx context.Context, path string, onData func(string), onActivity func()) error {
	body, err := c.connectSSE(ctx, path)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()

	idle := c.idleTimeoutOrDefault()
	var timedOut atomic.Bool
	timer := time.AfterFunc(idle, func() {
		timedOut.Store(true)
		_ = body.Close()
	})
	defer timer.Stop()

	reader := bufio.NewReader(body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if timedOut.Load() {
				return fmt.Errorf("%w: no data for %v", errIdleTimeout, idle)
			}
			return fmt.Errorf("stream read failed: %w", err)
		}
		timer.Reset(idle)

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		onActivity()

		if data, ok := strings.CutPrefix(line, sseDataPrefix); ok && data != "" {
			onData(data)
		}
	}
}

// connectSSE establishes an SSE connection to the given path and returns the response body.
func (c *Client) connectSSE(ctx context.Context, path string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%w: status %d", chaintracks.ErrServerRequestFailed, resp.StatusCode)
	}

	return resp.Body, nil
}

// jitter returns a random duration in [d/2, d].
func jitter(d time.Duration) time.Duration {
	half := d / 2
	if half <= 0 {
		return d
	}
	return half + rand.N(half+1) //nolint:gosec // jitter does not need a cryptographic source
}

// sleepCtx waits for d or until ctx is canceled. It reports whether the full delay elapsed.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
