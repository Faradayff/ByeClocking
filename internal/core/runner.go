package core

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Faradayff/ByeClocking/internal/clients"
	"github.com/Faradayff/ByeClocking/internal/config"
)

// Run is the main application loop that orchestrates the clocking actions throughout the day.
func Run(ctx context.Context, cfg *config.Config, clocker clients.Clocker) {
	for {
		if now := nowFunc().Weekday(); now == time.Saturday || now == time.Sunday {
			slog.Info("🏖️ Today is weekend, skipping clocking")
			if err := waitUntilTomorrow(ctx, cfg.ClockIn.Time); err != nil {
				break
			}
			continue
		}

		// Check connectivity before querying holiday status.
		if ok := checkConnectivity(ctx, "holiday check"); !ok {
			if ctx.Err() != nil {
				break
			}
			continue
		}

		if clocker.IsHoliday(ctx) {
			slog.Info("🎉 Today is holiday, skipping clocking")
			if err := waitUntilTomorrow(ctx, cfg.ClockIn.Time); err != nil {
				break
			}
			continue
		}

		slog.Info("🌅 Starting the day")
		clockInTime, lunchTime, lunchFinishTime, clockOutTime, hasLunch := randomizeHours(cfg)

		slog.Debug("🔐 Waiting to clock in")
		if toClock, err := waitUntil(ctx, clockInTime); err != nil {
			break
		} else if toClock {
			slog.Info("✅ Clock in time")
			// Check connectivity before clocking in.
			if ok := checkConnectivity(ctx, "clock in"); !ok {
				if ctx.Err() != nil {
					break
				}
				if err := waitUntilTomorrow(ctx, cfg.ClockIn.Time); err != nil {
					break
				}
				continue
			}
			if err := clocker.ClockIn(ctx); err != nil {
				slog.Error("❌ Failed to clock in", "error", err)
				break
			}
		} else {
			slog.Info("⏭️ Skipped clock in (missed event)")
		}

		if hasLunch {
			slog.Debug("🍴 Waiting to go to lunch")
			if toClock, err := waitUntil(ctx, lunchTime); err != nil {
				break
			} else if toClock {
				slog.Info("⏸️ Lunch time")
				// Check connectivity before clocking pause.
				if ok := checkConnectivity(ctx, "lunch pause"); !ok {
					if ctx.Err() != nil {
						break
					}
					if err := waitUntilTomorrow(ctx, cfg.ClockIn.Time); err != nil {
						break
					}
					continue
				}
				if err := clocker.ClockPause(ctx); err != nil {
					slog.Error("❌ Error when clocking pause for lunch", "error", err)
					break
				}
			} else {
				slog.Info("⏭️ Skipped lunch time (missed event)")
			}

			slog.Debug("🍽️ Waiting to go back from lunch")
			if toClock, err := waitUntil(ctx, lunchFinishTime); err != nil {
				break
			} else if toClock {
				slog.Info("▶️ Back from lunch time")
				// Check connectivity before clocking resume.
				if ok := checkConnectivity(ctx, "lunch resume"); !ok {
					if ctx.Err() != nil {
						break
					}
					if err := waitUntilTomorrow(ctx, cfg.ClockIn.Time); err != nil {
						break
					}
					continue
				}
				if err := clocker.ClockResume(ctx); err != nil {
					slog.Error("❌ Error when clocking resume", "error", err)
					break
				}
			} else {
				slog.Info("⏭️ Skipped back from lunch time (missed event)")
			}
		} else {
			slog.Info("🌞 Summer time. Skipping lunch break")
		}

		slog.Debug("🔐 Waiting to clock out")
		if toClock, err := waitUntil(ctx, clockOutTime); err != nil {
			break
		} else if toClock {
			slog.Info("🏁 Clock out time")
			// Check connectivity before clocking out.
			if ok := checkConnectivity(ctx, "clock out"); !ok {
				if ctx.Err() != nil {
					break
				}
				if err := waitUntilTomorrow(ctx, cfg.ClockIn.Time); err != nil {
					break
				}
				continue
			}
			if err := clocker.ClockOut(ctx); err != nil {
				slog.Error("❌ Error when clocking out", "error", err)
				break
			}
		} else {
			slog.Info("⏭️ Skipped clock out (missed event)")
		}

		if err := waitUntilTomorrow(ctx, cfg.ClockIn.Time); err != nil {
			break
		}
	}
}

// checkConnectivity verifies internet connectivity before a network operation.
// It retries for up to ConnectivityMaxWait, logging warnings on each attempt.
// Returns true if connectivity is confirmed, false if the timeout was exceeded
// or the context was canceled (check ctx.Err() to distinguish the two).
func checkConnectivity(ctx context.Context, operation string) bool {
	slog.Debug("🌐 Checking internet connectivity", "operation", operation)
	err := WaitForConnectivity(ctx, ConnectivityMaxWait)
	if err == nil {
		return true
	}
	if errors.Is(err, ErrConnectivityTimeout) {
		slog.Error("❌ No internet connectivity after maximum wait time. Skipping to next day",
			"operation", operation,
			"maxWait", ConnectivityMaxWait,
		)
	}
	// ctx canceled or timeout — caller checks ctx.Err() to decide whether to break.
	return false
}

// waitUntil waits until the targetHour. Returns true if the event should be executed,
// false if it was skipped due to being in the past, or an error if context was canceled.
func waitUntil(ctx context.Context, targetHour time.Time) (bool, error) {
	timeToClock := time.Until(targetHour)

	// If the event is more than 30 minutes in the past, consider it missed and skip it.
	if timeToClock < -30*time.Minute {
		slog.Debug("⏭️ Time to clock was way before, skipping it", "timeToClock", timeToClock.Round(time.Minute))
		return false, nil
	} else if timeToClock <= 0 { // If it's slightly in the past (e.g., up to 30 mins), execute immediately
		slog.Debug("⏱️ Time to clock was just a moment ago, clocking", "timeToClock", timeToClock.Round(time.Minute))
		return true, nil
	}

	slog.Debug("🕒 Waiting until time", "duration", timeToClock.Round(time.Minute))
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
	now := nowFunc()
	tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, clockIn.Hour(), clockIn.Minute(), clockIn.Second(), clockIn.Nanosecond(), now.Location())
	wakeUpTime := time.Until(tomorrow) - 30*time.Minute
	slog.Debug("🌙 Waiting until tomorrow", "duration", wakeUpTime.Round(time.Minute))

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
