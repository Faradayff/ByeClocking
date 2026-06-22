package main

import (
	"context"
	"log/slog"
)

type Clocker interface {
	ClockIn(ctx context.Context) error
	ClockPause(ctx context.Context) error
	ClockResume(ctx context.Context) error
	ClockOut(ctx context.Context) error
}

type DummyClocker struct{}

// ClockIn simulates clocking in at the start of the work period.
func (d *DummyClocker) ClockIn(ctx context.Context) error {
	slog.Debug("Action: Clock In")
	return nil
}

// ClockPause simulates pausing the clock, usually for a lunch break.
func (d *DummyClocker) ClockPause(ctx context.Context) error {
	slog.Debug("Action: Clock Pause (Lunch)")
	return nil
}

// ClockResume simulates resuming the clock, usually after returning from a lunch break.
func (d *DummyClocker) ClockResume(ctx context.Context) error {
	slog.Debug("Action: Clock Resume (Back from Lunch)")
	return nil
}

// ClockOut simulates clocking out at the end of the work period.
func (d *DummyClocker) ClockOut(ctx context.Context) error {
	slog.Debug("Action: Clock Out")
	return nil
}
