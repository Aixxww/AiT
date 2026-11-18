package store

import (
	"math"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestV7BandLabel(t *testing.T) {
	edges := DefaultV7RRBandEdges
	for _, c := range []struct {
		v    float64
		want string
	}{{0.9, "lt_1.0"}, {1.0, "1.0_1.2"}, {1.19, "1.0_1.2"}, {1.2, "1.2_1.5"}, {1.99, "1.5_2.0"}, {2.0, "ge_2.0"}, {9, "ge_2.0"}} {
		if got := V7BandLabel(c.v, true, edges); got != c.want {
			t.Errorf("band(%v) = %s, want %s", c.v, got, c.want)
		}
	}
	if got := V7BandLabel(1.3, false, edges); got != V7BandUnknown {
		t.Errorf("missing input = %s", got)
	}
	if got := V7BandLabel(0.7, true, DefaultV7StopATRBandEdges); got != "0.5_1.0" {
		t.Errorf("stop/ATR band = %s", got)
	}
}

// The OD-2 formula as used by U0-A: (|tp1-E| - E*c) / (|E-stop| + E*c).
func TestV7FeeNetRRAtTP1(t *testing.T) {
	rr, ok := V7FeeNetRRAtTP1(100, 99, 101.8, 0.0015)
	if want := (1.8 - 0.15) / (1 + 0.15); !ok || math.Abs(rr-want) > 1e-12 {
		t.Fatalf("long rr = %v, want %v", rr, want)
	}
	rr, _ = V7FeeNetRRAtTP1(100, 101, 98.2, 0.0015)
	if want := (1.8 - 0.15) / (1 + 0.15); math.Abs(rr-want) > 1e-12 {
		t.Fatalf("short rr = %v, want %v", rr, want)
	}
	if _, ok := V7FeeNetRRAtTP1(0, 99, 101, 0.0015); ok {
		t.Fatalf("no entry must not be ok")
	}
}

func TestV7FamilyAndEffectiveOutcomeTags(t *testing.T) {
	if V7Family("ignition_early_short") != "ignition_early" || V7Family("range_expansion_event") != "range_expansion_event" {
		t.Fatalf("family suffix stripping")
	}
	d := EffectiveV7OutcomeTags(nil)
	if d.FeeNetCostPct != 0.0015 || len(d.RRBandEdges) != 4 || len(d.StopATRBandEdges) != 5 || d.WatchAudit || d.WatchAuditMaxTracked != 200 {
		t.Fatalf("defaults = %+v", d)
	}
	c := EffectiveV7OutcomeTags(&V7OutcomeTagConfig{FeeNetCostPct: 0.001, RRBandEdges: []float64{1, 2}, WatchAudit: true})
	if c.FeeNetCostPct != 0.001 || len(c.RRBandEdges) != 2 || !c.WatchAudit {
		t.Fatalf("overrides = %+v", c)
	}
}

func newV7SignalTestStore(t *testing.T) (*gorm.DB, *HunterV7SignalStore) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&HunterV7SignalRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db, NewHunterV7SignalStore(db)
}

func TestUpdateTrackOutcomeWritesFeeNetOnlyWhenResolved(t *testing.T) {
	db, s := newV7SignalTestStore(t)
	if err := s.CreateBatch([]HunterV7SignalRecord{{CycleNumber: 1, Timestamp: time.Now().UTC(), Symbol: "A", Direction: "LONG", SetupType: "x", Status: "candidate"}}); err != nil {
		t.Fatal(err)
	}
	var rec HunterV7SignalRecord
	db.First(&rec)
	if err := s.UpdateTrackOutcome(rec.ID, HunterV7SignalTrackUpdate{Status: "ACTIVE"}); err != nil {
		t.Fatal(err)
	}
	db.First(&rec, rec.ID)
	if rec.FeeNetR != nil || rec.FeeNetOutcome != "" {
		t.Fatalf("pending walk must leave fee-net columns empty: %+v", rec)
	}
	r, d := 1.43, 0.2
	if err := s.UpdateTrackOutcome(rec.ID, HunterV7SignalTrackUpdate{Status: "WIN_TP1", FeeNetOutcome: V7FeeNetTP1, FeeNetR: &r, FeeNetEntry: 100.5, DriftAtEntryPct: &d}); err != nil {
		t.Fatal(err)
	}
	db.First(&rec, rec.ID)
	if rec.FeeNetR == nil || *rec.FeeNetR != 1.43 || rec.FeeNetOutcome != V7FeeNetTP1 || rec.FeeNetEntry != 100.5 || rec.DriftAtEntryPct == nil || *rec.DriftAtEntryPct != 0.2 {
		t.Fatalf("fee-net columns = %+v", rec)
	}
}

// watch_audit and shadow outcomes stay out of the real win-rate
// denominators; pre-U5 rows (no source) and open-review rows stay in.
func TestOutcomeStatsExcludeAuditSources(t *testing.T) {
	_, s := newV7SignalTestStore(t)
	base := time.Now().UTC().Add(-time.Hour)
	exit := base.Add(10 * time.Minute)
	mk := func(source, status string) HunterV7SignalRecord {
		return HunterV7SignalRecord{CycleNumber: 1, Timestamp: base, Symbol: "A", Direction: "LONG", SetupType: "x", Status: "candidate",
			MarketRegime: "trend_up", TrackStatus: status, TrackExitTime: &exit, OutcomeSource: source}
	}
	if err := s.CreateBatch([]HunterV7SignalRecord{
		mk("", "WIN_TP1"), mk(V7OutcomeSourcePromptOpenReview, "STOP"),
		mk(V7OutcomeSourceWatchAudit, "WIN_TP1"), mk(V7OutcomeSourceShadow, "WIN_TP1"),
	}); err != nil {
		t.Fatal(err)
	}
	w, err := s.OutcomeWindowStats(base.Add(-time.Minute), base.Add(time.Minute), time.Hour, nil, "w")
	if err != nil {
		t.Fatal(err)
	}
	if w.Total != 2 || w.Wins != 1 {
		t.Fatalf("window stats total/wins = %d/%d, want 2/1", w.Total, w.Wins)
	}
	sr, err := s.SetupRegimeOutcomeStats(base.Add(-time.Minute), base.Add(time.Minute), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sr) != 1 || sr[0].Total != 2 {
		t.Fatalf("setup/regime stats = %+v", sr)
	}
}
