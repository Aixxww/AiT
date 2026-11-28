package main

import (
	"path/filepath"
	"testing"

	"github.com/Aixxww/AiT/store"
)

func TestGenerateReport_DoesNotCountConditionalPendingInOpenCount(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cycle_review.db")
	st, err := store.New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer st.Close()

	traderID := "tr-review"
	// Cycle 1: conditional pending registration (Success=true, ConditionalPending=true) -> NOT an open.
	if err := st.Decision().LogDecision(&store.DecisionRecord{
		TraderID:    traderID,
		CycleNumber: 1,
		Success:     true,
		Decisions: []store.DecisionAction{
			{Action: "open_short", Symbol: "BLESSUSDT", Success: true, ConditionalPending: true},
		},
	}); err != nil {
		t.Fatalf("log cycle 1: %v", err)
	}
	// Cycle 2: conditional entry fired (Success=true, ConditionalPending=false) -> counts as 1 open.
	if err := st.Decision().LogDecision(&store.DecisionRecord{
		TraderID:    traderID,
		CycleNumber: 2,
		Success:     true,
		Decisions: []store.DecisionAction{
			{Action: "open_short", Symbol: "BLESSUSDT", Success: true, ConditionalPending: false},
		},
	}); err != nil {
		t.Fatalf("log cycle 2: %v", err)
	}
	// Cycle 3: cancelled pending registration (Success=true, ConditionalPending=true) -> NOT an open.
	if err := st.Decision().LogDecision(&store.DecisionRecord{
		TraderID:    traderID,
		CycleNumber: 3,
		Success:     true,
		Decisions: []store.DecisionAction{
			{Action: "open_long", Symbol: "ICNTUSDT", Success: true, ConditionalPending: true},
		},
	}); err != nil {
		t.Fatalf("log cycle 3: %v", err)
	}

	report, err := GenerateReport(st.GormDB(), traderID, 10)
	if err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}
	if report.OpenCount != 1 {
		t.Fatalf("OpenCount = %d, want 1 (ConditionalPending registrations must not be counted)", report.OpenCount)
	}
}
