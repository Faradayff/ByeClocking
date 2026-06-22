package main

import (
	"log/slog"
	"math/rand"
	"time"
)

// randomizeHours generates the specific execution times for today's clock actions, applying unpunctuality delays.
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

// initDelays calculates the random delay durations for each clocking action based on the configuration limits.
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

// isSummerTime() checks if the current date is within the summer period.
// period expects two strings in "DD/MM" format, e.g. ["01/06", "31/08"].
func isSummerTime(period []string) bool {
	now := time.Now()

	if len(period) != 2 {
		return false
	}

	// Format is "DD/MM" - mapped to "02/01" in Go's reference time format
	start, err := time.Parse("02/01", period[0])
	if err != nil {
		slog.Warn("Invalid summer period format, expected DD/MM", "period", period[0])
		return false
	}

	end, err := time.Parse("02/01", period[1])
	if err != nil {
		slog.Warn("Invalid summer period format, expected DD/MM", "period", period[1])
		return false
	}

	// We only compare Month and Day. To do so, we normalize all dates to the same year.
	nowDay := time.Date(0, now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	startDay := time.Date(0, start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	endDay := time.Date(0, end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)

	return (nowDay.After(startDay) || nowDay.Equal(startDay)) && (nowDay.Before(endDay) || nowDay.Equal(endDay))
}
