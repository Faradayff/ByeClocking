package clients

import (
	"context"
	"log/slog"
)

type Clocker interface {
	ClockIn(ctx context.Context) error
	ClockPause(ctx context.Context) error
	ClockResume(ctx context.Context) error
	ClockOut(ctx context.Context) error
	IsHoliday(ctx context.Context) bool
}

type DummyClocker struct{}

// ClockIn simulates clocking in at the start of the work period.
func (d *DummyClocker) ClockIn(context.Context) error {
	slog.Debug("✅ Action: Clock In")
	return nil
}

// ClockPause simulates pausing the clock, usually for a lunch break.
func (d *DummyClocker) ClockPause(context.Context) error {
	slog.Debug("⏸️ Action: Clock Pause (Lunch)")
	return nil
}

// ClockResume simulates resuming the clock, usually after returning from a lunch break.
func (d *DummyClocker) ClockResume(context.Context) error {
	slog.Debug("▶️ Action: Clock Resume (Back from Lunch)")
	return nil
}

// ClockOut simulates clocking out at the end of the work period.
func (d *DummyClocker) ClockOut(context.Context) error {
	slog.Debug("🏁 Action: Clock Out")
	return nil
}

// IsHoliday checks if the current day is a holiday.
func (d *DummyClocker) IsHoliday(context.Context) bool {
	return false
}
