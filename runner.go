package main

import (
	"context"
	"log/slog"
	"time"
)

// Run is the main application loop that orchestrates the clocking actions throughout the day.
func Run(ctx context.Context, cfg *Config, clocker Clocker) {
	for {
		slog.Info("Starting the day")
		clockInTime, lunchTime, lunchFinishTime, clockOutTime, hasLunch := randomizeHours(cfg)

		slog.Debug("Waiting to clock in")
		if toClock, err := waitUntil(ctx, clockInTime); err != nil {
			break
		} else if toClock {
			slog.Info("Clock in time")
			err := clocker.ClockIn(ctx)
			if err != nil {
				slog.Warn("Error when clocking in", "error", err)
			}
		} else {
			slog.Info("Skipped clock in (missed event)")
		}

		if hasLunch {
			slog.Debug("Waiting to go to lunch")
			if toClock, err := waitUntil(ctx, lunchTime); err != nil {
				break
			} else if toClock {
				slog.Info("Lunch time")
				err := clocker.ClockPause(ctx)
				if err != nil {
					slog.Warn("Error when clocking pause for lunch", "error", err)
				}
			} else {
				slog.Info("Skipped lunch time (missed event)")
			}

			slog.Debug("Waiting to go back from lunch")
			if toClock, err := waitUntil(ctx, lunchFinishTime); err != nil {
				break
			} else if toClock {
				slog.Info("Back from lunch time")
				err := clocker.ClockResume(ctx)
				if err != nil {
					slog.Warn("Error when clocking resume", "error", err)
				}
			} else {
				slog.Info("Skipped back from lunch time (missed event)")
			}
		} else {
			slog.Info("Summer time. Skipping lunch break")
		}

		slog.Debug("Waiting to clock out")
		if toClock, err := waitUntil(ctx, clockOutTime); err != nil {
			break
		} else if toClock {
			slog.Info("Clock out time")
			err := clocker.ClockOut(ctx)
			if err != nil {
				slog.Warn("Error when clocking out", "error", err)
			}
		} else {
			slog.Info("Skipped clock out (missed event)")
		}

		if err := waitUntilTomorrow(ctx, cfg.ClockIn.Time); err != nil {
			break
		}
	}
}

// waitUntil waits until the targetHour. Returns true if the event should be executed,
// false if it was skipped due to being in the past, or an error if context was canceled.
func waitUntil(ctx context.Context, targetHour time.Time) (bool, error) {
	timeToClock := time.Until(targetHour)

	// If the event is more than 5 minutes in the past, consider it missed and skip it.
	if timeToClock < -5*time.Minute {
		slog.Debug("Time to clock was way before, skipping it", "timeToClock", timeToClock.Round(time.Minute))
		return false, nil
	} else if timeToClock <= 0 { // If it's slightly in the past (e.g., up to 5 mins), execute immediately
		slog.Debug("Time to clock was just a moment ago, time to clock", "timeToClock", timeToClock.Round(time.Minute))
		return true, nil
	}

	slog.Debug("Waiting until time", "duration", timeToClock.Round(time.Minute))
	timer := time.NewTimer(timeToClock)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

// waitUntilTomorrow calculates the time until the next day's clock-in time and waits for it.
func waitUntilTomorrow(ctx context.Context, clockIn time.Time) error {
	now := time.Now()
	tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, clockIn.Hour(), clockIn.Minute(), clockIn.Second(), clockIn.Nanosecond(), now.Location())
	wakeUpTime := time.Until(tomorrow) - time.Hour
	slog.Debug("Waiting until tomorrow", "duration", wakeUpTime.Round(time.Minute))

	if wakeUpTime <= 0 {
		return nil
	}

	timer := time.NewTimer(wakeUpTime)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
