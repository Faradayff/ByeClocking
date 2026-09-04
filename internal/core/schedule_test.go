package core

import (
	"testing"
	"time"

	"github.com/Faradayff/ByeClocking/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestInitDelays(t *testing.T) {
	cfg := &config.Config{
		Unpunctuality:      10,
		LeaveUnpunctuality: 15,
		LunchUnpunctuality: 5,
		MinTimeToLunch:     30,
		MaxTimeToLunch:     60,
	}

	for i := 0; i < 100; i++ { // Run multiple times to cover randomness
		clockInDelay, lunchDelay, lunchDuration, clockOutDelay := initDelays(cfg)

		assert.GreaterOrEqual(t, clockInDelay, time.Duration(0))
		assert.Less(t, clockInDelay, 10*time.Minute)

		assert.GreaterOrEqual(t, clockOutDelay, clockInDelay)
		assert.Less(t, clockOutDelay, clockInDelay+15*time.Minute)

		assert.GreaterOrEqual(t, lunchDelay, time.Duration(0))
		assert.Less(t, lunchDelay, 5*time.Minute)

		assert.GreaterOrEqual(t, lunchDuration, lunchDelay+30*time.Minute)
		assert.LessOrEqual(t, lunchDuration, lunchDelay+60*time.Minute)
	}
}

func TestInitDelaysZero(t *testing.T) {
	cfg := &config.Config{
		Unpunctuality:      0,
		LeaveUnpunctuality: 0,
		LunchUnpunctuality: 0,
		MinTimeToLunch:     30,
		MaxTimeToLunch:     30,
	}

	clockInDelay, lunchDelay, lunchDuration, clockOutDelay := initDelays(cfg)

	assert.Equal(t, time.Duration(0), clockInDelay)
	assert.Equal(t, time.Duration(0), clockOutDelay)
	assert.Equal(t, time.Duration(0), lunchDelay)
	assert.Equal(t, 30*time.Minute, lunchDuration)
}

func TestIsSummerTime(t *testing.T) {
	// Mock time
	originalNow := nowFunc
	nowFunc = func() time.Time {
		return time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	}
	defer func() { nowFunc = originalNow }()

	now := nowFunc()

	// Format is DD/MM or D/M
	todayStr := now.Format("02/01")

	yesterday := now.AddDate(0, 0, -1)
	yesterdayStr := yesterday.Format("02/01")

	tomorrow := now.AddDate(0, 0, 1)
	tomorrowStr := tomorrow.Format("02/01")

	tests := []struct {
		name     string
		period   []string
		expected bool
	}{
		{
			name:     "Today is inside period (start today)",
			period:   []string{todayStr, tomorrowStr},
			expected: true,
		},
		{
			name:     "Today is inside period (end today)",
			period:   []string{yesterdayStr, todayStr},
			expected: true,
		},
		{
			name:     "Today is inside period (middle)",
			period:   []string{yesterdayStr, tomorrowStr},
			expected: true,
		},
		{
			name:     "Today is outside period (before)",
			period:   []string{tomorrowStr, now.AddDate(0, 0, 2).Format("02/01")},
			expected: false,
		},
		{
			name:     "Today is outside period (after)",
			period:   []string{now.AddDate(0, 0, -2).Format("02/01"), yesterdayStr},
			expected: false,
		},
		{
			name:     "Invalid length",
			period:   []string{todayStr},
			expected: false,
		},
		{
			name:     "Invalid format start",
			period:   []string{"invalid", tomorrowStr},
			expected: false,
		},
		{
			name:     "Invalid format end",
			period:   []string{yesterdayStr, "invalid"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSummerTime(tt.period)
			assert.Equal(t, tt.expected, result, "isSummerTime(%v)", tt.period)
		})
	}
}

func TestRandomizeHours(t *testing.T) {
	now := time.Now()

	// Create base times
	clockInTime := time.Date(0, 1, 1, 9, 0, 0, 0, time.UTC)
	clockOutTime := time.Date(0, 1, 1, 18, 0, 0, 0, time.UTC)
	lunchTime := time.Date(0, 1, 1, 14, 0, 0, 0, time.UTC)

	cfg := &config.Config{
		ClockIn:            config.ClockTime{Time: clockInTime},
		ClockOut:           config.ClockTime{Time: clockOutTime},
		Lunchtime:          &config.ClockTime{Time: lunchTime},
		MinTimeToLunch:     30,
		MaxTimeToLunch:     60,
		Unpunctuality:      0,
		LeaveUnpunctuality: 0,
		LunchUnpunctuality: 0,
		// Tomorrow to day after (not summer)
		SummerPeriod: []string{now.AddDate(0, 0, 1).Format("02/01"), now.AddDate(0, 0, 2).Format("02/01")},
	}

	cIn, lTime, lFinish, cOut, hasLunch := randomizeHours(cfg)

	assert.True(t, hasLunch, "Expected hasLunch to be true")

	assert.Equal(t, 9, cIn.Hour())
	assert.Equal(t, 0, cIn.Minute())
	assert.Equal(t, now.Day(), cIn.Day())

	assert.Equal(t, 18, cOut.Hour())
	assert.Equal(t, 0, cOut.Minute())
	assert.Equal(t, now.Day(), cOut.Day())

	assert.Equal(t, 14, lTime.Hour())
	assert.Equal(t, 0, lTime.Minute())
	assert.Equal(t, now.Day(), lTime.Day())

	diff := lFinish.Sub(lTime)
	assert.GreaterOrEqual(t, diff, 30*time.Minute)
	assert.LessOrEqual(t, diff, 60*time.Minute)
}

func TestRandomizeHoursSummer(t *testing.T) {
	now := time.Now()

	// Create base times
	clockInTime := time.Date(0, 1, 1, 9, 0, 0, 0, time.UTC)
	clockOutTime := time.Date(0, 1, 1, 18, 0, 0, 0, time.UTC)
	lunchTime := time.Date(0, 1, 1, 14, 0, 0, 0, time.UTC)

	summerInTime := time.Date(0, 1, 1, 8, 0, 0, 0, time.UTC)
	summerOutTime := time.Date(0, 1, 1, 15, 0, 0, 0, time.UTC)

	cfg := &config.Config{
		ClockIn:            config.ClockTime{Time: clockInTime},
		ClockOut:           config.ClockTime{Time: clockOutTime},
		Lunchtime:          &config.ClockTime{Time: lunchTime},
		MinTimeToLunch:     30,
		MaxTimeToLunch:     60,
		Unpunctuality:      0,
		LeaveUnpunctuality: 0,
		LunchUnpunctuality: 0,
		// Yesterday to tomorrow (summer)
		SummerPeriod: []string{now.AddDate(0, 0, -1).Format("02/01"), now.AddDate(0, 0, 1).Format("02/01")},
		SummerTimes:  []config.ClockTime{{Time: summerInTime}, {Time: summerOutTime}},
	}

	cIn, _, _, cOut, hasLunch := randomizeHours(cfg)

	assert.False(t, hasLunch, "Expected hasLunch to be false during summer")

	assert.Equal(t, 8, cIn.Hour())
	assert.Equal(t, 0, cIn.Minute())
	assert.Equal(t, now.Day(), cIn.Day())

	assert.Equal(t, 15, cOut.Hour())
	assert.Equal(t, 0, cOut.Minute())
	assert.Equal(t, now.Day(), cOut.Day())
}

func TestRandomizeHoursFriday(t *testing.T) {
	// Mock time to be a Friday
	originalNow := nowFunc
	nowFunc = func() time.Time {
		// 2026-09-04 is a Friday
		return time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	}
	defer func() { nowFunc = originalNow }()

	now := nowFunc()

	// Create base times
	clockInTime := time.Date(0, 1, 1, 9, 0, 0, 0, time.UTC)
	clockOutTime := time.Date(0, 1, 1, 18, 0, 0, 0, time.UTC)
	lunchTime := time.Date(0, 1, 1, 14, 0, 0, 0, time.UTC)

	fridayInTime := time.Date(0, 1, 1, 8, 30, 0, 0, time.UTC)
	fridayOutTime := time.Date(0, 1, 1, 15, 30, 0, 0, time.UTC)

	cfg := &config.Config{
		ClockIn:            config.ClockTime{Time: clockInTime},
		ClockOut:           config.ClockTime{Time: clockOutTime},
		FridayTimes:        []config.ClockTime{{Time: fridayInTime}, {Time: fridayOutTime}},
		Lunchtime:          &config.ClockTime{Time: lunchTime},
		MinTimeToLunch:     30,
		MaxTimeToLunch:     60,
		Unpunctuality:      0,
		LeaveUnpunctuality: 0,
		LunchUnpunctuality: 0,
		// Tomorrow to day after (not summer)
		SummerPeriod: []string{now.AddDate(0, 0, 1).Format("02/01"), now.AddDate(0, 0, 2).Format("02/01")},
	}

	cIn, _, _, cOut, hasLunch := randomizeHours(cfg)

	assert.False(t, hasLunch, "Expected hasLunch to be false on Friday")

	assert.Equal(t, 8, cIn.Hour())
	assert.Equal(t, 30, cIn.Minute())
	assert.Equal(t, now.Day(), cIn.Day())

	assert.Equal(t, 15, cOut.Hour())
	assert.Equal(t, 30, cOut.Minute())
	assert.Equal(t, now.Day(), cOut.Day())
}

func TestRandomizeHoursWeekend(t *testing.T) {
	// Mock time to be a Saturday
	originalNow := nowFunc
	nowFunc = func() time.Time {
		// 2026-09-05 is a Saturday
		return time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	}
	defer func() { nowFunc = originalNow }()

	now := nowFunc()

	// Create base times
	clockInTime := time.Date(0, 1, 1, 9, 0, 0, 0, time.UTC)
	clockOutTime := time.Date(0, 1, 1, 18, 0, 0, 0, time.UTC)
	lunchTime := time.Date(0, 1, 1, 14, 0, 0, 0, time.UTC)

	fridayInTime := time.Date(0, 1, 1, 8, 30, 0, 0, time.UTC)
	fridayOutTime := time.Date(0, 1, 1, 15, 30, 0, 0, time.UTC)

	cfg := &config.Config{
		ClockIn:            config.ClockTime{Time: clockInTime},
		ClockOut:           config.ClockTime{Time: clockOutTime},
		FridayTimes:        []config.ClockTime{{Time: fridayInTime}, {Time: fridayOutTime}},
		Lunchtime:          &config.ClockTime{Time: lunchTime},
		MinTimeToLunch:     30,
		MaxTimeToLunch:     60,
		Unpunctuality:      0,
		LeaveUnpunctuality: 0,
		LunchUnpunctuality: 0,
		// Tomorrow to day after (not summer)
		SummerPeriod: []string{now.AddDate(0, 0, 1).Format("02/01"), now.AddDate(0, 0, 2).Format("02/01")},
	}

	cIn, _, _, cOut, hasLunch := randomizeHours(cfg)

	// Even with FridayTimes configured, since it's Saturday, it should use normal times
	// The runner handles skipping weekends, randomizeHours just returns the default
	assert.True(t, hasLunch, "Expected hasLunch to be true on weekend fallback")

	assert.Equal(t, 9, cIn.Hour())
	assert.Equal(t, 0, cIn.Minute())
	assert.Equal(t, now.Day(), cIn.Day())

	assert.Equal(t, 18, cOut.Hour())
	assert.Equal(t, 0, cOut.Minute())
	assert.Equal(t, now.Day(), cOut.Day())
}
