package client

import (
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

const (
	sseTestTimeout = 3 * time.Second
	pollInterval   = 5 * time.Millisecond

	// restTipHeight is the height newStreamServer serves from /v2/tip, so tests can
	// tell a REST fallback apart from a tip delivered over the stream.
	restTipHeight = 500
)

// newTestClient returns a client with short reconnect backoff so tests do not wait seconds.
// Pass options to override, e.g. WithLogger or WithIdleTimeout.
func newTestClient(url string, opts ...Option) *Client {
	c := New(url, opts...)
	c.minBackoff = pollInterval
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

// connBehavior describes how the test server treats one stream connection.
type connBehavior struct {
	// status, if non-zero, rejects the connection with this HTTP status.
	status int
	// events are written, in order, once the stream is open.
	events []any
	// hold keeps the connection open until the client disconnects. Without it the
	// server closes the connection right after writing events, simulating a drop.
	hold bool
	// keepalive, if positive and hold is set, writes a ": keepalive" comment at this interval.
	keepalive time.Duration
}

// reject refuses the connection with the given HTTP status.
func reject(status int) connBehavior { return connBehavior{status: status} }

// dropAfter writes events and then closes the connection.
func dropAfter(events ...any) connBehavior { return connBehavior{events: events} }

// holdAfter writes events and then keeps the connection open without writing again.
// With no events it simulates a stalled stream.
func holdAfter(events ...any) connBehavior { return connBehavior{events: events, hold: true} }

// withKeepalives writes events and then keeps the connection open, sending a keepalive every interval.
func withKeepalives(interval time.Duration, events ...any) connBehavior {
	return connBehavior{events: events, hold: true, keepalive: interval}
}

// streamServer is a test chaintracks server. Stream connections follow a script of
// behaviors, one per connection in order; connections beyond the script repeat its last entry.
type streamServer struct {
	*httptest.Server

	connections atomic.Int32
}

// newStreamServer serves both SSE streams according to script, and /v2/tip with a REST
// tip of restTipHeight. The server is closed when the test ends, after the client's
// context (t.Context) has been canceled.
func newStreamServer(t *testing.T, script ...connBehavior) *streamServer {
	t.Helper()
	require.NotEmpty(t, script, "script needs at least one behavior")

	s := &streamServer{}
	stream := func(w http.ResponseWriter, r *http.Request) {
		n := int(s.connections.Add(1))
		script[min(n, len(script))-1].serve(t, w, r)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v2/tip/stream", stream)
	mux.HandleFunc("/v2/reorg/stream", stream)
	mux.HandleFunc("/v2/tip", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tipAt(restTipHeight))
	})

	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)

	return s
}

func (b connBehavior) serve(t *testing.T, w http.ResponseWriter, r *http.Request) {
	if b.status != 0 {
		w.WriteHeader(b.status)
		return
	}

	startStream(w)
	for _, ev := range b.events {
		writeEvent(t, w, ev)
	}
	if !b.hold {
		return
	}

	if b.keepalive <= 0 {
		<-r.Context().Done()
		return
	}

	ticker := time.NewTicker(b.keepalive)
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
}

func startStream(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	w.(http.Flusher).Flush()
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

// recv waits for one value from ch, failing the test if it times out or ch is closed.
func recv[T any](t *testing.T, ch <-chan T) T {
	t.Helper()

	select {
	case v, ok := <-ch:
		require.True(t, ok, "channel closed")
		return v
	case <-time.After(sseTestTimeout):
		t.Fatal("timeout waiting for a value")
	}

	var zero T
	return zero
}

// waitForTip waits for the next tip on ch and checks its height.
func waitForTip(t *testing.T, ch <-chan *chaintracks.BlockHeader, height uint32) {
	t.Helper()
	assert.Equal(t, height, recv(t, ch).Height)
}

// eventually fails the test unless cond becomes true within sseTestTimeout.
func eventually(t *testing.T, cond func() bool, msgAndArgs ...any) {
	t.Helper()
	require.Eventually(t, cond, sseTestTimeout, pollInterval, msgAndArgs...)
}
