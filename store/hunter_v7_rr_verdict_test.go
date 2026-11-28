package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestHunterV7RRVerdictStoreRoundTripAndMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rr_verdicts.db")
	st, err := New(path)
	if err != nil {
		t.Fatalf("New store: %v", err)
	}
	defer st.Close()

	var tableCount int
	if err := st.DB().QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'hunter_v7_rr_verdicts'`).Scan(&tableCount); err != nil || tableCount != 1 {
		t.Fatalf("hunter_v7_rr_verdicts table missing (count=%d, err=%v)", tableCount, err)
	}

	targets := []HunterV7RRTargetTerm{
		{Index: 0, Label: "tp0", Price: 101.5, RawRewardPct: 1.5, CappedRewardPct: 1.5, Capped: false},
		{Index: 1, Label: "tp1", Price: 104.5, RawRewardPct: 4.5, CappedRewardPct: 3.25, Capped: true},
	}
	ts := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	rec := HunterV7RRVerdictRecord{
		CycleNumber:            7,
		Timestamp:              ts,
		Stage:                  "prompt_final",
		Symbol:                 "TESTUSDT",
		Direction:              "LONG",
		SetupType:              "trend_breakout_long",
		SignalID:               "7|TESTUSDT|LONG|trend_breakout_long",
		Stack:                  "native",
		SignalClass:            V7ClassTrigger,
		ReferencePrice:         100,
		ReferencePriceSource:   "price_context_last",
		InvalidationPrice:      98.8,
		RawRiskPct:             1.2,
		EffectiveRiskPct:       4.1,
		MinSLMovePct:           4.0,
		MinStopFloorApplied:    true,
		StopRepairBufferPct:    0.10,
		TargetRewardsJSON:      ToJSON(targets),
		MaxTPMovePct:           5.65,
		BestRewardPct:          4.5,
		BestTargetIndex:        1,
		BestTargetLabel:        "tp1",
		BestTargetPrice:        104.5,
		BestTargetRawRewardPct: 4.5,
		BestTargetCapped:       false,
		CappedRR:               4.5 / 4.1,
		MinRRFloor:             1.2,
		OK:                     true,
		Reason:                 "backend_rr_infeasible",
		ResolvedMinRR:          1.2,
		ResolvedMinSLMovePct:   4.0,
		ResolvedMaxTPMovePct:   5.65,
		MinRRSource:            "strategy",
		MinSLSource:            "strategy",
		MaxTPSource:            "strategy",
	}
	if err := st.HunterV7Signal().CreateRRVerdict(&rec); err != nil {
		t.Fatalf("CreateRRVerdict: %v", err)
	}
	if rec.ID <= 0 {
		t.Fatalf("expected auto-increment ID, got %d", rec.ID)
	}

	byCycle, err := st.HunterV7Signal().QueryRRVerdictsByCycle(7)
	if err != nil {
		t.Fatalf("QueryRRVerdictsByCycle: %v", err)
	}
	if len(byCycle) != 1 {
		t.Fatalf("len(byCycle) = %d, want 1", len(byCycle))
	}
	got := byCycle[0]
	if got.Symbol != rec.Symbol || got.Stage != rec.Stage || got.SignalID != rec.SignalID ||
		got.ReferencePrice != rec.ReferencePrice || got.ReferencePriceSource != rec.ReferencePriceSource ||
		got.RawRiskPct != rec.RawRiskPct || got.EffectiveRiskPct != rec.EffectiveRiskPct ||
		!got.MinStopFloorApplied || got.StopRepairBufferPct != 0.10 ||
		got.BestTargetIndex != 1 || got.BestTargetLabel != "tp1" || got.BestTargetPrice != 104.5 ||
		got.CappedRR != rec.CappedRR || got.MinRRFloor != 1.2 || !got.OK || got.Reason != "backend_rr_infeasible" ||
		got.MinRRSource != "strategy" || got.MinSLSource != "strategy" || got.MaxTPSource != "strategy" {
		t.Fatalf("round-trip mismatch:\ngot:  %+v\nwant: %+v", got, rec)
	}

	var decodedTargets []HunterV7RRTargetTerm
	if err := json.Unmarshal([]byte(got.TargetRewardsJSON), &decodedTargets); err != nil {
		t.Fatalf("unmarshal target_rewards_json: %v", err)
	}
	if len(decodedTargets) != 2 || decodedTargets[1].Label != "tp1" || !decodedTargets[1].Capped {
		t.Fatalf("decoded targets mismatch: %+v", decodedTargets)
	}

	recent, err := st.HunterV7Signal().QueryRRVerdicts(10)
	if err != nil || len(recent) != 1 {
		t.Fatalf("QueryRRVerdicts: len=%d err=%v", len(recent), err)
	}
}
