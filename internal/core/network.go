package core

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"time"
)

const (
	// connectivityCheckHost is the hostname used to verify internet connectivity via DNS.
	connectivityCheckHost = "dns.google"

	// connectivityRetryInterval is the wait time between consecutive connectivity checks.
	connectivityRetryInterval = 30 * time.Second

	// connectivityProbeTimeout is the timeout for each individual DNS probe attempt.
	connectivityProbeTimeout = 5 * time.Second

	// ConnectivityMaxWait is the maximum time WaitForConnectivity will retry before giving up.
	ConnectivityMaxWait = 30 * time.Minute
)

// ErrConnectivityTimeout is returned by WaitForConnectivity when connectivity could not
// be established within the allowed maximum wait time.
var ErrConnectivityTimeout = errors.New("internet connectivity could not be established within the timeout")

// WaitForConnectivity blocks until internet connectivity is confirmed by successfully
// resolving a well-known hostname, or until maxWait is exceeded.
// It logs a warning on each failed retry.
// Returns ErrConnectivityTimeout if the deadline is reached, or ctx.Err() if canceled.
func WaitForConnectivity(ctx context.Context, maxWait time.Duration) error {
	if isConnected() {
		return nil
	}

	deadline := time.Now().Add(maxWait)
	slog.Warn("🌐 No internet connectivity detected. Will retry until connection is available or timeout is reached",
		"host", connectivityCheckHost,
		"retryInterval", connectivityRetryInterval,
		"maxWait", maxWait,
	)

	ticker := time.NewTicker(connectivityRetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if isConnected() {
				slog.Info("✅ Internet connectivity restored")
				return nil
			}
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return ErrConnectivityTimeout
			}
			slog.Debug("🔄 Still no internet connectivity, retrying...",
				"retryInterval", connectivityRetryInterval,
				"timeRemaining", remaining.Round(time.Second),
			)
		}
	}
}

// isConnected checks whether internet connectivity is available by attempting
// a DNS lookup of a well-known hostname.
func isConnected() bool {
	resolver := &net.Resolver{}
	ctx, cancel := context.WithTimeout(context.Background(), connectivityProbeTimeout)
	defer cancel()

	addresses, err := resolver.LookupHost(ctx, connectivityCheckHost)
	return err == nil && len(addresses) > 0
}
