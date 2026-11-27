package core

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Faradayff/ByeClocking/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file validates the wiring between the runner and a Notifier using the
// real clocking sequence. It runs as part of the default test suite (no build
// tag), does not require network access (the connectivity check is stubbed via
// the waitForConnectivity seam), and does not depend on a specific weekday
// except where noted (it skips on weekend / non-Mon-Thu as appropriate).

type recEvent struct {
	level slog.Level
	msg   string
}

type recNotifier struct {
	mu     sync.Mutex
	events []recEvent
}

func (r *recNotifier) Notify(_ context.Context, level slog.Level, msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, recEvent{level: level, msg: msg})
}

func (r *recNotifier) get() []recEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recEvent(nil), r.events...)
}

type stubClocker struct {
	mu       sync.Mutex
	calls    []string
	clkInErr error
}

func (s *stubClocker) record(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, name)
	if name == "ClockIn" {
		return s.clkInErr
	}
	return nil
}

func (s *stubClocker) ClockIn(context.Context) error     { return s.record("ClockIn") }
func (s *stubClocker) ClockPause(context.Context) error  { return s.record("ClockPause") }
func (s *stubClocker) ClockResume(context.Context) error { return s.record("ClockResume") }
func (s *stubClocker) ClockOut(context.Context) error    { return s.record("ClockOut") }
func (s *stubClocker) IsHoliday(context.Context) bool    { return false }
func (s *stubClocker) callCount() int                    { s.mu.Lock(); defer s.mu.Unlock(); return len(s.calls) }
func (s *stubClocker) getCalls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

// stubConnectivity forces the runner to see itself as "connected" so the tests
// do not depend on the real network or wait on WaitForConnectivity.
func stubConnectivity(t *testing.T) {
	t.Helper()
	original := waitForConnectivity
	waitForConnectivity = func(context.Context, time.Duration) error { return nil }
	t.Cleanup(func() { waitForConnectivity = original })
}

// notifCfg builds a config where every action time is a few seconds in the past,
// so waitUntil() executes them immediately, with zero randomisation.
func notifCfg() *config.Config {
	past := func(d time.Duration) config.ClockTime {
		n := time.Now().Add(-d)
		return config.ClockTime{Time: time.Date(0, 1, 1, n.Hour(), n.Minute(), n.Second(), 0, time.UTC)}
	}
	lunch := past(4 * time.Second)
	now := time.Now()
	return &config.Config{
		ClockIn:            past(6 * time.Second),
		ClockOut:           past(2 * time.Second),
		Lunchtime:          &lunch,
		MinTimeToLunch:     0,
		MaxTimeToLunch:     0,
		Unpunctuality:      0,
		LeaveUnpunctuality: 0,
		LunchUnpunctuality: 0,
		// Summer period set to the future so it is not "summer".
		SummerPeriod: []string{now.AddDate(0, 0, 1).Format("02/01"), now.AddDate(0, 0, 2).Format("02/01")},
	}
}

func isWeekend() bool {
	w := time.Now().Weekday()
	return w == time.Saturday || w == time.Sunday
}

// TestRun_NotifiesWeekdaySuccess checks that a clean weekday run (Mon–Fri with a
// lunch configured) emits one INFO success notification per clock action: clock
// in, lunch pause, lunch resume, and clock out. Weekends skip clocking entirely.
//
// Note: "Friday" special timing only kicks in when FridayTimes is configured;
// this test's config does not set it, so Friday behaves like any other weekday.
func TestRun_NotifiesWeekdaySuccess(t *testing.T) {
	if isWeekend() {
		t.Skip("weekend: the runner skips clocking entirely")
	}
	stubConnectivity(t)

	rec := &recNotifier{}
	clock := &stubClocker{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(ctx, notifCfg(), clock, rec)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for clock.callCount() < 4 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done

	assert.Equal(t, []string{"ClockIn", "ClockPause", "ClockResume", "ClockOut"},
		clock.getCalls(), "expected the full 4-action weekday sequence")

	info := eventsLevel(rec.get(), slog.LevelInfo)
	require.Len(t, info, 4, "expected 4 INFO notifications, got %v", info)
	combined := strings.Join(mustMessages(info), "\n")
	// These must be the SAME strings the normal log writes (unified one-line-per-event).
	for _, want := range []string{"✅ Clock in time", "⏸️ Lunch time", "▶️ Back from lunch time", "🏁 Clock out time"} {
		assert.Contains(t, combined, want, "INFO notifications should match the log line %q", want)
	}
	assert.Empty(t, eventsLevel(rec.get(), slog.LevelError), "no ERROR notifications on a clean run")
}

func mustMessages(events []recEvent) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.msg)
	}
	return out
}

// TestRun_NotifiesClockInError checks that a failing ClockIn produces exactly one
// ERROR notification (Mon–Fri) and that the runner stops.
func TestRun_NotifiesClockInError(t *testing.T) {
	if isWeekend() {
		t.Skip("weekend: the runner skips clocking entirely")
	}
	stubConnectivity(t)

	rec := &recNotifier{}
	clock := &stubClocker{clkInErr: errors.New("boom")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(ctx, notifCfg(), clock, rec)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runner did not exit after a ClockIn error")
	}

	assert.Equal(t, []string{"ClockIn"}, clock.getCalls(), "runner should stop after ClockIn error")
	errs := eventsLevel(rec.get(), slog.LevelError)
	require.Len(t, errs, 1, "expected exactly 1 ERROR notification, got %v", errs)
	assert.Contains(t, errs[0].msg, "Failed to clock in")
}

func eventsLevel(events []recEvent, level slog.Level) []recEvent {
	var out []recEvent
	for _, e := range events {
		if e.level == level {
			out = append(out, e)
		}
	}
	return out
}
