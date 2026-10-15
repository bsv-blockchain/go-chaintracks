package client

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

type recordingHandler struct {
	store *logStore
	attrs map[string]any
}

func (h recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h recordingHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]any, len(h.attrs)+r.NumAttrs())
	for k, v := range h.attrs {
		attrs[k] = v
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Resolve().Any()
		return true
	})
	h.store.add(logRecord{level: r.Level, msg: r.Message, attrs: attrs})
	return nil
}

func (h recordingHandler) WithAttrs(as []slog.Attr) slog.Handler {
	attrs := make(map[string]any, len(h.attrs)+len(as))
	for k, v := range h.attrs {
		attrs[k] = v
	}
	for _, a := range as {
		attrs[a.Key] = a.Value.Resolve().Any()
	}
	return recordingHandler{store: h.store, attrs: attrs}
}

func (h recordingHandler) WithGroup(string) slog.Handler { return h }

func newRecordingLogger() (*slog.Logger, *logStore) {
	store := &logStore{}
	return slog.New(recordingHandler{store: store}), store
}

func TestLogsWarnOnConnectFailureWithStatusAndRetryDelay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	logger, logs := newRecordingLogger()
	client := New(server.URL, WithLogger(logger))
	client.minBackoff = 5 * time.Millisecond
	client.maxBackoff = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	_ = client.Subscribe(ctx)

	require.Eventually(t, func() bool { return logs.has(slog.LevelWarn, "stream connect failed") },
		sseTestTimeout, 5*time.Millisecond)

	rec, _ := logs.find(slog.LevelWarn, "stream connect failed")
	assert.Equal(t, "/v2/tip/stream", rec.attrs["path"])
	assert.Contains(t, fmt.Sprint(rec.attrs["error"]), "503", "HTTP status should be logged")
	assert.IsType(t, time.Duration(0), rec.attrs["retry_in"])
	assert.Positive(t, rec.attrs["retry_in"])
	assert.Zero(t, logs.count(slog.LevelInfo), "no recovery logged while still failing")
}

func TestLogsInfoWithAttemptsAndDowntimeWhenReconnectSucceeds(t *testing.T) {
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if connections.Add(1) <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		startStream(w)
		<-r.Context().Done()
	}))
	defer server.Close()

	logger, logs := newRecordingLogger()
	client := New(server.URL, WithLogger(logger))
	client.minBackoff = 5 * time.Millisecond
	client.maxBackoff = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	_ = client.Subscribe(ctx)

	require.Eventually(t, func() bool { return logs.has(slog.LevelInfo, "stream reconnected") },
		sseTestTimeout, 5*time.Millisecond)

	rec, _ := logs.find(slog.LevelInfo, "stream reconnected")
	assert.Equal(t, "/v2/tip/stream", rec.attrs["path"])
	assert.EqualValues(t, 3, rec.attrs["attempts"], "two failures then the successful attempt")
	downtime, ok := rec.attrs["downtime"].(time.Duration)
	require.True(t, ok, "downtime should be a duration")
	assert.Positive(t, downtime)
	assert.Equal(t, 2, logs.count(slog.LevelWarn), "one warning per failed attempt")
}

func TestLogsWarnWhenEstablishedStreamDrops(t *testing.T) {
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

	logger, logs := newRecordingLogger()
	client := New(server.URL, WithLogger(logger))
	client.minBackoff = 5 * time.Millisecond
	client.maxBackoff = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	_ = client.Subscribe(ctx)

	require.Eventually(t, func() bool { return logs.has(slog.LevelInfo, "stream reconnected") },
		sseTestTimeout, 5*time.Millisecond)

	rec, ok := logs.find(slog.LevelWarn, "stream dropped")
	require.True(t, ok, "a stream that was connected and then ended is a drop, not a connect failure")
	assert.Equal(t, "/v2/tip/stream", rec.attrs["path"])
	assert.NotNil(t, rec.attrs["error"])
	assert.False(t, logs.has(slog.LevelWarn, "stream connect failed"))

	reconnected, _ := logs.find(slog.LevelInfo, "stream reconnected")
	assert.EqualValues(t, 1, reconnected.attrs["attempts"])
}

func TestLogsWarnWhenStreamIdleTimeoutFires(t *testing.T) {
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connections.Add(1)
		startStream(w)
		<-r.Context().Done() // stall: never write
	}))
	defer server.Close()

	logger, logs := newRecordingLogger()
	client := New(server.URL, WithLogger(logger), WithIdleTimeout(100*time.Millisecond))
	client.minBackoff = 5 * time.Millisecond
	client.maxBackoff = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	_ = client.Subscribe(ctx)

	require.Eventually(t, func() bool { return logs.has(slog.LevelWarn, "stream idle timeout") },
		sseTestTimeout, 5*time.Millisecond)

	rec, _ := logs.find(slog.LevelWarn, "stream idle timeout")
	assert.Equal(t, "/v2/tip/stream", rec.attrs["path"])
	assert.Positive(t, rec.attrs["retry_in"])
}

func TestReorgStreamLogsUseReorgPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	logger, logs := newRecordingLogger()
	client := New(server.URL, WithLogger(logger))
	client.minBackoff = 5 * time.Millisecond
	client.maxBackoff = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	_ = client.SubscribeReorg(ctx)

	require.Eventually(t, func() bool { return logs.has(slog.LevelWarn, "stream connect failed") },
		sseTestTimeout, 5*time.Millisecond)
	rec, _ := logs.find(slog.LevelWarn, "stream connect failed")
	assert.Equal(t, "/v2/reorg/stream", rec.attrs["path"])
}

func TestNoLoggerDiscardsAndDoesNotPanic(t *testing.T) {
	assert.NotNil(t, (&Client{}).loggerOrDiscard())
	assert.NotNil(t, New("localhost:1", WithLogger(nil)).loggerOrDiscard())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := newFastClient(server.URL) // no logger configured
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_ = client.Subscribe(ctx)
	time.Sleep(50 * time.Millisecond) // let it fail and log to the discard handler
}

func TestTipStreamStatus(t *testing.T) {
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if connections.Add(1) > 1 {
			w.WriteHeader(http.StatusServiceUnavailable) // stream stays down after the drop
			return
		}
		startStream(w)
		writeTip(t, w, tipAt(1))
		<-r.Context().Done()
	}))
	defer server.Close()

	client := newFastClient(server.URL)

	before := client.TipStreamStatus()
	assert.False(t, before.Connected, "not connected before the first subscriber")
	assert.True(t, before.LastActivity.IsZero())

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ch := client.Subscribe(ctx)
	waitForTip(t, ch, 1)

	status := client.TipStreamStatus()
	assert.True(t, status.Connected)
	assert.WithinDuration(t, time.Now(), status.LastActivity, sseTestTimeout)

	// Last subscriber leaves: the stream is stopped and reported as disconnected.
	client.Unsubscribe(ch)
	require.Eventually(t, func() bool { return !client.TipStreamStatus().Connected },
		sseTestTimeout, 5*time.Millisecond)
	assert.False(t, client.ReorgStreamStatus().Connected, "reorg stream was never started")
}

func TestTipStreamStatusReportsDisconnectWhileRetrying(t *testing.T) {
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if connections.Add(1) > 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		startStream(w)
		writeTip(t, w, tipAt(1))
		// first connection drops here, later attempts are rejected
	}))
	defer server.Close()

	client := newFastClient(server.URL)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ch := client.Subscribe(ctx)
	waitForTip(t, ch, 1)

	require.Eventually(t, func() bool { return !client.TipStreamStatus().Connected },
		sseTestTimeout, 5*time.Millisecond, "a dropped stream must be reported as disconnected")
	assert.False(t, client.TipStreamStatus().LastActivity.IsZero(), "last activity is kept for diagnostics")
}

func TestStreamStatusLastActivityAdvancesOnKeepalive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startStream(w)
		ticker := time.NewTicker(10 * time.Millisecond)
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
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_ = client.SubscribeReorg(ctx)

	var first time.Time
	require.Eventually(t, func() bool {
		first = client.ReorgStreamStatus().LastActivity
		return !first.IsZero()
	}, sseTestTimeout, 5*time.Millisecond)
	assert.True(t, client.ReorgStreamStatus().Connected)

	require.Eventually(t, func() bool {
		return client.ReorgStreamStatus().LastActivity.After(first)
	}, sseTestTimeout, 5*time.Millisecond, "keepalives should advance LastActivity")
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
