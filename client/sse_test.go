package client

import (
	"context"
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
	return &chaintracks.BlockHeader{
		Header: &block.Header{},
		Height: height,
		Hash:   chainhash.Hash{byte(height)},
	}
}

func writeTip(t *testing.T, w http.ResponseWriter, tip *chaintracks.BlockHeader) {
	t.Helper()
	data, err := json.Marshal(tip)
	require.NoError(t, err)
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	require.NoError(t, err)
	w.(http.Flusher).Flush()
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
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := connections.Add(1)
		startStream(w)
		writeTip(t, w, tipAt(uint32(n)))
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
		data, err := json.Marshal(chaintracks.ReorgEvent{Depth: 1, NewTip: tipAt(10)})
		require.NoError(t, err)
		_, err = fmt.Fprintf(w, "data: %s\n\n", data)
		require.NoError(t, err)
		w.(http.Flusher).Flush()
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
