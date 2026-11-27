// Package webhook provides a generic, HTTP-based notification client.
//
// It is deliberately agnostic of the specific backend (any notification hub,
// webhook endpoint, alert service, etc.). The only hard requirement is that the
// remote endpoint accepts an HTTP POST. All behaviour (endpoint, message shape,
// severity threshold, retry policy, extra headers) is driven by WEBHOOK_*
// environment variables so the clocking configuration file stays 100% focused
// on clocking.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Defaults. Any of them can be overridden via a WEBHOOK_* environment variable.
const (
	DefaultTitle         = "ByeClocking"
	DefaultTitleField    = "title"
	DefaultTextField     = "text"
	DefaultTimeout       = 10 * time.Second
	DefaultRetryMax      = 3
	DefaultRetryBackoff  = 2 * time.Second
	DefaultLevel         = slog.LevelWarn
	maxBackoff           = 30 * time.Second
	maxResponseBodyBytes = 512
)

// Env var names.
const (
	EnvURL          = "WEBHOOK_URL"
	EnvTitle        = "WEBHOOK_TITLE"
	EnvTitleField   = "WEBHOOK_TITLE_FIELD"
	EnvTextField    = "WEBHOOK_TEXT_FIELD"
	EnvLevel        = "WEBHOOK_LEVEL"
	EnvTimeout      = "WEBHOOK_TIMEOUT"
	EnvRetryMax     = "WEBHOOK_RETRY_MAX"
	EnvRetryBackoff = "WEBHOOK_RETRY_BACKOFF"
	EnvHeaders      = "WEBHOOK_HEADERS"
)

// Client is a generic webhook notifier configured from environment variables.
// A zero-value / disabled client simply drops every notification (Notify is a
// no-op), which lets the rest of the app stay agnostic of whether a webhook is
// actually configured.
type Client struct {
	enabled  bool
	url      string
	title    string
	titleKey string
	textKey  string
	headers  map[string]string
	level    slog.Level
	http     *http.Client
	retryMax int
	backoff  time.Duration
}

// FromEnv builds a Client from WEBHOOK_* environment variables.
//
// If WEBHOOK_URL is empty/absent the returned client is disabled (IsEnabled
// false) and Notify acts as a no-op. If WEBHOOK_URL is present but any other
// value is invalid, an error is returned so the caller can fail fast rather
// than silently disable notifications.
func FromEnv() (*Client, error) {
	c := &Client{
		title:    DefaultTitle,
		titleKey: DefaultTitleField,
		textKey:  DefaultTextField,
		level:    DefaultLevel,
		retryMax: DefaultRetryMax,
		backoff:  DefaultRetryBackoff,
		headers:  make(map[string]string),
	}

	rawURL := strings.TrimSpace(os.Getenv(EnvURL))
	if rawURL == "" {
		return c, nil // notifications disabled
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid %s %q: expected a full http(s) URL", EnvURL, rawURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("invalid %s %q: scheme must be http or https", EnvURL, rawURL)
	}
	c.enabled = true
	c.url = rawURL

	if v := strings.TrimSpace(os.Getenv(EnvTitle)); v != "" {
		c.title = v
	}
	if v := strings.TrimSpace(os.Getenv(EnvTitleField)); v != "" {
		c.titleKey = v
	}
	if v := strings.TrimSpace(os.Getenv(EnvTextField)); v != "" {
		c.textKey = v
	}

	if rawLevel, ok := os.LookupEnv(EnvLevel); ok {
		if strings.TrimSpace(rawLevel) != "" {
			level, err := parseLevel(rawLevel)
			if err != nil {
				return nil, fmt.Errorf("invalid %s %q: %w", EnvLevel, rawLevel, err)
			}
			c.level = level
		}
	}

	timeout, err := durationFromEnv(EnvTimeout, DefaultTimeout)
	if err != nil {
		return nil, err
	}
	if timeout < 0 {
		return nil, fmt.Errorf("invalid %s %q: must be zero or positive", EnvTimeout, os.Getenv(EnvTimeout))
	}
	c.http = &http.Client{Timeout: timeout}

	retryMax, err := intFromEnv(EnvRetryMax, DefaultRetryMax)
	if err != nil {
		return nil, err
	}
	if retryMax < 0 {
		return nil, fmt.Errorf("invalid %s %q: must be zero or positive", EnvRetryMax, os.Getenv(EnvRetryMax))
	}
	c.retryMax = retryMax

	backoff, err := durationFromEnv(EnvRetryBackoff, DefaultRetryBackoff)
	if err != nil {
		return nil, err
	}
	if backoff < 0 {
		return nil, fmt.Errorf("invalid %s %q: must be zero or positive", EnvRetryBackoff, os.Getenv(EnvRetryBackoff))
	}
	c.backoff = backoff

	headers, err := parseHeaders(os.Getenv(EnvHeaders))
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", EnvHeaders, err)
	}
	c.headers = headers

	return c, nil
}

// IsEnabled reports whether the client has a webhook URL configured.
func (c *Client) IsEnabled() bool { return c.enabled }

// Notify delivers a message for the given severity, unless the client is
// disabled, the severity is below the configured threshold, or the context is
// already canceled. It never blocks the caller indefinitely: all HTTP work is
// bounded by the configured timeout and it never panics. Failures are logged
// and swallowed on purpose, so a broken webhook can never take down clocking.
func (c *Client) Notify(ctx context.Context, level slog.Level, message string) {
	if !c.enabled || level < c.level || msgEmpty(message) {
		return
	}
	c.send(ctx, message)
}

func msgEmpty(s string) bool { return strings.TrimSpace(s) == "" }

func (c *Client) send(ctx context.Context, message string) {
	body := map[string]string{
		c.titleKey: c.title,
		c.textKey:  message,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		slog.Error("❌ Webhook: failed to encode payload", "error", err)
		return
	}

	for attempt := 0; attempt <= c.retryMax; attempt++ {
		if ctx.Err() != nil {
			slog.Error("⛔ Webhook: canceled before sending, giving up", "error", ctx.Err())
			return
		}

		if attempt > 0 {
			if !c.wait(ctx, backoffFor(c.backoff, maxBackoff, attempt)) {
				return
			}
		}

		status, respBody, reqErr := c.doOnce(ctx, payload)
		if reqErr != nil {
			slog.Warn("⚠️ Webhook: request failed (transient)", "error", reqErr, "attempt", attempt+1)
			continue
		}

		if status >= http.StatusOK && status < http.StatusMultipleChoices {
			slog.Debug("✅ Webhook: notification delivered", "status", status, "attempt", attempt+1)
			return
		}

		if isPermanent(status) {
			slog.Error("❌ Webhook: permanent failure, not retrying",
				"status", status, "body", respBody)
			return
		}

		slog.Warn("⚠️ Webhook: transient status, will retry",
			"status", status, "body", respBody, "attempt", attempt+1)
	}

	slog.Error("❌ Webhook: all attempts failed", "attempts", c.retryMax+1)
}

func (c *Client) doOnce(ctx context.Context, payload []byte) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(payload))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Length", strconv.Itoa(len(payload)))
	// Custom headers are applied last so the caller can (for example) override
	// Content-Type or add Authorization.
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	return resp.StatusCode, string(data), nil
}

// isPermanent reports whether an HTTP status describes a failure that retrying
// cannot fix. Anything that is 5xx, or a 4xx which is not 408/429, maps to a
// permanent failure.
func isPermanent(status int) bool {
	if status >= 500 {
		return false // 5xx is (optimistically) transient
	}
	if status >= 400 && status < 500 {
		return status != http.StatusRequestTimeout && status != http.StatusTooManyRequests
	}
	return false
}

// wait blocks for d while honoring ctx. Returns false if ctx was canceled.
func (c *Client) wait(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	select {
	case <-ctx.Done():
		slog.Error("⛔ Webhook: canceled while waiting to retry", "error", ctx.Err())
		return false
	case <-time.After(d):
		return true
	}
}

// backoffFor returns backoff * 2^(attempt-1), capped at cap.
func backoffFor(backoff, cap time.Duration, attempt int) time.Duration {
	wait := backoff
	for i := 1; i < attempt; i++ {
		wait *= 2
		if wait >= cap {
			return cap
		}
	}
	if wait > cap {
		return cap
	}
	return wait
}

// parseLevel parses a log level name case-insensitively.
func parseLevel(value string) (slog.Level, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "DEBUG":
		return slog.LevelDebug, nil
	case "INFO":
		return slog.LevelInfo, nil
	case "WARN", "WARNING":
		return slog.LevelWarn, nil
	case "ERROR":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("expected DEBUG, INFO, WARN or ERROR")
	}
}

// durationFromEnv reads a duration from the named env var (or the default when
// unset). It accepts Go durations ("10s", "500ms") and bare integer seconds.
func durationFromEnv(name string, def time.Duration) (time.Duration, error) {
	raw, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(raw) == "" {
		return def, nil
	}
	raw = strings.TrimSpace(raw)
	if d, err := time.ParseDuration(raw); err == nil {
		return d, nil
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		return time.Duration(secs) * time.Second, nil
	}
	return 0, fmt.Errorf("invalid %s %q: expected a duration like \"10s\" or integer seconds", name, raw)
}

func intFromEnv(name string, def int) (int, error) {
	raw, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(raw) == "" {
		return def, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: expected an integer", name, raw)
	}
	return n, nil
}

// parseHeaders parses "Key: Value; Key2: Value2" into a map. Empty entries are
// ignored. Keys must be non-empty.
func parseHeaders(raw string) (map[string]string, error) {
	out := make(map[string]string)
	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx := strings.Index(part, ":")
		if idx <= 0 {
			return nil, fmt.Errorf("expected \"Key: Value\" but got %q", part)
		}
		key := strings.TrimSpace(part[:idx])
		val := strings.TrimSpace(part[idx+1:])
		if key == "" {
			return nil, fmt.Errorf("header key is empty in %q", part)
		}
		out[key] = val
	}
	return out, nil
}
