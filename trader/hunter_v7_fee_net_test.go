package trader

import (
	"math"
	"testing"
	"time"

	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// feeNetHarness feeds a fixed 1m bar list (T = close time, as live) to a
// fresh tracker registered at t0 and returns the last outcome.
type feeNetBar struct{ h, l, c float64 }

func runFeeNet(t *testing.T, cfg *TrackerConfig, meta TrackedSignalMeta, tier, dir string, entry, stop, tp0, tp1, tp2 float64, bars []feeNetBar) (*SignalOutcomeTracker, TrackedOutcome) {
	t.Helper()
	t0 := time.Date(2026, 9, 1, 10, 0, 30, 0, time.UTC) // mid-minute signal
	if cfg == nil {
		cfg = DefaultTrackerConfig()
		cfg.EnableDynamicStop = false
	}
	tr := NewSignalOutcomeTracker(cfg, nil)
	tr.SetCandleHistorySource(func(_ string, since time.Time) []TrackedCandle {
		var out []TrackedCandle
		open := t0.Truncate(time.Minute) // the signal minute itself, then the following ones
		for i, b := range bars {
			ct := open.Add(time.Duration(i+1)*time.Minute - time.Millisecond)
			if ct.After(since) {
				out = append(out, TrackedCandle{T: ct, Open: b.c, High: b.h, Low: b.l, Close: b.c})
			}
		}
		return out
	})
	var last TrackedOutcome
	tr.SetOutcomeCallback(func(o TrackedOutcome) { last = o })
	tr.RegisterWithMeta(meta, 1, "AUSDT", dir, "ignition_early_long", tier, entry, stop, tp0, tp1, tp2, t0)
	tr.TickNow()
	return tr, last
}

const fnLong, fnShort = string(local.V7DirLong), string(local.V7DirShort)

// Bar 0 is the signal minute (ignored), bar 1 the anchor (E = its close).
func TestFeeNetTP1First(t *testing.T) {
	_, o := runFeeNet(t, nil, TrackedSignalMeta{OutcomeSource: store.V7OutcomeSourcePromptOpenReview, TriggerPrice: 99.5}, "REVIEWABLE", fnLong, 100, 99, 0, 101.8, 103,
		[]feeNetBar{{120, 80, 100}, {100.2, 99.9, 100}, {101, 99.5, 100.5}, {101.9, 100.5, 101.5}})
	want, _ := store.V7FeeNetRRAtTP1(100, 99, 101.8, store.DefaultV7FeeNetCostPct)
	if o.FeeNetOutcome != store.V7FeeNetTP1 || o.FeeNetR == nil || math.Abs(*o.FeeNetR-want) > 1e-12 || o.FeeNetEntry != 100 {
		t.Fatalf("outcome = %s r=%v E=%v, want tp1 %v at E 100", o.FeeNetOutcome, o.FeeNetR, o.FeeNetEntry, want)
	}
	if o.DriftAtEntryPct == nil || math.Abs(*o.DriftAtEntryPct-(0.5/99.5*100)) > 1e-9 {
		t.Fatalf("drift = %v", o.DriftAtEntryPct)
	}
}

// Stop first when one bar touches both; SHORT mirrors LONG; drift sign is
// directional (adverse = positive).
func TestFeeNetStopFirstInSameBarShort(t *testing.T) {
	_, o := runFeeNet(t, nil, TrackedSignalMeta{TriggerPrice: 100.5}, "REVIEWABLE", fnShort, 100, 101, 0, 98.2, 97,
		[]feeNetBar{{100, 100, 100}, {100.1, 99.9, 100}, {101.1, 98.0, 99}})
	if o.FeeNetOutcome != store.V7FeeNetStop || o.FeeNetR == nil || *o.FeeNetR != -1 {
		t.Fatalf("outcome = %s r=%v, want stop -1", o.FeeNetOutcome, o.FeeNetR)
	}
	if o.DriftAtEntryPct == nil || *o.DriftAtEntryPct <= 0 {
		t.Fatalf("short entry below the trigger is a chase (adverse, positive drift), got %v", o.DriftAtEntryPct)
	}
}

func TestFeeNetDegenerateAndHorizon(t *testing.T) {
	_, o := runFeeNet(t, nil, TrackedSignalMeta{}, "REVIEWABLE", fnLong, 100, 99, 0, 101.8, 103,
		[]feeNetBar{{100, 100, 100}, {99, 98.5, 98.9}})
	if o.FeeNetOutcome != store.V7FeeNetDegenerateStop || o.FeeNetR != nil {
		t.Fatalf("degenerate = %s %v", o.FeeNetOutcome, o.FeeNetR)
	}
	cfg := DefaultTrackerConfig()
	cfg.EnableDynamicStop = false
	cfg.TimeoutDuration = 3 * time.Minute
	_, o = runFeeNet(t, cfg, TrackedSignalMeta{}, "REVIEWABLE", fnLong, 100, 99, 0, 101.8, 103,
		[]feeNetBar{{100, 100, 100}, {100, 100, 100}, {100.5, 99.5, 100}, {100.5, 99.5, 100}, {102, 100, 101.9}})
	if o.FeeNetOutcome != store.V7FeeNetNone || o.FeeNetR == nil || *o.FeeNetR != 0 {
		t.Fatalf("horizon = %s %v, want none 0 (TP1 bar is after the horizon)", o.FeeNetOutcome, o.FeeNetR)
	}
}

// The PnL path can end first (here: TP0 partial, then the breakeven runner
// is stopped at entry). The fee-net walk keeps the plan stop and continues
// to TP1 in the same and later ticks.
func TestFeeNetContinuesAfterPnLOutcome(t *testing.T) {
	tr, o := runFeeNet(t, nil, TrackedSignalMeta{}, "REVIEWABLE", fnLong, 100, 99, 100.6, 101.8, 103,
		[]feeNetBar{{100, 100, 100}, {100, 100, 100}, {100.7, 100.1, 100.6}, {100.4, 99.95, 100.2}, {101.9, 100.2, 101.8}})
	if o.Status != TrackedProtectedStop {
		t.Fatalf("PnL status = %s, want PROTECTED_STOP", o.Status)
	}
	if o.FeeNetOutcome != store.V7FeeNetTP1 {
		t.Fatalf("fee-net outcome = %s, want tp1", o.FeeNetOutcome)
	}
	if len(tr.feeNetPending) != 0 || len(tr.GetActive()) != 0 {
		t.Fatalf("pending/active not cleared: %d/%d", len(tr.feeNetPending), len(tr.GetActive()))
	}
}

// Audit rows have their own capacity and thesis slot and stay out of Total.
func TestWatchAuditBucketSeparated(t *testing.T) {
	cfg := DefaultTrackerConfig()
	cfg.MaxTracked, cfg.WatchAuditMaxTracked = 1, 1
	tr := NewSignalOutcomeTracker(cfg, func(string) float64 { return 100 })
	now := time.Now()
	audit := TrackedSignalMeta{OutcomeSource: store.V7OutcomeSourceWatchAudit}
	if ok, _ := tr.RegisterWithMeta(audit, 1, "A", fnLong, "s", "WATCH", 100, 99, 0, 101, 102, now); !ok {
		t.Fatalf("audit row must register")
	}
	if ok, _ := tr.RegisterWithMeta(audit, 2, "B", fnLong, "s", "WATCH", 100, 99, 0, 101, 102, now); ok {
		t.Fatalf("audit cap must hold")
	}
	ok, replaced := tr.RegisterWithMeta(TrackedSignalMeta{OutcomeSource: store.V7OutcomeSourcePromptOpenReview}, 3, "A", fnLong, "s", "REVIEWABLE", 100, 99, 0, 101, 102, now.Add(time.Minute))
	if !ok || replaced != 0 {
		t.Fatalf("open-review row must register beside the audit row without replacing it (ok=%v replaced=%d)", ok, replaced)
	}
	tr.completed = append(tr.completed,
		&TrackedSignal{SetupType: "s", Tier: "WATCH", OutcomeSource: store.V7OutcomeSourceShadow, Status: TrackedStop},
		&TrackedSignal{SetupType: "s", Tier: "REVIEWABLE", OutcomeSource: store.V7OutcomeSourcePromptOpenReview, Status: TrackedStop, FeeNetR: ptrF(-1)},
	)
	st := tr.GetStatsBySetupType()["s"]
	if st.Total != 1 || st.AuditTracked != 2 || st.FeeNetN != 1 || st.AvgFeeNetR != -1 {
		t.Fatalf("stats = %+v", st)
	}
}

func ptrF(v float64) *float64 { return &v }
