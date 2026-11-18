package store

import (
	"testing"
	"time"
)

func TestEffectiveV7ExpectedRDefaults(t *testing.T) {
	c := EffectiveV7ExpectedR(nil)
	if c.Enabled || c.MinRows != 30 || c.LowerBoundConfidence != 0.9 || c.RefitMinNewOutcomes != 25 || c.RefitMaxAgeHours != 720 ||
		c.AdmitFraction != 0.5 || c.RandomDraws != 1000 || c.Quantiles != 5 || c.F3MaxP != 0.1 || c.Seed != 20261118 || len(c.KGrid) != 9 {
		t.Fatalf("defaults = %+v", c)
	}
	if o := EffectiveV7ExpectedR(&V7ExpectedRConfig{Enabled: true, MinRows: 50, KGrid: []float64{3}}); !o.Enabled || o.MinRows != 50 || len(o.KGrid) != 1 {
		t.Fatalf("overrides = %+v", o)
	}
}

func TestCalibrationOutcomesQuery(t *testing.T) {
	db, s := newV7SignalTestStore(t)
	now := time.Now().UTC()
	r := 1.43
	recs := []HunterV7SignalRecord{
		{CycleNumber: 1, Timestamp: now, Symbol: "A", Direction: "LONG", SetupType: "ignition_early_long", Status: "c",
			SignalClass: V7ClassTrigger, Family: "ignition_early", RRBand: "1.2_1.5", InvalidationPrice: 99, TP1Price: 101.8},
		{CycleNumber: 1, Timestamp: now, Symbol: "B", Direction: "LONG", SetupType: "ignition_early_long", Status: "c"},
		{CycleNumber: 1, Timestamp: now.Add(-48 * time.Hour), Symbol: "C", Direction: "SHORT", SetupType: "x_short", Status: "c",
			InvalidationPrice: 101, Target1: 98.2},
	}
	if err := s.CreateBatch(recs); err != nil {
		t.Fatal(err)
	}
	var saved []HunterV7SignalRecord
	db.Order("id").Find(&saved)
	_ = s.UpdateTrackOutcome(saved[0].ID, HunterV7SignalTrackUpdate{Status: "WIN_TP1", FeeNetOutcome: V7FeeNetTP1, FeeNetR: &r, FeeNetEntry: 100})
	_ = s.UpdateTrackOutcome(saved[2].ID, HunterV7SignalTrackUpdate{Status: "STOP", FeeNetOutcome: V7FeeNetStop, FeeNetEntry: 100})
	all, err := s.CalibrationOutcomes(time.Time{}, 0.0015)
	if err != nil || len(all) != 2 {
		t.Fatalf("all = %v %v", all, err)
	}
	want, _ := V7FeeNetRRAtTP1(100, 99, 101.8, 0.0015)
	if all[1].Outcome != V7FeeNetTP1 || all[1].RWin != want || all[1].Class != V7ClassTrigger {
		t.Fatalf("row = %+v", all[1])
	}
	if all[0].RWin <= 0 { // TP1 from target_1 when tp1_price is empty
		t.Fatalf("target_1 fallback = %+v", all[0])
	}
	recent, _ := s.CalibrationOutcomes(now.Add(-time.Hour), 0.0015)
	if len(recent) != 1 {
		t.Fatalf("window = %d", len(recent))
	}
}
