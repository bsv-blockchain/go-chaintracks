package client

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	tipStreamPath   = "/v2/tip/stream"
	reorgStreamPath = "/v2/reorg/stream"

	msgConnectFailed = "stream connect failed"
	msgDropped       = "stream dropped"
	msgIdleTimeout   = "stream idle timeout"
	msgReconnected   = "stream reconnected"
)

type logRecord struct {
	level slog.Level
	msg   string
	attrs map[string]any
}

// logStore collects slog records so tests can assert on what was logged.
type logStore struct {
	mu   sync.Mutex
	recs []logRecord
}

func (s *logStore) add(r logRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recs = append(s.recs, r)
}

// find returns the first record with the given level and message.
func (s *logStore) find(level slog.Level, msg string) (logRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.recs {
		if r.level == level && r.msg == msg {
			return r, true
		}
	}
	return logRecord{}, false
}

func (s *logStore) has(level slog.Level, msg string) bool {
	_, ok := s.find(level, msg)
	return ok
}

func (s *logStore) count(level slog.Level) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.recs {
		if r.level == level {
			n++
		}
	}
	return n
}

// awaitLog waits for a record with the given level and message and returns it.
func (s *logStore) awaitLog(t *testing.T, level slog.Level, msg string) logRecord {
	t.Helper()
	eventually(t, func() bool { return s.has(level, msg) }, "expected %v log %q", level, msg)
	rec, _ := s.find(level, msg)
	return rec
}

type recordingHandler struct {
	store *logStore
	attrs map[string]any
}

// withAttrs returns the handler's attributes merged with extra ones, as a new map.
func (h recordingHandler) withAttrs(extra func(yield func(slog.Attr) bool)) map[string]any {
	merged := maps.Clone(h.attrs)
	if merged == nil {
		merged = map[string]any{}
	}
	extra(func(a slog.Attr) bool {
		merged[a.Key] = a.Value.Resolve().Any()
		return true
	})
	return merged
}

func (h recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.store.add(logRecord{level: r.Level, msg: r.Message, attrs: h.withAttrs(r.Attrs)})
	return nil
}

func (h recordingHandler) WithAttrs(as []slog.Attr) slog.Handler {
	attrs := h.withAttrs(func(yield func(slog.Attr) bool) {
		for _, a := range as {
			if !yield(a) {
				return
			}
		}
	})
	return recordingHandler{store: h.store, attrs: attrs}
}

func (h recordingHandler) WithGroup(string) slog.Handler { return h }

// newLoggedTestClient returns a test client wired to a recording logger.
func newLoggedTestClient(url string, opts ...Option) (*Client, *logStore) {
	store := &logStore{}
	logger := slog.New(recordingHandler{store: store})
	return newTestClient(url, append([]Option{WithLogger(logger)}, opts...)...), store
}

func TestLogsWarnOnConnectFailureWithStatusAndRetryDelay(t *testing.T) {
	server := newStreamServer(t, reject(http.StatusServiceUnavailable))
	client, logs := newLoggedTestClient(server.URL)

	_ = client.Subscribe(t.Context())

	rec := logs.awaitLog(t, slog.LevelWarn, msgConnectFailed)
	assert.Equal(t, tipStreamPath, rec.attrs["path"])
	assert.Contains(t, fmt.Sprint(rec.attrs["error"]), "503", "HTTP status should be logged")
	assert.IsType(t, time.Duration(0), rec.attrs["retry_in"])
	assert.Positive(t, rec.attrs["retry_in"])
	assert.Zero(t, logs.count(slog.LevelInfo), "no recovery logged while still failing")
}

func TestLogsInfoWithAttemptsAndDowntimeWhenReconnectSucceeds(t *testing.T) {
	server := newStreamServer(t, reject(http.StatusInternalServerError), reject(http.StatusInternalServerError), holdAfter())
	client, logs := newLoggedTestClient(server.URL)

	_ = client.Subscribe(t.Context())

	rec := logs.awaitLog(t, slog.LevelInfo, msgReconnected)
	assert.Equal(t, tipStreamPath, rec.attrs["path"])
	assert.EqualValues(t, 3, rec.attrs["attempts"], "two failures then the successful attempt")
	downtime, ok := rec.attrs["downtime"].(time.Duration)
	require.True(t, ok, "downtime should be a duration")
	assert.Positive(t, downtime)
	assert.Equal(t, 2, logs.count(slog.LevelWarn), "one warning per failed attempt")
}

func TestLogsWarnWhenEstablishedStreamDrops(t *testing.T) {
	server := newStreamServer(t, dropAfter(tipAt(1)), holdAfter(tipAt(2)))
	client, logs := newLoggedTestClient(server.URL)

	_ = client.Subscribe(t.Context())

	reconnected := logs.awaitLog(t, slog.LevelInfo, msgReconnected)
	assert.EqualValues(t, 1, reconnected.attrs["attempts"])

	rec, ok := logs.find(slog.LevelWarn, msgDropped)
	require.True(t, ok, "a stream that was connected and then ended is a drop, not a connect failure")
	assert.Equal(t, tipStreamPath, rec.attrs["path"])
	assert.NotNil(t, rec.attrs["error"])
	assert.False(t, logs.has(slog.LevelWarn, msgConnectFailed))
}

func TestLogsWarnWhenStreamIdleTimeoutFires(t *testing.T) {
	server := newStreamServer(t, holdAfter()) // stalls: never writes
	client, logs := newLoggedTestClient(server.URL, WithIdleTimeout(100*time.Millisecond))

	_ = client.Subscribe(t.Context())

	rec := logs.awaitLog(t, slog.LevelWarn, msgIdleTimeout)
	assert.Equal(t, tipStreamPath, rec.attrs["path"])
	assert.Positive(t, rec.attrs["retry_in"])
}

func TestReorgStreamLogsUseReorgPath(t *testing.T) {
	server := newStreamServer(t, reject(http.StatusBadGateway))
	client, logs := newLoggedTestClient(server.URL)

	_ = client.SubscribeReorg(t.Context())

	rec := logs.awaitLog(t, slog.LevelWarn, msgConnectFailed)
	assert.Equal(t, reorgStreamPath, rec.attrs["path"])
}

func TestNoLoggerDiscardsAndDoesNotPanic(t *testing.T) {
	assert.NotNil(t, (&Client{}).loggerOrDiscard())
	assert.NotNil(t, New("localhost:1", WithLogger(nil)).loggerOrDiscard())

	server := newStreamServer(t, reject(http.StatusServiceUnavailable))
	client := newTestClient(server.URL) // no logger configured

	_ = client.Subscribe(t.Context())
	eventually(t, func() bool { return server.connections.Load() >= 2 }) // failed and logged to the discard handler
}

func TestTipStreamStatus(t *testing.T) {
	server := newStreamServer(t, holdAfter(tipAt(1)), reject(http.StatusServiceUnavailable))
	client := newTestClient(server.URL)

	before := client.TipStreamStatus()
	assert.False(t, before.Connected, "not connected before the first subscriber")
	assert.True(t, before.LastActivity.IsZero())

	ch := client.Subscribe(t.Context())
	waitForTip(t, ch, 1)

	status := client.TipStreamStatus()
	assert.True(t, status.Connected)
	assert.WithinDuration(t, time.Now(), status.LastActivity, sseTestTimeout)

	// Last subscriber leaves: the stream is stopped and reported as disconnected.
	client.Unsubscribe(ch)
	eventually(t, func() bool { return !client.TipStreamStatus().Connected })
	assert.False(t, client.ReorgStreamStatus().Connected, "reorg stream was never started")
}

func TestTipStreamStatusReportsDisconnectWhileRetrying(t *testing.T) {
	// First connection drops, later attempts are rejected.
	server := newStreamServer(t, dropAfter(tipAt(1)), reject(http.StatusServiceUnavailable))
	client := newTestClient(server.URL)

	waitForTip(t, client.Subscribe(t.Context()), 1)

	eventually(t, func() bool { return !client.TipStreamStatus().Connected }, "a dropped stream must be reported as disconnected")
	assert.False(t, client.TipStreamStatus().LastActivity.IsZero(), "last activity is kept for diagnostics")
}

func TestStreamStatusLastActivityAdvancesOnKeepalive(t *testing.T) {
	server := newStreamServer(t, withKeepalives(10*time.Millisecond))
	client := newTestClient(server.URL)

	_ = client.SubscribeReorg(t.Context())

	var first time.Time
	eventually(t, func() bool {
		first = client.ReorgStreamStatus().LastActivity
		return !first.IsZero()
	})
	assert.True(t, client.ReorgStreamStatus().Connected)

	eventually(t, func() bool {
		return client.ReorgStreamStatus().LastActivity.After(first)
	}, "keepalives should advance LastActivity")
}

func TestStreamStateIgnoresStaleRuns(t *testing.T) {
	var s streamState

	old := s.begin()
	s.setConnected(old, true)
	require.True(t, s.status().Connected)

	// A new run starts (stream stopped and restarted) before the old one finishes shutting down.
	current := s.begin()
	assert.False(t, s.status().Connected, "a new run starts disconnected")
	s.setConnected(current, true)

	s.setConnected(old, false) // the old run's cleanup arrives late
	s.touch(old)

	status := s.status()
	assert.True(t, status.Connected, "stale run must not mark the new run disconnected")
	assert.True(t, status.LastActivity.IsZero(), "stale run must not record activity")
}
