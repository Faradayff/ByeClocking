package main

import (
	"log/slog"
	"time"
)

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
