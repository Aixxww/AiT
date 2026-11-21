package main

import (
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/datafetch"
)

// Fix 03: the truncation gate and the fetch budget wiring.
func TestDetailTruncationGate(t *testing.T) {
	m := func(req, skipped int) datafetch.SnapshotMeta {
		return datafetch.SnapshotMeta{DetailRequested: req, DetailCompleted: req - skipped, DetailSkippedDeadline: skipped}
	}
	if detailTruncationError(m(220, 22), 10) != nil {
		t.Fatal("exactly 10% must pass")
	}
	err := detailTruncationError(m(220, 23), 10)
	if err == nil || !strings.Contains(err.Error(), "23 of 220 detail symbols skipped (10.5% > 10.0%)") {
		t.Fatalf("23/220 at 10%%: %v", err)
	}
	if class, _ := classifyRoundError(transientError(err)); class != failTransient {
		t.Fatal("truncation must be a transient failure")
	}
	if detailTruncationError(m(220, 0), 0) != nil || detailTruncationError(m(220, 1), 0) == nil {
		t.Fatal("0% = any skip fails")
	}
	if detailTruncationError(m(220, 200), 100) != nil || detailTruncationError(m(0, 0), 10) != nil {
		t.Fatal(">= 100 disables; no detail phase passes")
	}
}

// A truncated round is retried as transient and, if every retry is
// truncated, stops the run with the resumable exit code (TA-2).
func TestTruncatedRoundIsRetriedThenExit2(t *testing.T) {
	trunc := detailTruncationError(datafetch.SnapshotMeta{DetailRequested: 220, DetailSkippedDeadline: 40}, 10)
	att := &scriptedAttempts{errs: []error{transientError(trunc)}}
	sl := &sleepRecorder{}
	out := runRoundsWithPolicy(1, 3, time.Second, defaultRoundPolicy(), newRunBreaker(3), att.fn, sl.sleep, nil)
	if out.exitCode != exitTransient || att.calls != 4 || len(out.reports) != 0 || len(out.rejected) != 0 {
		t.Fatalf("exit %d calls %d reports %d rejected %d, want 2/4/0/0", out.exitCode, att.calls, len(out.reports), len(out.rejected))
	}
	att = &scriptedAttempts{errs: []error{transientError(trunc), nil}}
	if res := runRoundWithPolicy(1, defaultRoundPolicy(), att.fn, (&sleepRecorder{}).sleep); res.status != roundAccepted {
		t.Fatalf("a retry within the limit is accepted: %s", res.status)
	}
}

func TestRoundFetcherConfigCarriesBudget(t *testing.T) {
	if c := roundFetcherConfig(validationOptions{}, false, false); c.FetchBudget != 3*time.Minute {
		t.Fatalf("default budget %v, want 3m", c.FetchBudget)
	}
	if c := roundFetcherConfig(validationOptions{fetchBudget: 150 * time.Second}, false, false); c.FetchBudget != 150*time.Second {
		t.Fatalf("configured budget %v", c.FetchBudget)
	}
}
