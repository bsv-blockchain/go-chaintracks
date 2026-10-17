package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
)

func TestSubscribeReconnectsWhenServerClosesStream(t *testing.T) {
	server := newStreamServer(t, dropAfter(tipAt(1)), holdAfter(tipAt(2)))
	client := newTestClient(server.URL)

	ch := client.Subscribe(t.Context())
	waitForTip(t, ch, 1)
	waitForTip(t, ch, 2)
	assert.Equal(t, uint32(2), client.GetTip(t.Context()).Height)
}

func TestSubscribeRetriesAfterServerError(t *testing.T) {
	server := newStreamServer(t, reject(http.StatusInternalServerError), reject(http.StatusInternalServerError), holdAfter(tipAt(7)))
	client := newTestClient(server.URL)

	ch := client.Subscribe(t.Context())
	waitForTip(t, ch, 7)
	assert.GreaterOrEqual(t, server.connections.Load(), int32(3))
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
				writeEvent(t, w, tipAt(h))
			case <-r.Context().Done():
				return
			}
		}
	}))
	t.Cleanup(server.Close)

	client := newTestClient(server.URL)
	firstCtx, cancelFirst := context.WithCancel(t.Context())
	defer cancelFirst()

	_ = client.Subscribe(firstCtx)
	second := client.Subscribe(t.Context())

	events <- 1
	waitForTip(t, second, 1)

	cancelFirst()
	eventually(t, func() bool {
		client.subMu.Lock()
		defer client.subMu.Unlock()
		return len(client.subscribers) == 1
	})

	events <- 2
	waitForTip(t, second, 2)
	assert.Equal(t, int32(1), connections.Load(), "stream must not restart")
}

func TestUnsubscribeLastStopsRetryLoop(t *testing.T) {
	server := newStreamServer(t, reject(http.StatusServiceUnavailable))
	client := newTestClient(server.URL)

	ch := client.Subscribe(t.Context())
	eventually(t, func() bool { return server.connections.Load() >= 2 })

	client.Unsubscribe(ch)

	// Let any in-flight attempt finish, then make sure no more arrive.
	time.Sleep(50 * time.Millisecond)
	settled := server.connections.Load()
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, settled, server.connections.Load(), "retry loop should have stopped")
}

func TestSubscribeReorgReconnects(t *testing.T) {
	server := newStreamServer(t, reject(http.StatusBadGateway), holdAfter(chaintracks.ReorgEvent{Depth: 1, NewTip: tipAt(10)}))
	client := newTestClient(server.URL)

	ev := recv(t, client.SubscribeReorg(t.Context()))
	require.NotNil(t, ev)
	assert.Equal(t, uint32(10), ev.NewTip.Height)
}

func TestSubscribeReconnectsWhenStreamGoesIdle(t *testing.T) {
	// Each connection delivers a tip and then stalls: open, but never writes again.
	server := newStreamServer(t, holdAfter(tipAt(1)), holdAfter(tipAt(2)))
	client := newTestClient(server.URL, WithIdleTimeout(100*time.Millisecond))

	ch := client.Subscribe(t.Context())
	waitForTip(t, ch, 1)
	waitForTip(t, ch, 2) // only possible after the idle timeout forced a reconnect
	assert.GreaterOrEqual(t, server.connections.Load(), int32(2))
}

func TestKeepalivesPreventIdleTimeout(t *testing.T) {
	server := newStreamServer(t, withKeepalives(20*time.Millisecond, tipAt(1)))
	client := newTestClient(server.URL, WithIdleTimeout(100*time.Millisecond))

	ch := client.Subscribe(t.Context())
	waitForTip(t, ch, 1)

	// Several idle timeouts' worth of keepalive-only traffic must not trigger a reconnect.
	time.Sleep(500 * time.Millisecond)
	assert.Equal(t, int32(1), server.connections.Load(), "keepalives should keep the stream alive")
}

func TestGetTipFallsBackToRESTWhileStreamDown(t *testing.T) {
	server := newStreamServer(t, dropAfter(tipAt(1)), reject(http.StatusServiceUnavailable))
	client := newTestClient(server.URL)

	waitForTip(t, client.Subscribe(t.Context()), 1)

	// Once the stream is down the stale cached tip (1) must not be served.
	eventually(t, func() bool {
		tip := client.GetTip(t.Context())
		return tip != nil && tip.Height == restTipHeight
	}, "GetTip should fall back to REST while the stream is down")

	height, err := client.CurrentHeight(t.Context())
	require.NoError(t, err)
	assert.Equal(t, uint32(restTipHeight), height)
}

func TestGetTipCacheRepopulatedAfterReconnectWithSameTip(t *testing.T) {
	// The server resends the same current tip on every connect.
	server := newStreamServer(t, dropAfter(tipAt(1)), holdAfter(tipAt(1)))
	client := newTestClient(server.URL)

	waitForTip(t, client.Subscribe(t.Context()), 1)
	eventually(t, func() bool { return server.connections.Load() >= 2 })

	// The deduped tip is not broadcast again, but it must still repopulate the cache
	// that was cleared on disconnect; otherwise GetTip would keep hitting REST (500).
	eventually(t, func() bool {
		tip := client.GetTip(t.Context())
		return tip != nil && tip.Height == 1
	}, "cache should be repopulated from the resent tip")
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
