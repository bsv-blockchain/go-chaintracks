package client

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
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

// streamSpec describes one SSE stream for runStream.
type streamSpec struct {
	path  string
	state *streamState
	// onData is called with the payload of every non-empty "data: " line.
	onData func(data string)
	// onDisconnect, if non-nil, is called each time a connection ends (but not on
	// cancellation), before the reconnect delay, so callers can invalidate state
	// derived from the stream.
	onDisconnect func()
}

// streamHooks are the callbacks streamOnce invokes while a connection is open.
type streamHooks struct {
	onConnect  func()
	onActivity func()
	onData     func(data string)
}

// runStream keeps the SSE stream described by spec connected until ctx is canceled.
// After a connect failure, a non-200 response, a read error or an idle timeout it
// reconnects with exponential backoff and jitter. The backoff resets once the server
// sends an event or a keepalive.
//
// Operators are told what is happening through the client logger: WARN when a
// connect attempt fails, the stream drops or goes idle (with the next retry delay),
// and INFO when a reconnect succeeds (with the attempt count and downtime).
func (c *Client) runStream(ctx context.Context, spec streamSpec) {
	lg := c.loggerOrDiscard().With("path", spec.path)
	minBackoff, maxBackoff := c.backoffBounds()
	backoff := minBackoff

	gen := spec.state.begin()
	defer spec.state.setConnected(gen, false)

	var (
		attempts  int       // connection attempts since the stream was last healthy
		downSince time.Time // when the stream last went down; zero while healthy
	)

	for ctx.Err() == nil {
		attempts++
		connected := false

		err := c.streamOnce(ctx, spec.path, streamHooks{
			onData: spec.onData,
			onConnect: func() {
				connected = true
				spec.state.setConnected(gen, true)

				if downSince.IsZero() {
					lg.Debug("stream connected")
				} else {
					lg.Info("stream reconnected", "attempts", attempts, "downtime", time.Since(downSince))
				}

				attempts = 0
				downSince = time.Time{}
			},
			onActivity: func() {
				backoff = minBackoff
				spec.state.touch(gen)
			},
		})
		spec.state.setConnected(gen, false)

		if ctx.Err() != nil {
			return
		}

		if downSince.IsZero() {
			downSince = time.Now()
		}

		if spec.onDisconnect != nil {
			spec.onDisconnect()
		}

		delay := jitter(backoff)
		logStreamEnd(lg, err, connected, delay)

		if !sleepCtx(ctx, delay) {
			return
		}

		backoff = min(backoff*2, maxBackoff)
	}
}

// logStreamEnd logs why a stream attempt ended and when the next one will start.
// The error includes the HTTP status when the server rejected the connection.
func logStreamEnd(lg *slog.Logger, err error, wasConnected bool, retryIn time.Duration) {
	switch {
	case errors.Is(err, errIdleTimeout):
		lg.Warn("stream idle timeout", "error", err, "retry_in", retryIn)
	case wasConnected:
		lg.Warn("stream dropped", "error", err, "retry_in", retryIn)
	default:
		lg.Warn("stream connect failed", "error", err, "retry_in", retryIn)
	}
}

// loggerOrDiscard returns the configured logger, or one that discards everything.
func (c *Client) loggerOrDiscard() *slog.Logger {
	if c.logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return c.logger
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
// h.onConnect is called once the server accepts the connection, and h.onActivity for every
// non-empty line received.
func (c *Client) streamOnce(ctx context.Context, path string, h streamHooks) error {
	body, err := c.connectSSE(ctx, path)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()

	h.onConnect()

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
		h.onActivity()

		if data, ok := strings.CutPrefix(line, sseDataPrefix); ok && data != "" {
			h.onData(data)
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
