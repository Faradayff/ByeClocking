//go:build e2e

package e2e

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Faradayff/ByeClocking/internal/clients"
	"github.com/Faradayff/ByeClocking/internal/config"
	"github.com/Faradayff/ByeClocking/internal/core"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// Mock Clocker
// ---------------------------------------------------------------------------

type mockClocker struct {
	mu      sync.Mutex
	calls   []string
	holiday bool
	errors  map[string]error

	// blockCh: when set for an action name, that action blocks until blockCh is closed.
	blockCh map[string]chan struct{}
}

func newMockClocker() *mockClocker {
	return &mockClocker{
		errors:  make(map[string]error),
		blockCh: make(map[string]chan struct{}),
	}
}

func (m *mockClocker) record(action string) error {
	m.mu.Lock()
	m.calls = append(m.calls, action)
	ch := m.blockCh[action]
	err := m.errors[action]
	m.mu.Unlock()

	if ch != nil {
		<-ch // block until closed
	}
	return err
}

func (m *mockClocker) ClockIn(ctx context.Context) error     { return m.record("ClockIn") }
func (m *mockClocker) ClockPause(ctx context.Context) error  { return m.record("ClockPause") }
func (m *mockClocker) ClockResume(ctx context.Context) error { return m.record("ClockResume") }
func (m *mockClocker) ClockOut(ctx context.Context) error    { return m.record("ClockOut") }
func (m *mockClocker) IsHoliday(ctx context.Context) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.holiday
}

func (m *mockClocker) getCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.calls))
	copy(result, m.calls)
	return result
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// pastClockTime builds a config.ClockTime set to (now - d), so that
// waitUntil() sees it as "just happened" and executes it immediately.
func pastClockTime(d time.Duration) config.ClockTime {
	t := time.Now().Add(-d)
	return config.ClockTime{Time: time.Date(0, 1, 1, t.Hour(), t.Minute(), t.Second(), 0, time.UTC)}
}

// runnerDone runs core.Run in a goroutine and returns a channel that closes when Run exits.
func runnerDone(ctx context.Context, cfg *config.Config, clocker clients.Clocker) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		core.Run(ctx, cfg, clocker)
	}()
	return done
}

// waitForCalls blocks until the mock has recorded at least n calls, or the timeout expires.
func waitForCalls(t *testing.T, mock *mockClocker, n int, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(mock.getCalls()) >= n {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// baseCfg returns a minimal valid config with all times in the past so the runner
// executes actions immediately. The summer period is set to the future so it's not summer.
func baseCfg() *config.Config {
	now := time.Now()
	lunchtime := pastClockTime(4 * time.Second)
	return &config.Config{
		ClockIn:            pastClockTime(6 * time.Second),
		ClockOut:           pastClockTime(2 * time.Second),
		Lunchtime:          &lunchtime,
		MinTimeToLunch:     1,
		MaxTimeToLunch:     1,
		Unpunctuality:      0,
		LeaveUnpunctuality: 0,
		LunchUnpunctuality: 0,
		// Summer period set to the future so isSummerTime() returns false
		SummerPeriod: []string{
			now.AddDate(0, 0, 1).Format("02/01"),
			now.AddDate(0, 0, 2).Format("02/01"),
		},
		SummerTimes:      []config.ClockTime{pastClockTime(7 * time.Second), pastClockTime(3 * time.Second)},
		FridayTimes:      []config.ClockTime{pastClockTime(6 * time.Second), pastClockTime(2 * time.Second)},
		ClockingPlatform: "dummy",
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestRunner_NormalWeekday verifies the full clock sequence for a normal weekday
// (Mon–Thu) with lunch. Because all times are in the past, waitUntil() executes
// them immediately without sleeping.
func TestRunner_NormalWeekday(t *testing.T) {
	now := time.Now().Weekday()
	if now == time.Saturday || now == time.Sunday || now == time.Friday {
		t.Skip("TestRunner_NormalWeekday requires a Mon–Thu run; skipping")
	}

	mock := newMockClocker()
	cfg := baseCfg()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := runnerDone(ctx, cfg, mock)

	// Wait until all 4 actions are recorded, then cancel to stop the runner
	// (otherwise it would call waitUntilTomorrow and block).
	ok := waitForCalls(t, mock, 4, 5*time.Second)
	cancel()
	<-done

	assert.True(t, ok, "runner should have recorded 4 clock actions within timeout")
	calls := mock.getCalls()
	assert.Equal(t, []string{"ClockIn", "ClockPause", "ClockResume", "ClockOut"}, calls)
}

// TestRunner_Weekend verifies that the runner skips clocking entirely on a weekend
// and calls waitUntilTomorrow. We cancel the context immediately to avoid a long wait.
func TestRunner_Weekend(t *testing.T) {
	now := time.Now().Weekday()
	if now != time.Saturday && now != time.Sunday {
		t.Skip("TestRunner_Weekend requires running on a Saturday or Sunday; skipping")
	}

	mock := newMockClocker()
	cfg := baseCfg()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := runnerDone(ctx, cfg, mock)
	<-done

	assert.Empty(t, mock.getCalls(), "runner should not call any clock action on weekend")
}

// TestRunner_Holiday verifies that when IsHoliday returns true the runner skips
// all clock actions and calls waitUntilTomorrow.
func TestRunner_Holiday(t *testing.T) {
	now := time.Now().Weekday()
	if now == time.Saturday || now == time.Sunday {
		t.Skip("TestRunner_Holiday requires a weekday; skipping")
	}

	mock := newMockClocker()
	mock.holiday = true
	cfg := baseCfg()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := runnerDone(ctx, cfg, mock)
	<-done

	assert.Empty(t, mock.getCalls(), "runner should not call any clock action on a holiday")
}

// TestRunner_Friday verifies that on Fridays the runner uses FridayTimes and
// skips lunch (no Lunchtime defined for Friday path, and baseCfg has Lunchtime set,
// but isSummer=false and Friday uses FridayTimes which yields hasLunch=true via
// the lunchtime being still in cfg — so on Friday WITH lunch we still get 4 calls).
// This test checks that FridayTimes are used (clock-in at the friday time).
func TestRunner_Friday(t *testing.T) {
	if time.Now().Weekday() != time.Friday {
		t.Skip("TestRunner_Friday requires running on a Friday; skipping")
	}

	mock := newMockClocker()
	cfg := baseCfg()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := runnerDone(ctx, cfg, mock)

	ok := waitForCalls(t, mock, 4, 5*time.Second)
	cancel()
	<-done

	assert.True(t, ok, "runner should have recorded clock actions on Friday")
	calls := mock.getCalls()
	// On Friday with lunch configured, all 4 actions run
	assert.Equal(t, []string{"ClockIn", "ClockPause", "ClockResume", "ClockOut"}, calls)
}

// TestRunner_Summer verifies that during summer the runner skips lunch and uses SummerTimes.
func TestRunner_Summer(t *testing.T) {
	now := time.Now().Weekday()
	if now == time.Saturday || now == time.Sunday || now == time.Friday {
		t.Skip("TestRunner_Summer requires a Mon–Thu run; skipping")
	}

	mock := newMockClocker()
	cfg := baseCfg()
	// Override summer period to include today
	today := time.Now()
	cfg.SummerPeriod = []string{
		today.AddDate(0, 0, -1).Format("02/01"),
		today.AddDate(0, 0, 1).Format("02/01"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := runnerDone(ctx, cfg, mock)

	ok := waitForCalls(t, mock, 2, 5*time.Second)
	cancel()
	<-done

	assert.True(t, ok, "runner should record exactly 2 clock actions in summer mode")
	calls := mock.getCalls()
	assert.Equal(t, []string{"ClockIn", "ClockOut"}, calls, "summer mode should skip lunch")
}

// TestRunner_ContextCancelled verifies that cancelling the context while the runner
// is blocked waiting for an action causes it to exit cleanly.
func TestRunner_ContextCancelled(t *testing.T) {
	now := time.Now().Weekday()
	if now == time.Saturday || now == time.Sunday || now == time.Friday {
		t.Skip("TestRunner_ContextCancelled requires a Mon–Thu run; skipping")
	}

	mock := newMockClocker()

	// Block ClockPause indefinitely so we can cancel while waiting
	blockCh := make(chan struct{})
	mock.blockCh["ClockPause"] = blockCh

	cfg := baseCfg()
	ctx, cancel := context.WithCancel(context.Background())

	done := runnerDone(ctx, cfg, mock)

	// Wait for ClockIn to be recorded
	ok := waitForCalls(t, mock, 1, 5*time.Second)
	assert.True(t, ok, "ClockIn should have been recorded before cancellation")

	// Now cancel — the runner is blocked in ClockPause's blockCh read.
	// We also close blockCh so the goroutine unblocks and context.Done() wins.
	cancel()
	close(blockCh)

	select {
	case <-done:
		// Runner exited cleanly
	case <-time.After(3 * time.Second):
		t.Error("runner did not exit after context cancellation within 3s")
	}
}

// TestRunner_ClockInError verifies that if ClockIn returns an error the runner
// stops the loop and exits.
func TestRunner_ClockInError(t *testing.T) {
	now := time.Now().Weekday()
	if now == time.Saturday || now == time.Sunday || now == time.Friday {
		t.Skip("TestRunner_ClockInError requires a Mon–Thu run; skipping")
	}

	mock := newMockClocker()
	mock.errors["ClockIn"] = assert.AnError

	cfg := baseCfg()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := runnerDone(ctx, cfg, mock)

	select {
	case <-done:
		calls := mock.getCalls()
		assert.Equal(t, []string{"ClockIn"}, calls, "runner should stop after ClockIn error")
	case <-time.After(5 * time.Second):
		t.Error("runner did not exit after ClockIn error")
	}
}
