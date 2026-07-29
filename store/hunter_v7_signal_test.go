package store

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestHunterV7SignalStoreUpdateTrackOutcome(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&HunterV7SignalRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	store := NewHunterV7SignalStore(db)
	rec := HunterV7SignalRecord{
		CycleNumber: 1,
		Timestamp:   time.Now().UTC(),
		Symbol:      "TESTUSDT",
		Direction:   "LONG",
		SetupType:   "leader_momentum_long",
		Status:      "candidate",
	}
	if err := store.CreateBatch([]HunterV7SignalRecord{rec}); err != nil {
		t.Fatalf("create: %v", err)
	}

	var saved HunterV7SignalRecord
	if err := db.First(&saved).Error; err != nil {
		t.Fatalf("load saved: %v", err)
	}

	exitAt := time.Now().UTC()
	err = store.UpdateTrackOutcome(saved.ID, HunterV7SignalTrackUpdate{
		Status:       "WIN_TP0",
		CurrentPrice: 101,
		ExitPrice:    101,
		StopPrice:    99,
		PnLPct:       1,
		MFE:          1.2,
		MAE:          -0.3,
		ExitTime:     &exitAt,
		Snapshots:    `[{"price":101}]`,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	var updated HunterV7SignalRecord
	if err := db.First(&updated, saved.ID).Error; err != nil {
		t.Fatalf("load updated: %v", err)
	}
	if updated.TrackStatus != "WIN_TP0" || updated.TrackPnLPct != 1 || updated.TrackMFE != 1.2 || updated.TrackStopPrice != 99 {
		t.Fatalf("unexpected tracking fields: %+v", updated)
	}
	if updated.TrackExitTime == nil {
		t.Fatalf("expected track exit time")
	}
}

func TestHunterV7SignalStoreOutcomeStats(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&HunterV7SignalRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := NewHunterV7SignalStore(db)

	base := time.Now().UTC().Add(-2 * time.Hour)
	exit10m := base.Add(10 * time.Minute)
	exit90m := base.Add(90 * time.Minute)
	exit3h := base.Add(3 * time.Hour)
	records := []HunterV7SignalRecord{
		{CycleNumber: 1, Timestamp: base, Symbol: "A", Direction: "LONG", SetupType: "pullback_long", MarketRegime: "trend_up", Status: "candidate", ExecutionTier: "EXECUTABLE", TrackStatus: "WIN_TP0", TrackExitTime: &exit10m, TrackPnLPct: 1, TrackMFE: 1.5, TrackMAE: -0.2},
		{CycleNumber: 1, Timestamp: base, Symbol: "B", Direction: "LONG", SetupType: "pullback_long", MarketRegime: "trend_up", Status: "candidate", ExecutionTier: "EXECUTABLE", TrackStatus: "STOP", TrackExitTime: &exit10m, TrackPnLPct: -1, TrackMFE: 0.4, TrackMAE: -1.1},
		{CycleNumber: 1, Timestamp: base, Symbol: "C", Direction: "LONG", SetupType: "squeeze_breakout", MarketRegime: "compression", Status: "candidate", ExecutionTier: "REVIEWABLE", TrackStatus: "WIN_TP1", TrackExitTime: &exit90m, TrackPnLPct: 2.5, TrackMFE: 3, TrackMAE: -0.3},
		{CycleNumber: 1, Timestamp: base, Symbol: "D", Direction: "LONG", SetupType: "squeeze_breakout", MarketRegime: "compression", Status: "candidate", ExecutionTier: "REVIEWABLE", TrackStatus: "WIN_TP1", TrackExitTime: &exit3h, TrackPnLPct: 2, TrackMFE: 2.2, TrackMAE: -0.4},
	}
	if err := store.CreateBatch(records); err != nil {
		t.Fatalf("create: %v", err)
	}

	tp0, err := store.OutcomeWindowStats(base.Add(-time.Minute), base.Add(time.Minute), 30*time.Minute, []string{"WIN_TP0", "WIN_TP1", "WIN_TP2"}, "30m_tp0")
	if err != nil {
		t.Fatalf("tp0 stats: %v", err)
	}
	if tp0.Total != 2 || tp0.Wins != 1 || tp0.WinRate != 50 {
		t.Fatalf("unexpected tp0 stats: %+v", tp0)
	}

	tp1, err := store.OutcomeWindowStats(base.Add(-time.Minute), base.Add(time.Minute), 2*time.Hour, []string{"WIN_TP1", "WIN_TP2"}, "2h_tp1")
	if err != nil {
		t.Fatalf("tp1 stats: %v", err)
	}
	if tp1.Total != 3 || tp1.Wins != 1 {
		t.Fatalf("unexpected tp1 stats: %+v", tp1)
	}

	grouped, err := store.SetupRegimeOutcomeStats(base.Add(-time.Minute), base.Add(time.Minute), 2)
	if err != nil {
		t.Fatalf("grouped stats: %v", err)
	}
	if len(grouped) != 2 {
		t.Fatalf("grouped len = %d, want 2: %+v", len(grouped), grouped)
	}
}

func TestHunterV7SignalStoreRecentSignalsPrioritizesActionableRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&HunterV7SignalRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := NewHunterV7SignalStore(db)
	ts := time.Now().UTC()
	records := []HunterV7SignalRecord{
		{CycleNumber: 1, Timestamp: ts, Symbol: "EXEC", ExecutionTier: "EXECUTABLE", AIPriority: 80},
		{CycleNumber: 1, Timestamp: ts, Symbol: "REV", ExecutionTier: "REVIEWABLE", AIPriority: 70},
		{CycleNumber: 1, Timestamp: ts, Symbol: "MISS", ExecutionTier: "REJECTED", TierReason: "module_no_match", AIPriority: 0},
	}
	if err := store.CreateBatch(records); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := store.RecentSignals(2)
	if err != nil {
		t.Fatalf("recent signals: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Symbol != "EXEC" || got[1].Symbol != "REV" {
		t.Fatalf("recent order = %s,%s; want EXEC,REV", got[0].Symbol, got[1].Symbol)
	}
}
