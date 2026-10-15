package client

import (
	"sync"
	"time"
)

// StreamStatus is a point-in-time view of an SSE stream's health, suitable for
// exporting as a metric or backing a readiness probe.
type StreamStatus struct {
	// Connected reports whether the stream currently has an open connection.
	// It is false before the first subscriber, while reconnecting, and after the last subscriber leaves.
	Connected bool
	// LastActivity is when the last line (event or keepalive) was received.
	// It is the zero time if nothing has been received yet.
	LastActivity time.Time
}

// streamState tracks the health of one SSE stream. It is safe for concurrent use.
//
// Each run of the stream owns a generation number so that a run that is
// shutting down cannot overwrite the state of a newer run started right after it.
type streamState struct {
	mu           sync.Mutex
	gen          uint64
	connected    bool
	lastActivity time.Time
}

// begin starts a new run, marking the stream disconnected, and returns its generation.
func (s *streamState) begin() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.gen++
	s.connected = false

	return s.gen
}

// setConnected records the connection state for run gen. Stale runs are ignored.
func (s *streamState) setConnected(gen uint64, connected bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.gen == gen {
		s.connected = connected
	}
}

// touch records that a line was received on run gen. Stale runs are ignored.
func (s *streamState) touch(gen uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.gen == gen {
		s.lastActivity = time.Now()
	}
}

func (s *streamState) status() StreamStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	return StreamStatus{Connected: s.connected, LastActivity: s.lastActivity}
}

// TipStreamStatus reports the health of the tip SSE stream (/v2/tip/stream).
// The stream is only running while there is at least one Subscribe subscriber.
func (c *Client) TipStreamStatus() StreamStatus {
	return c.tipStream.status()
}

// ReorgStreamStatus reports the health of the reorg SSE stream (/v2/reorg/stream).
// The stream is only running while there is at least one SubscribeReorg subscriber.
func (c *Client) ReorgStreamStatus() StreamStatus {
	return c.reorgStream.status()
}
