package sleeper

import (
	"testing"
	"time"
)

func allow(string, time.Time) bool { return true }

// TestUpdateOptimisticallySetsSleeping documents the existing (intentional)
// behavior that Update() flags a device Sleeping=true as soon as it decides
// to sleep it, before any real sleep command has run. MarkSleepResult is
// what reconciles this against the actual outcome (see below).
func TestUpdateOptimisticallySetsSleeping(t *testing.T) {
	e := NewEngine([]string{"c0t0d0"}, Options{Wait: 10 * time.Second, VerifyIdle: 1, AllowSleep: allow})
	base := time.Unix(0, 0)

	// First sample: device goes idle.
	e.Update(map[string]bool{}, base)
	// Second sample: idle long enough, verify countdown expires.
	sleep, _ := e.Update(map[string]bool{}, base.Add(11*time.Second))
	if len(sleep) != 1 || sleep[0] != "c0t0d0" {
		t.Fatalf("expected c0t0d0 to be selected for sleep, got %v", sleep)
	}
	snap := e.Snapshot()
	if !snap[0].Sleeping {
		t.Fatalf("expected Sleeping=true immediately after Update() selects the device, before any command ran")
	}
}

// TestMarkSleepResultRevertsOnFailure is the regression test for the false
// "sleeping" bug: a failed real sleep attempt (bad device path, smartctl
// error, hardware/driver limitation, etc.) must not leave state.json
// reporting the device as asleep.
func TestMarkSleepResultRevertsOnFailure(t *testing.T) {
	e := NewEngine([]string{"c0t0d0"}, Options{Wait: 10 * time.Second, VerifyIdle: 1, AllowSleep: allow})
	base := time.Unix(0, 0)
	e.Update(map[string]bool{}, base)
	e.Update(map[string]bool{}, base.Add(11*time.Second))

	failAt := base.Add(12 * time.Second)
	e.MarkSleepResult("c0t0d0", false, failAt)

	snap := e.Snapshot()
	if snap[0].Sleeping {
		t.Fatalf("Sleeping must be false after a failed sleep attempt, got true")
	}
	if !snap[0].IdleSince.Equal(failAt) {
		t.Fatalf("IdleSince should restart at the failure time %v, got %v", failAt, snap[0].IdleSince)
	}

	// The engine must not immediately retry -- it needs to wait a full
	// Wait+VerifyIdle cycle again, not just the next sample.
	sleepAgain, _ := e.Update(map[string]bool{}, failAt.Add(1*time.Second))
	if len(sleepAgain) != 0 {
		t.Fatalf("expected no immediate retry right after a failure, got %v", sleepAgain)
	}

	// But after waiting the full cycle again, it should retry.
	sleepRetry, _ := e.Update(map[string]bool{}, failAt.Add(11*time.Second))
	if len(sleepRetry) != 1 || sleepRetry[0] != "c0t0d0" {
		t.Fatalf("expected a retry after waiting a full cycle, got %v", sleepRetry)
	}
}

// TestMarkSleepResultKeepsStateOnSuccess ensures a successful sleep leaves
// the optimistic Sleeping=true flag untouched.
func TestMarkSleepResultKeepsStateOnSuccess(t *testing.T) {
	e := NewEngine([]string{"c0t0d0"}, Options{Wait: 10 * time.Second, VerifyIdle: 1, AllowSleep: allow})
	base := time.Unix(0, 0)
	e.Update(map[string]bool{}, base)
	e.Update(map[string]bool{}, base.Add(11*time.Second))

	e.MarkSleepResult("c0t0d0", true, base.Add(12*time.Second))

	snap := e.Snapshot()
	if !snap[0].Sleeping {
		t.Fatalf("Sleeping should remain true after a successful sleep attempt")
	}
}

// TestMarkSleepResultUnknownDevice must not panic on a device name that
// isn't tracked (e.g. removed from the pool between Update() and the
// caller's command).
func TestMarkSleepResultUnknownDevice(t *testing.T) {
	e := NewEngine([]string{"c0t0d0"}, Options{Wait: 10 * time.Second, VerifyIdle: 1, AllowSleep: allow})
	e.MarkSleepResult("does-not-exist", false, time.Unix(0, 0))
}
