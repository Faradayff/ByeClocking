package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Faradayff/ByeClocking/internal/clients"
	"github.com/Faradayff/ByeClocking/internal/config"
)

// Announce records ONE line in the normal log AND delivers that same line —
// same level, same message — to the webhook notifier. Keeping the log and the
// notification in a single call guarantees they are always identical.
func Announce(ctx context.Context, notifier Notifier, level slog.Level, msg string) {
	slog.Log(ctx, level, msg)
	if notifier != nil {
		notifier.Notify(ctx, level, msg)
	}
}

// Run is the main application loop that orchestrates the clocking actions throughout the day.
func Run(ctx context.Context, cfg *config.Config, clocker clients.Clocker, notifier Notifier) {
	if notifier == nil {
		notifier = NoopNotifier{}
	}

	for {
		if now := nowFunc().Weekday(); now == time.Saturday || now == time.Sunday {
			Announce(ctx, notifier, slog.LevelInfo, "🏖️ Today is weekend, skipping clocking")
			if err := waitUntilTomorrow(ctx, cfg.ClockIn.Time); err != nil {
				break
			}
			continue
		}

		// Check connectivity before querying holiday status.
		if ok := checkConnectivity(ctx, "holiday check", notifier); !ok {
			if ctx.Err() != nil {
				break
			}
			continue
		}

		if clocker.IsHoliday(ctx) {
			Announce(ctx, notifier, slog.LevelInfo, "🎉 Today is holiday, skipping clocking")
			if err := waitUntilTomorrow(ctx, cfg.ClockIn.Time); err != nil {
				break
			}
			continue
		}

		slog.Info("🌅 Starting the day")
		clockInTime, lunchTime, lunchFinishTime, clockOutTime, hasLunch := randomizeHours(cfg)

		// --- Clock in ---
		slog.Debug("🔐 Waiting to clock in")
		if toClock, err := waitUntil(ctx, clockInTime); err != nil {
			break
		} else if !toClock {
			Announce(ctx, notifier, slog.LevelWarn, "⏭️ Skipped clock in (missed event)")
		} else {
			if ok := checkConnectivity(ctx, "clock in", notifier); !ok {
				if ctx.Err() != nil {
					break
				}
				if err := waitUntilTomorrow(ctx, cfg.ClockIn.Time); err != nil {
					break
				}
				continue
			}
			if err := clocker.ClockIn(ctx); err != nil {
				Announce(ctx, notifier, slog.LevelError, "❌ Failed to clock in: "+err.Error())
				break
			}
			Announce(ctx, notifier, slog.LevelInfo, "✅ Clock in time")
		}

		// --- Lunch break ---
		if hasLunch {
			slog.Debug("🍴 Waiting to go to lunch")
			if toClock, err := waitUntil(ctx, lunchTime); err != nil {
				break
			} else if !toClock {
				Announce(ctx, notifier, slog.LevelWarn, "⏭️ Skipped lunch time (missed event)")
			} else {
				if ok := checkConnectivity(ctx, "lunch pause", notifier); !ok {
					if ctx.Err() != nil {
						break
					}
					if err := waitUntilTomorrow(ctx, cfg.ClockIn.Time); err != nil {
						break
					}
					continue
				}
				if err := clocker.ClockPause(ctx); err != nil {
					Announce(ctx, notifier, slog.LevelError, "❌ Error when clocking pause for lunch: "+err.Error())
					break
				}
				Announce(ctx, notifier, slog.LevelInfo, "⏸️ Lunch time")
			}

			slog.Debug("🍽️ Waiting to go back from lunch")
			if toClock, err := waitUntil(ctx, lunchFinishTime); err != nil {
				break
			} else if !toClock {
				Announce(ctx, notifier, slog.LevelWarn, "⏭️ Skipped back from lunch time (missed event)")
			} else {
				if ok := checkConnectivity(ctx, "lunch resume", notifier); !ok {
					if ctx.Err() != nil {
						break
					}
					if err := waitUntilTomorrow(ctx, cfg.ClockIn.Time); err != nil {
						break
					}
					continue
				}
				if err := clocker.ClockResume(ctx); err != nil {
					Announce(ctx, notifier, slog.LevelError, "❌ Error when clocking resume: "+err.Error())
					break
				}
				Announce(ctx, notifier, slog.LevelInfo, "▶️ Back from lunch time")
			}
		} else {
			slog.Info("🎉 No lunch today. Skipping lunch break")
		}

		// --- Clock out ---
		slog.Debug("🔐 Waiting to clock out")
		if toClock, err := waitUntil(ctx, clockOutTime); err != nil {
			break
		} else if !toClock {
			Announce(ctx, notifier, slog.LevelWarn, "⏭️ Skipped clock out (missed event)")
		} else {
			if ok := checkConnectivity(ctx, "clock out", notifier); !ok {
				if ctx.Err() != nil {
					break
				}
				if err := waitUntilTomorrow(ctx, cfg.ClockIn.Time); err != nil {
					break
				}
				continue
			}
			if err := clocker.ClockOut(ctx); err != nil {
				Announce(ctx, notifier, slog.LevelError, "❌ Error when clocking out: "+err.Error())
				break
			}
			Announce(ctx, notifier, slog.LevelInfo, "🏁 Clock out time")
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
func checkConnectivity(ctx context.Context, operation string, notifier Notifier) bool {
	slog.Debug("🌐 Checking internet connectivity", "operation", operation)
	err := waitForConnectivity(ctx, ConnectivityMaxWait)
	if err == nil {
		return true
	}
	if errors.Is(err, ErrConnectivityTimeout) {
		Announce(ctx, notifier, slog.LevelError,
			fmt.Sprintf("❌ No internet connectivity before %q. Skipping to next day.", operation))
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
	wakeUpTime := time.Until(tomorrow) - time.Hour
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
