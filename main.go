package main

import (
	"context"
	"flag"
	"io"
	"log"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

const (
	logDir = "."
)

func main() {
	logLevel := flag.String("loglevel", "DEBUG", "Log level: DEBUG, INFO, WARN or ERROR")
	flag.Parse()

	initLogging(*logLevel)

	slog.Info("Starting application")

	cfg, err := LoadConfig("config.json")
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	for {
		slog.Info("Starting the day")
		clockInTime, lunchTime, lunchFinishTime, clockOutTime, hasLunch := randomizeHours(cfg)

		slog.Debug("Waiting to clock in")
		if toClock, err := waitUntil(ctx, clockInTime); err != nil {
			break
		} else if toClock {
			slog.Info("Clock in time")
			// clockIn
		} else {
			slog.Info("Skipped clock in (missed event)")
		}

		if hasLunch {
			slog.Debug("Waiting to go to lunch")
			if toClock, err := waitUntil(ctx, lunchTime); err != nil {
				break
			} else if toClock {
				slog.Info("Lunch time")
				// clockPause
			} else {
				slog.Info("Skipped lunch time (missed event)")
			}

			slog.Debug("Waiting to go back from lunch")
			if toClock, err := waitUntil(ctx, lunchFinishTime); err != nil {
				break
			} else if toClock {
				slog.Info("Back from lunch time")
				// clockResume
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
			// clockOut
		} else {
			slog.Info("Skipped clock out (missed event)")
		}

		if err := waitUntilTomorrow(ctx, cfg.ClockIn.Time); err != nil {
			break
		}
	}

	slog.Info("Application shut down gracefully")
}

func initLogging(loglevel string) {
	opts := &slog.HandlerOptions{}

	switch loglevel {
	case "DEBUG":
		opts.Level = slog.LevelDebug
	case "INFO":
		opts.Level = slog.LevelInfo
	case "WARN":
		opts.Level = slog.LevelWarn
	case "ERROR":
		opts.Level = slog.LevelError
	default:
		opts.Level = slog.LevelInfo
	}

	logFilePath := filepath.Join(logDir, "ByeClocking.log")

	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()

	var handler slog.Handler
	if loglevel == "DEBUG" {
		multiWriter := io.MultiWriter(os.Stdout, logFile)
		handler = slog.NewTextHandler(multiWriter, opts)
	} else {
		handler = slog.NewTextHandler(logFile, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
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

func randomizeHours(cfg *Config) (time.Time, time.Time, time.Time, time.Time, bool) {
	slog.Debug("Setting up delays and times")
	clockInDelay, lunchDelay, lunchDuration, clockOutDelay := initDelays(cfg)

	now := time.Now()
	setToday := func(t time.Time) time.Time {
		return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), now.Location())
	}

	isSummer := isSummerTime(cfg.SummerPeriod)

	var clockIn, clockOut time.Time
	if isSummer {
		clockIn = cfg.SummerTimes[0].Time
		clockOut = cfg.SummerTimes[1].Time
		slog.Info("Using summer times", "clockIn", clockIn, "clockOut", clockOut)
	} else {
		clockIn = cfg.ClockIn.Time
		clockOut = cfg.ClockOut.Time
	}

	clockInTime := setToday(clockIn).Add(clockInDelay)
	clockOutTime := setToday(clockOut).Add(clockOutDelay)

	var lunchTime, lunchFinishTime time.Time
	hasLunch := cfg.Lunchtime != nil && !isSummer

	if hasLunch {
		lunchTime = setToday(cfg.Lunchtime.Time).Add(lunchDelay)
		lunchFinishTime = setToday(cfg.Lunchtime.Time).Add(lunchDuration)
	}

	slog.Debug("Times initialized", "clockInTime", clockInTime, "lunchTime", lunchTime, "lunchFinishTime", lunchFinishTime, "clockOutTime", clockOutTime, "hasLunch", hasLunch)
	return clockInTime, lunchTime, lunchFinishTime, clockOutTime, hasLunch
}

func initDelays(cfg *Config) (clockInDelay time.Duration, lunchDelay time.Duration, lunchDuration time.Duration, clockOutDelay time.Duration) {
	if cfg.Unpunctuality > 0 {
		clockInDelay = time.Duration(rand.Intn(cfg.Unpunctuality)) * time.Minute
		clockOutDelay = time.Duration(rand.Intn(cfg.LeaveUnpunctuality))*time.Minute + clockInDelay
	}
	if cfg.LunchUnpunctuality > 0 {
		lunchDelay = time.Duration(rand.Intn(cfg.LunchUnpunctuality)) * time.Minute
		lunchDuration = time.Duration(cfg.MinTimeToLunch+rand.Intn(cfg.MaxTimeToLunch-cfg.MinTimeToLunch+1)) * time.Minute
	}

	slog.Debug("Delays initialized", "ClockInDelay", clockInDelay, "lunchDelay", lunchDelay, "lunchDuration", lunchDuration, "clockOutDelay", clockOutDelay)
	return
}
