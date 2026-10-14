package client

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bsv-blockchain/go-sdk/block"
	"github.com/bsv-blockchain/go-sdk/chainhash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
)

const sseTestTimeout = 3 * time.Second

func newFastClient(url string) *Client {
	c := New(url)
	c.minBackoff = 5 * time.Millisecond
	c.maxBackoff = 20 * time.Millisecond
	return c
}

func tipAt(height uint32) *chaintracks.BlockHeader {
	var hash chainhash.Hash
	binary.LittleEndian.PutUint32(hash[:], height)

	return &chaintracks.BlockHeader{
		Header: &block.Header{},
		Height: height,
		Hash:   hash,
	}
}

// writeEvent sends v as one SSE data event. It is called from HTTP handler
// goroutines, so it reports failures with t.Errorf: require would call FailNow off
// the test goroutine, and testifylint rejects assert for error checks.
func writeEvent(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Errorf("marshal event: %v", err)
		return
	}
	if _, err = fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		t.Errorf("write event: %v", err)
		return
	}
	w.(http.Flusher).Flush()
}

func writeTip(t *testing.T, w http.ResponseWriter, tip *chaintracks.BlockHeader) {
	t.Helper()
	writeEvent(t, w, tip)
}

func startStream(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	w.(http.Flusher).Flush()
}

func waitForTip(t *testing.T, ch <-chan *chaintracks.BlockHeader, height uint32) {
	t.Helper()
	select {
	case tip, ok := <-ch:
		require.True(t, ok, "subscriber channel closed")
		assert.Equal(t, height, tip.Height)
	case <-time.After(sseTestTimeout):
		t.Fatalf("timeout waiting for tip at height %d", height)
	}
}

func TestSubscribeReconnectsWhenServerClosesStream(t *testing.T) {
	var connections atomic.Uint32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := connections.Add(1)
		startStream(w)
		writeTip(t, w, tipAt(n))
		if n == 1 {
			return // drop the first connection
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	client := newFastClient(server.URL)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ch := client.Subscribe(ctx)
	waitForTip(t, ch, 1)
	waitForTip(t, ch, 2)
	assert.Equal(t, uint32(2), client.GetTip(ctx).Height)
}

func TestSubscribeRetriesAfterServerError(t *testing.T) {
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if connections.Add(1) <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		startStream(w)
		writeTip(t, w, tipAt(7))
		<-r.Context().Done()
	}))
	defer server.Close()

	client := newFastClient(server.URL)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ch := client.Subscribe(ctx)
	waitForTip(t, ch, 7)
	assert.GreaterOrEqual(t, connections.Load(), int32(3))
}

func TestSubscribeSurvivesFirstSubscriberCancel(t *testing.T) {
	var connections atomic.Int32
	events := make(chan uint32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connections.Add(1)
		startStream(w)
		for {
			select {
			case h := <-events:
				writeTip(t, w, tipAt(h))
			case <-r.Context().Done():
				return
			}
		}
	}))
	defer server.Close()

	client := newFastClient(server.URL)
	firstCtx, cancelFirst := context.WithCancel(t.Context())
	defer cancelFirst()
	secondCtx, cancelSecond := context.WithCancel(t.Context())
	defer cancelSecond()

	_ = client.Subscribe(firstCtx)
	second := client.Subscribe(secondCtx)

	events <- 1
	waitForTip(t, second, 1)

	cancelFirst()
	require.Eventually(t, func() bool {
		client.subMu.Lock()
		defer client.subMu.Unlock()
		return len(client.subscribers) == 1
	}, sseTestTimeout, 5*time.Millisecond)

	events <- 2
	waitForTip(t, second, 2)
	assert.Equal(t, int32(1), connections.Load(), "stream must not restart")
}

func TestUnsubscribeLastStopsRetryLoop(t *testing.T) {
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		connections.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := newFastClient(server.URL)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ch := client.Subscribe(ctx)
	require.Eventually(t, func() bool { return connections.Load() >= 2 }, sseTestTimeout, 5*time.Millisecond)

	client.Unsubscribe(ch)

	// Let any in-flight attempt finish, then make sure no more arrive.
	time.Sleep(50 * time.Millisecond)
	settled := connections.Load()
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, settled, connections.Load(), "retry loop should have stopped")
}

func TestSubscribeReorgReconnects(t *testing.T) {
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := connections.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		startStream(w)
		writeEvent(t, w, chaintracks.ReorgEvent{Depth: 1, NewTip: tipAt(10)})
		<-r.Context().Done()
	}))
	defer server.Close()

	client := newFastClient(server.URL)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ch := client.SubscribeReorg(ctx)
	select {
	case ev := <-ch:
		require.NotNil(t, ev)
		assert.Equal(t, uint32(10), ev.NewTip.Height)
	case <-time.After(sseTestTimeout):
		t.Fatal("timeout waiting for reorg event after reconnect")
	}
}

func TestSubscribeReconnectsWhenStreamGoesIdle(t *testing.T) {
	var connections atomic.Uint32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := connections.Add(1)
		startStream(w)
		writeTip(t, w, tipAt(n))
		// Stall: keep the TCP connection open but never write again.
		<-r.Context().Done()
	}))
	defer server.Close()

	client := newFastClient(server.URL)
	client.idleTimeout = 100 * time.Millisecond
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ch := client.Subscribe(ctx)
	waitForTip(t, ch, 1)
	waitForTip(t, ch, 2) // only possible after the idle timeout forced a reconnect
	assert.GreaterOrEqual(t, connections.Load(), uint32(2))
}

func TestKeepalivesPreventIdleTimeout(t *testing.T) {
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connections.Add(1)
		startStream(w)
		writeTip(t, w, tipAt(1))

		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
					return
				}
				w.(http.Flusher).Flush()
			}
		}
	}))
	defer server.Close()

	client := newFastClient(server.URL)
	client.idleTimeout = 100 * time.Millisecond
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ch := client.Subscribe(ctx)
	waitForTip(t, ch, 1)

	// Several idle timeouts' worth of keepalive-only traffic must not trigger a reconnect.
	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, int32(1), connections.Load(), "keepalives should keep the stream alive")
}

// newTipServer serves /v2/tip/stream via stream and /v2/tip with a fixed REST tip.
func newTipServer(t *testing.T, restHeight uint32, stream http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/tip/stream", stream)
	mux.HandleFunc("/v2/tip", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tipAt(restHeight))
	})
	return httptest.NewServer(mux)
}

func TestGetTipFallsBackToRESTWhileStreamDown(t *testing.T) {
	const restHeight = 500
	var connections atomic.Int32
	server := newTipServer(t, restHeight, func(w http.ResponseWriter, _ *http.Request) {
		if connections.Add(1) > 1 {
			w.WriteHeader(http.StatusServiceUnavailable) // stream stays down
			return
		}
		startStream(w)
		writeTip(t, w, tipAt(1))
		// first connection drops here
	})
	defer server.Close()

	client := newFastClient(server.URL)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ch := client.Subscribe(ctx)
	waitForTip(t, ch, 1)

	// Once the stream is down the stale cached tip (1) must not be served.
	require.Eventually(t, func() bool {
		tip := client.GetTip(ctx)
		return tip != nil && tip.Height == restHeight
	}, sseTestTimeout, 5*time.Millisecond, "GetTip should fall back to REST while the stream is down")

	height, err := client.CurrentHeight(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint32(restHeight), height)
}

func TestGetTipCacheRepopulatedAfterReconnectWithSameTip(t *testing.T) {
	const restHeight = 500
	var connections atomic.Int32
	server := newTipServer(t, restHeight, func(w http.ResponseWriter, r *http.Request) {
		n := connections.Add(1)
		startStream(w)
		writeTip(t, w, tipAt(1)) // the server resends the same current tip on every connect
		if n == 1 {
			return // drop the first connection
		}
		<-r.Context().Done()
	})
	defer server.Close()

	client := newFastClient(server.URL)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ch := client.Subscribe(ctx)
	waitForTip(t, ch, 1)

	require.Eventually(t, func() bool { return connections.Load() >= 2 }, sseTestTimeout, 5*time.Millisecond)

	// The deduped tip is not broadcast again, but it must still repopulate the cache
	// that was cleared on disconnect; otherwise GetTip would keep hitting REST (500).
	require.Eventually(t, func() bool {
		tip := client.GetTip(ctx)
		return tip != nil && tip.Height == 1
	}, sseTestTimeout, 5*time.Millisecond, "cache should be repopulated from the resent tip")
}

func TestWithIdleTimeout(t *testing.T) {
	assert.Equal(t, defaultIdleTimeout, New("localhost:1").idleTimeout)
	assert.Equal(t, 5*time.Second, New("localhost:1", WithIdleTimeout(5*time.Second)).idleTimeout)
	assert.Equal(t, defaultIdleTimeout, New("localhost:1", WithIdleTimeout(0)).idleTimeout, "non-positive ignored")
	assert.Equal(t, defaultIdleTimeout, (&Client{}).idleTimeoutOrDefault(), "zero-value client falls back")
}

func TestJitter(t *testing.T) {
	for range 100 {
		d := jitter(time.Second)
		assert.GreaterOrEqual(t, d, 500*time.Millisecond)
		assert.LessOrEqual(t, d, time.Second)
	}
	assert.Equal(t, time.Duration(0), jitter(0))
}

func TestBackoffBoundsDefaultsForZeroValueClient(t *testing.T) {
	minB, maxB := (&Client{}).backoffBounds()
	assert.Equal(t, defaultMinBackoff, minB)
	assert.Equal(t, defaultMaxBackoff, maxB)
}
