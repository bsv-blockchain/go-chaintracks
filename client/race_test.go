package client

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bsv-blockchain/go-chaintracks/chaintracks"
)

const (
	raceWorkers    = 8
	raceIterations = 50
)

// hammer runs fn from raceWorkers goroutines, raceIterations times each, and waits for all.
func hammer(fn func()) {
	var wg sync.WaitGroup
	for range raceWorkers {
		wg.Go(func() {
			for range raceIterations {
				fn()
			}
		})
	}
	wg.Wait()
}

// These tests are meaningful under `go test -race`: they cross the first/last
// subscriber boundary from many goroutines, so every start/stop of the stream races
// with another subscribe, unsubscribe, broadcast and GetTip.

func TestConcurrentSubscribeUnsubscribeAcrossLastSubscriberBoundary(t *testing.T) {
	server := newStreamServer(t, holdAfter(tipAt(1)))
	client := newTestClient(server.URL)

	ctx := t.Context()
	go func() {
		for ctx.Err() == nil {
			_ = client.GetTip(ctx)
			_ = client.TipStreamStatus()
			time.Sleep(time.Millisecond)
		}
	}()

	hammer(func() {
		ch := client.Subscribe(t.Context())
		select {
		case <-ch:
		case <-time.After(time.Millisecond):
		}
		client.Unsubscribe(ch)
	})

	client.subMu.Lock()
	defer client.subMu.Unlock()
	assert.Empty(t, client.subscribers)
	assert.Nil(t, client.sseCancel, "stream must be stopped once the last subscriber is gone")
}

func TestConcurrentSubscribeReorgUnsubscribeAcrossLastSubscriberBoundary(t *testing.T) {
	server := newStreamServer(t, holdAfter(tipAt(1)))
	client := newTestClient(server.URL)

	hammer(func() {
		ch := client.SubscribeReorg(t.Context())
		select {
		case <-ch:
		case <-time.After(time.Millisecond):
		}
		client.UnsubscribeReorg(ch)
	})

	client.reorgSubMu.Lock()
	defer client.reorgSubMu.Unlock()
	assert.Empty(t, client.reorgSubscribers)
	assert.Nil(t, client.reorgSSECancel, "reorg stream must be stopped once the last subscriber is gone")
}

// A run that was stopped can still have a tip or reorg event in flight. It must not
// reach the cache or the subscribers of a stream that was started afterwards.
func TestStoppedRunCannotCacheOrBroadcast(t *testing.T) {
	client := New("localhost:1")
	stopped, stop := context.WithCancel(t.Context())
	stop()

	tipCh := make(chan *chaintracks.BlockHeader, 1)
	reorgCh := make(chan *chaintracks.ReorgEvent, 1)
	client.subscribers[tipCh] = struct{}{}
	client.reorgSubscribers[reorgCh] = struct{}{}

	assert.False(t, client.cacheTip(stopped, tipAt(1)))
	assert.Nil(t, client.currentTip, "stopped run must not repopulate the cache")

	client.broadcast(stopped, tipAt(1))
	client.reorgBroadcast(stopped, &chaintracks.ReorgEvent{NewTip: tipAt(1)})
	assert.Empty(t, tipCh, "stopped run must not broadcast tips")
	assert.Empty(t, reorgCh, "stopped run must not broadcast reorgs")

	// The same calls from a live run still work.
	assert.True(t, client.cacheTip(t.Context(), tipAt(2)))
	assert.Equal(t, uint32(2), client.currentTip.Height)

	// A stopped run's late disconnect must not wipe the tip cached since; a live run's does.
	client.clearTipIfLive(stopped)
	require.NotNil(t, client.currentTip, "stopped run must not clear the cache")
	client.clearTipIfLive(t.Context())
	assert.Nil(t, client.currentTip, "live run clears the cache on disconnect")
	require.True(t, client.cacheTip(t.Context(), tipAt(2)))

	client.broadcast(t.Context(), tipAt(2))
	client.reorgBroadcast(t.Context(), &chaintracks.ReorgEvent{NewTip: tipAt(2)})
	assert.Len(t, tipCh, 1)
	assert.Len(t, reorgCh, 1)
}

func TestSubscribeAfterStopRestartsStream(t *testing.T) {
	server := newStreamServer(t, holdAfter(tipAt(1)))
	client := newTestClient(server.URL)

	first := client.Subscribe(t.Context())
	waitForTip(t, first, 1)
	client.Unsubscribe(first)

	// A restarted stream must deliver to the new subscriber, not to a leftover
	// fan-out from the previous run.
	second := client.Subscribe(t.Context())
	waitForTip(t, second, 1)
	assert.Equal(t, int32(2), server.connections.Load())
}
