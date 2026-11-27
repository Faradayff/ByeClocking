package core

import (
	"context"
	"log/slog"
)

// Notifier is implemented by any component able to deliver notifications (for
// example the webhook.Client). Keeping the interface here means the core
// package does not depend on any concrete transport, and it can be satisfied
// by fakes in tests.
type Notifier interface {
	// Notify delivers a message for the given severity. Implementations must
	// be safe to call at any time and must never block the caller beyond their
	// own bounded work or panic.
	Notify(ctx context.Context, level slog.Level, message string)
}

// NoopNotifier is a Notifier that discards every notification.
type NoopNotifier struct{}

// Compile-time check that NoopNotifier satisfies Notifier.
var _ Notifier = NoopNotifier{}

// Notify discards the message.
func (NoopNotifier) Notify(context.Context, slog.Level, string) {}
