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

func (d *DummyClocker) ClockIn(ctx context.Context) error {
	slog.Debug("Action: Clock In")
	return nil
}

func (d *DummyClocker) ClockPause(ctx context.Context) error {
	slog.Debug("Action: Clock Pause (Lunch)")
	return nil
}

func (d *DummyClocker) ClockResume(ctx context.Context) error {
	slog.Debug("Action: Clock Resume (Back from Lunch)")
	return nil
}

func (d *DummyClocker) ClockOut(ctx context.Context) error {
	slog.Debug("Action: Clock Out")
	return nil
}
