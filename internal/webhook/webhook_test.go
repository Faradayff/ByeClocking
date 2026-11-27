package webhook

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allEnv lists every WEBHOOK_* variable the client understands.
var allEnv = []string{
	EnvURL, EnvTitle, EnvTitleField, EnvTextField, EnvLevel,
	EnvTimeout, EnvRetryMax, EnvRetryBackoff, EnvHeaders,
}

// withEnv forces every webhook env var to "" (so each one falls back to its
// default, independent of the host environment) and then applies the given
// overrides. It returns the resulting client and any parsing error.
func withEnv(t *testing.T, overrides map[string]string) (*Client, error) {
	t.Helper()
	for _, k := range allEnv {
		if _, ok := overrides[k]; !ok {
			t.Setenv(k, "")
		}
	}
	for k, v := range overrides {
		t.Setenv(k, v)
	}
	return FromEnv()
}

// testServer returns an httptest server that counts requests, plus a pointer
// to the counter and helpers for the test.
func testServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *int32) {
	t.Helper()
	var count int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&count, 1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &count
}

func ctxOK() context.Context { return context.Background() }

func countOf(c *int32) int { return int(atomic.LoadInt32(c)) }

// ---------------------------------------------------------------------------
// FromEnv parsing / validation
// ---------------------------------------------------------------------------

func TestFromEnv_DisabledWhenNoURL(t *testing.T) {
	c, err := withEnv(t, nil)
	require.NoError(t, err)
	assert.False(t, c.IsEnabled(), "client must be disabled when WEBHOOK_URL is empty")

	// A disabled client must be safe to call (a no-op).
	neverPanic := func() (recovered bool) {
		defer func() {
			if r := recover(); r != nil {
				recovered = true
			}
		}()
		c.Notify(ctxOK(), slog.LevelError, "should not send")
		return
	}()
	assert.False(t, neverPanic, "Notify on a disabled client must be a safe no-op")
}

func TestFromEnv_Defaults(t *testing.T) {
	srv, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {})
	c, err := withEnv(t, map[string]string{EnvURL: srv.URL})
	require.NoError(t, err)

	assert.True(t, c.IsEnabled())
	assert.Equal(t, DefaultTitle, c.title)
	assert.Equal(t, "title", c.titleKey)
	assert.Equal(t, "text", c.textKey)
	assert.Equal(t, slog.LevelWarn, c.level)
	assert.Equal(t, 3, c.retryMax)
	assert.Equal(t, 2*time.Second, c.backoff)
	assert.Equal(t, 10*time.Second, c.http.Timeout)
	assert.Empty(t, c.headers)
}

func TestFromEnv_Overrides(t *testing.T) {
	srv, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {})
	c, err := withEnv(t, map[string]string{
		EnvURL:          srv.URL,
		EnvTitle:        "MySvc",
		EnvTitleField:   "subject",
		EnvTextField:    "detail",
		EnvLevel:        "debug",
		EnvTimeout:      "7s",
		EnvRetryMax:     "5",
		EnvRetryBackoff: "100ms",
	})
	require.NoError(t, err)
	assert.Equal(t, "MySvc", c.title)
	assert.Equal(t, "subject", c.titleKey)
	assert.Equal(t, "detail", c.textKey)
	assert.Equal(t, slog.LevelDebug, c.level)
	assert.Equal(t, 5, c.retryMax)
	assert.Equal(t, 100*time.Millisecond, c.backoff)
	assert.Equal(t, 7*time.Second, c.http.Timeout)
}

func TestFromEnv_BareIntSeconds(t *testing.T) {
	srv, _ := testServer(t, func(w http.ResponseWriter, _ *http.Request) {})
	c, err := withEnv(t, map[string]string{EnvURL: srv.URL, EnvTimeout: "6", EnvRetryBackoff: "3"})
	require.NoError(t, err)
	assert.Equal(t, 6*time.Second, c.http.Timeout)
	assert.Equal(t, 3*time.Second, c.backoff)
}

func TestFromEnv_InvalidURL(t *testing.T) {
	_, err := withEnv(t, map[string]string{EnvURL: "ftp://example.com/hook"})
	assert.Error(t, err, "non-http(s) scheme must be rejected")

	_, err = withEnv(t, map[string]string{EnvURL: "://no-scheme"})
	assert.Error(t, err, "URL without scheme/host must be rejected")
}

func TestFromEnv_InvalidValues(t *testing.T) {
	base := map[string]string{EnvURL: "http://example.com/hook"}

	_, err := withEnv(t, merge(base, EnvLevel, "VERBOSE"))
	assert.Error(t, err, "unknown level must be rejected")

	_, err = withEnv(t, merge(base, EnvTimeout, "soon"))
	assert.Error(t, err, "non-duration timeout must be rejected")

	_, err = withEnv(t, merge(base, EnvRetryMax, "abc"))
	assert.Error(t, err, "non-integer retry count must be rejected")

	_, err = withEnv(t, merge(base, EnvRetryMax, "-1"))
	assert.Error(t, err, "negative retry count must be rejected")

	_, err = withEnv(t, merge(base, EnvHeaders, "NoColonHere"))
	assert.Error(t, err, "header without a separator must be rejected")
}

func TestParseHeaders(t *testing.T) {
	h, err := parseHeaders("Authorization: Bearer abc; X-Foo: bar ;")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"Authorization": "Bearer abc", "X-Foo": "bar"}, h)

	_, err = parseHeaders("Broken")
	assert.Error(t, err)
}

func merge(base map[string]string, k, v string) map[string]string {
	out := make(map[string]string, len(base)+1)
	for key, val := range base {
		out[key] = val
	}
	out[k] = v
	return out
}

// ---------------------------------------------------------------------------
// Notify behaviour (level filter, body, headers, retries)
// ---------------------------------------------------------------------------

func TestNotify_LevelFilter(t *testing.T) {
	srv, count := testServer(t, func(w http.ResponseWriter, _ *http.Request) {})
	c, err := withEnv(t, map[string]string{EnvURL: srv.URL, EnvLevel: "warn"})
	require.NoError(t, err)

	c.Notify(ctxOK(), slog.LevelInfo, "below threshold")
	c.Notify(ctxOK(), slog.LevelDebug, "below threshold")
	assert.Equal(t, 0, countOf(count), "INFO/DEBUG must not be sent at level WARN")

	c.Notify(ctxOK(), slog.LevelWarn, "at threshold")
	c.Notify(ctxOK(), slog.LevelError, "above threshold")
	assert.Equal(t, 2, countOf(count), "WARN and ERROR must be sent at level WARN")
}

func TestNotify_EmptyMessageIsDropped(t *testing.T) {
	srv, count := testServer(t, func(w http.ResponseWriter, _ *http.Request) {})
	c, err := withEnv(t, map[string]string{EnvURL: srv.URL, EnvLevel: "error"})
	require.NoError(t, err)

	c.Notify(ctxOK(), slog.LevelError, "   ")
	assert.Equal(t, 0, countOf(count), "blank messages must not be sent")
}

func TestNotify_SendsCorrectBodyAndHeaders(t *testing.T) {
	var got bodyAndHeaders
	srv, _ := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		got.contentType = r.Header.Get("Content-Type")
		got.auth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(b, &got.body))
	})

	c, err := withEnv(t, map[string]string{
		EnvURL:     srv.URL,
		EnvTitle:   "ByeClocking",
		EnvLevel:   "error",
		EnvHeaders: "Authorization: Bearer secret-token",
	})
	require.NoError(t, err)

	c.Notify(ctxOK(), slog.LevelError, "clock in failed")

	assert.Equal(t, "application/json", got.contentType)
	assert.Equal(t, "Bearer secret-token", got.auth, "custom headers must be forwarded")
	assert.Equal(t, "ByeClocking", got.body["title"])
	assert.Equal(t, "clock in failed", got.body["text"])
}

func TestNotify_CustomFieldNames(t *testing.T) {
	var got map[string]string
	srv, _ := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(b, &got))
	})

	c, err := withEnv(t, map[string]string{
		EnvURL:        srv.URL,
		EnvTitleField: "heading",
		EnvTextField:  "message",
		EnvLevel:      "error",
	})
	require.NoError(t, err)

	c.Notify(ctxOK(), slog.LevelError, "hello")
	assert.Equal(t, map[string]string{"heading": "ByeClocking", "message": "hello"}, got)
}

func TestNotify_RetriesTransientThenSucceeds(t *testing.T) {
	var failedFirst int32
	srv, count := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if atomic.CompareAndSwapInt32(&failedFirst, 0, 1) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	c, err := withEnv(t, map[string]string{
		EnvURL:          srv.URL,
		EnvLevel:        "error",
		EnvRetryMax:     "2",
		EnvRetryBackoff: "0s",
	})
	require.NoError(t, err)

	c.Notify(ctxOK(), slog.LevelError, "retry me")

	assert.Equal(t, 2, countOf(count), "should succeed on the 2nd attempt")
}

func TestNotify_RetriesExhausted(t *testing.T) {
	srv, count := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway) // 502 → transient
	})

	c, err := withEnv(t, map[string]string{
		EnvURL:          srv.URL,
		EnvLevel:        "error",
		EnvRetryMax:     "2", // 1 initial + 2 retries = 3 attempts
		EnvRetryBackoff: "0s",
	})
	require.NoError(t, err)

	c.Notify(ctxOK(), slog.LevelError, "never succeeds")
	assert.Equal(t, 3, countOf(count), "5xx errors must be retried up to the limit")
}

func TestNotify_PermanentFailureNotRetried(t *testing.T) {
	srv, count := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound) // 404 → permanent
	})

	c, err := withEnv(t, map[string]string{
		EnvURL:          srv.URL,
		EnvLevel:        "error",
		EnvRetryMax:     "4",
		EnvRetryBackoff: "0s",
	})
	require.NoError(t, err)

	c.Notify(ctxOK(), slog.LevelError, "bad token")
	assert.Equal(t, 1, countOf(count), "404 is permanent and must not be retried")
}

func TestNotify_RetriesRateLimit(t *testing.T) {
	var failedFirst int32
	srv, count := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if atomic.CompareAndSwapInt32(&failedFirst, 0, 1) {
			w.WriteHeader(http.StatusTooManyRequests) // 429 → transient
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	c, err := withEnv(t, map[string]string{
		EnvURL:          srv.URL,
		EnvLevel:        "error",
		EnvRetryMax:     "2",
		EnvRetryBackoff: "0s",
	})
	require.NoError(t, err)

	c.Notify(ctxOK(), slog.LevelError, "rate limited")
	assert.Equal(t, 2, countOf(count), "429 must be retried")
}

func TestNotify_Timeout(t *testing.T) {
	srv, count := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond) // longer than the 50ms timeout
		w.WriteHeader(http.StatusOK)
	})

	c, err := withEnv(t, map[string]string{
		EnvURL:          srv.URL,
		EnvLevel:        "error",
		EnvTimeout:      "50ms",
		EnvRetryMax:     "0",
		EnvRetryBackoff: "0s",
	})
	require.NoError(t, err)

	// Must not hang or panic even though the server is slower than the timeout.
	done := make(chan struct{})
	go func() {
		c.Notify(ctxOK(), slog.LevelError, "timeout please")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Notify did not return within 3s despite a 50ms client timeout")
	}
	assert.Equal(t, 1, countOf(count), "the request should have been attempted")
}

func TestNotify_ContextCanceled(t *testing.T) {
	block := make(chan struct{})
	srv, count := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		<-block // hold the request until the test releases it
		w.WriteHeader(http.StatusOK)
	})

	c, err := withEnv(t, map[string]string{
		EnvURL:          srv.URL,
		EnvLevel:        "error",
		EnvTimeout:      "5s",
		EnvRetryMax:     "3",
		EnvRetryBackoff: "200ms",
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.Notify(ctx, slog.LevelError, "should be canceled")
		close(done)
	}()

	// Give the first attempt time to fire, then cancel.
	time.Sleep(150 * time.Millisecond)
	cancel()
	close(block)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Notify did not return after context cancellation")
	}
	// At least the first (blocked) request must have been attempted.
	assert.GreaterOrEqual(t, countOf(count), 1)
}

type bodyAndHeaders struct {
	contentType string
	auth        string
	body        map[string]string
}
