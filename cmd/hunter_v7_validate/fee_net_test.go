package main

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/Aixxww/AiT/store"
	aittrader "github.com/Aixxww/AiT/trader"
)

// TA-7: tracked validation outcomes carry the U5 fee-net walk fields, with
// the values the live tracker computes, and they reach the DB.

func TestValidationFeeNetCostPct(t *testing.T) {
	if got := validationFeeNetCostPct(nil); got != store.DefaultV7FeeNetCostPct {
		t.Fatalf("default cost %v, want %v", got, store.DefaultV7FeeNetCostPct)
	}
	cfg := &store.StrategyConfig{}
	cfg.CoinSource.Hunter = &store.HunterConfig{V7OutcomeTags: &store.V7OutcomeTagConfig{FeeNetCostPct: 0.003}}
	if got := validationFeeNetCostPct(cfg); got != 0.003 {
		t.Fatalf("configured cost %v, want 0.003", got)
	}
}

func TestValidationTrackUpdateCarriesFeeNetFields(t *testing.T) {
	r, d := 0.42, 0.5
	exit := time.Now()
	o := aittrader.TrackedOutcome{RecordID: 9, Status: aittrader.TrackedActive, PnLPct: 1.2, ExitTime: &exit,
		FeeNetOutcome: store.V7FeeNetTP1, FeeNetR: &r, FeeNetEntry: 100.5, DriftAtEntryPct: &d}
	u := validationTrackUpdate(o)
	if u.FeeNetOutcome != store.V7FeeNetTP1 || u.FeeNetR != &r || u.FeeNetEntry != 100.5 || u.DriftAtEntryPct != &d {
		t.Fatalf("fee-net fields not mapped: %+v", u)
	}
	if u.Status != string(aittrader.TrackedActive) || u.PnLPct != 1.2 || u.ExitTime != &exit {
		t.Fatalf("existing fields changed: %+v", u)
	}
}

// End to end: the harness tracker (configured cost) + callback mapping +
// store write. The persisted fee_net_r equals the live formula
// store.V7FeeNetRRAtTP1(E, stop, tp1, cost).
func TestTrackedOutcomeFeeNetPersisted(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "v.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	rec := store.HunterV7SignalRecord{Symbol: "AUSDT", Direction: "LONG", SetupType: "ignition_early_long", Timestamp: time.Now().UTC()}
	recs := []store.HunterV7SignalRecord{rec}
	if err := st.HunterV7Signal().CreateBatch(recs); err != nil {
		t.Fatal(err)
	}
	id := recs[0].ID
	if id <= 0 {
		t.Fatal("record id not assigned")
	}

	cfg := &store.StrategyConfig{}
	cfg.CoinSource.Hunter = &store.HunterConfig{V7OutcomeTags: &store.V7OutcomeTagConfig{FeeNetCostPct: 0.003}}
	cost := validationFeeNetCostPct(cfg)
	tr := aittrader.NewSignalOutcomeTracker(&aittrader.TrackerConfig{
		PollInterval: time.Minute, TimeoutDuration: 2 * time.Hour, MaxTracked: 500, SnapshotLimit: 180,
		ActiveOutcomeInterval: time.Second, TrackPartialTargets: true, FeeNetCostPct: cost,
	}, nil)
	t0 := time.Date(2026, 11, 20, 10, 0, 30, 0, time.UTC)
	bars := []struct{ h, l, c float64 }{{100, 100, 100}, {100.2, 99.9, 100}, {101, 99.5, 100.5}, {101.9, 100.5, 101.5}}
	tr.SetCandleHistorySource(func(_ string, since time.Time) []aittrader.TrackedCandle {
		var out []aittrader.TrackedCandle
		for i, b := range bars {
			ct := t0.Truncate(time.Minute).Add(time.Duration(i+1)*time.Minute - time.Millisecond)
			if ct.After(since) {
				out = append(out, aittrader.TrackedCandle{T: ct, Open: b.c, High: b.h, Low: b.l, Close: b.c})
			}
		}
		return out
	})
	tr.SetOutcomeCallback(func(o aittrader.TrackedOutcome) {
		if err := st.HunterV7Signal().UpdateTrackOutcome(o.RecordID, validationTrackUpdate(o)); err != nil {
			t.Errorf("update: %v", err)
		}
	})
	tr.RegisterWithMeta(aittrader.TrackedSignalMeta{OutcomeSource: store.V7OutcomeSourcePromptOpenReview, TriggerPrice: 99.5},
		id, "AUSDT", "LONG", "ignition_early_long", "REVIEWABLE", 100, 99, 0, 101.8, 103, t0)
	tr.TickNow()

	var got store.HunterV7SignalRecord
	if err := st.GormDB().First(&got, id).Error; err != nil {
		t.Fatal(err)
	}
	want, _ := store.V7FeeNetRRAtTP1(100, 99, 101.8, cost)
	if got.FeeNetOutcome != store.V7FeeNetTP1 || got.FeeNetR == nil || math.Abs(*got.FeeNetR-want) > 1e-12 || got.FeeNetEntry != 100 {
		t.Fatalf("persisted fee_net_outcome=%q fee_net_r=%v fee_net_entry=%v, want tp1 %v at E 100", got.FeeNetOutcome, got.FeeNetR, got.FeeNetEntry, want)
	}
	def, _ := store.V7FeeNetRRAtTP1(100, 99, 101.8, store.DefaultV7FeeNetCostPct)
	if math.Abs(want-def) < 1e-9 {
		t.Fatal("test cost must differ from the default to prove the strategy cost is used")
	}
	if got.DriftAtEntryPct == nil || math.Abs(*got.DriftAtEntryPct-(0.5/99.5*100)) > 1e-9 {
		t.Fatalf("drift_at_entry_pct = %v", got.DriftAtEntryPct)
	}
}
