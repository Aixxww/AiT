package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestClosePositionFullyPreservesProtectedCloseIntent(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "close-intent.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})

	nowMs := time.Now().UTC().UnixMilli()
	if err := st.Position().CreateOpenPosition(&TraderPosition{
		TraderID:     "trader-1",
		ExchangeID:   "exchange-1",
		ExchangeType: "binance",
		Symbol:       "IDOLUSDT",
		Side:         "SHORT",
		Quantity:     100,
		EntryPrice:   0.01636,
		EntryTime:    nowMs,
		Leverage:     20,
		Status:       "OPEN",
		Source:       "sync",
		CreatedAt:    nowMs,
		UpdatedAt:    nowMs,
	}); err != nil {
		t.Fatalf("create open position: %v", err)
	}
	pos, err := st.Position().GetOpenPositionBySymbol("trader-1", "IDOLUSDT", "SHORT")
	if err != nil || pos == nil {
		t.Fatalf("get open position: pos=%v err=%v", pos, err)
	}
	if err := st.Position().MarkPositionCloseIntent(pos.ID, "giveback_close", "system_protector"); err != nil {
		t.Fatalf("mark close intent: %v", err)
	}
	if err := st.Position().ClosePositionFully(pos.ID, 0.01630, "close-1", nowMs+1000, 1.20, 0.02, "sync"); err != nil {
		t.Fatalf("close position: %v", err)
	}

	closed, err := st.Position().GetLatestPositionBySymbol("trader-1", "IDOLUSDT", "SHORT")
	if err != nil || closed == nil {
		t.Fatalf("get closed position: pos=%v err=%v", closed, err)
	}
	if closed.CloseReason != "giveback_close" {
		t.Fatalf("close reason = %q, want giveback_close", closed.CloseReason)
	}
	if closed.Source != "system_protector" {
		t.Fatalf("source = %q, want system_protector", closed.Source)
	}
}
