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

func TestPositionBuilderAppliesPendingProtectedCloseIntent(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "pending-close-intent.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})

	nowMs := time.Now().UTC().UnixMilli()
	if err := st.Position().RecordPendingCloseIntent("trader-1", "LABUSDT", "SHORT", "hard_loss_close", "system_protector"); err != nil {
		t.Fatalf("record pending intent: %v", err)
	}

	builder := NewPositionBuilder(st.Position())
	if err := builder.ProcessTradeWithLeverage("trader-1", "exchange-1", "binance", "LABUSDT", "SHORT", "open_short", 2, 7.198, 0.007, 0, nowMs, "open-1", 10); err != nil {
		t.Fatalf("process open: %v", err)
	}
	if err := builder.ProcessTradeWithLeverage("trader-1", "exchange-1", "binance", "LABUSDT", "SHORT", "close_short", 2, 7.295, 0.007, -0.194, nowMs+33000, "close-1", 10); err != nil {
		t.Fatalf("process close: %v", err)
	}

	closed, err := st.Position().GetLatestPositionBySymbol("trader-1", "LABUSDT", "SHORT")
	if err != nil || closed == nil {
		t.Fatalf("get closed position: pos=%v err=%v", closed, err)
	}
	if closed.CloseReason != "hard_loss_close" {
		t.Fatalf("close reason = %q, want hard_loss_close", closed.CloseReason)
	}
	if closed.Source != "system_protector" {
		t.Fatalf("source = %q, want system_protector", closed.Source)
	}
}

func TestPositionBuilderIgnoresExpiredPendingProtectedCloseIntent(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "expired-pending-close-intent.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})

	nowMs := time.Now().UTC().UnixMilli()
	if err := st.Position().RecordPendingCloseIntent("trader-1", "LABUSDT", "SHORT", "hard_loss_close", "system_protector"); err != nil {
		t.Fatalf("record pending intent: %v", err)
	}
	expiredMs := nowMs - int64(30*time.Minute/time.Millisecond)
	if err := st.Position().db.Model(&PositionCloseIntent{}).Where("trader_id = ?", "trader-1").Updates(map[string]interface{}{
		"created_at": expiredMs,
		"updated_at": expiredMs,
	}).Error; err != nil {
		t.Fatalf("age pending intent: %v", err)
	}

	builder := NewPositionBuilder(st.Position())
	if err := builder.ProcessTradeWithLeverage("trader-1", "exchange-1", "binance", "LABUSDT", "SHORT", "open_short", 2, 7.198, 0.007, 0, nowMs, "open-1", 10); err != nil {
		t.Fatalf("process open: %v", err)
	}
	if err := builder.ProcessTradeWithLeverage("trader-1", "exchange-1", "binance", "LABUSDT", "SHORT", "close_short", 2, 7.295, 0.007, -0.194, nowMs+33000, "close-1", 10); err != nil {
		t.Fatalf("process close: %v", err)
	}

	closed, err := st.Position().GetLatestPositionBySymbol("trader-1", "LABUSDT", "SHORT")
	if err != nil || closed == nil {
		t.Fatalf("get closed position: pos=%v err=%v", closed, err)
	}
	if closed.CloseReason != "sync" {
		t.Fatalf("close reason = %q, want sync", closed.CloseReason)
	}
	if closed.Source != "sync" {
		t.Fatalf("source = %q, want sync", closed.Source)
	}
}

func TestPositionBuilderUsesExchangeCloseReasonWhenProvided(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "exchange-close-reason.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})

	nowMs := time.Now().UTC().UnixMilli()
	builder := NewPositionBuilder(st.Position())
	if err := builder.ProcessTradeWithLeverageAndCloseReason("trader-1", "exchange-1", "bybit", "SOLUSDT", "LONG", "open_long", 2, 100, 0.01, 0, nowMs, "open-1", 5, "sync"); err != nil {
		t.Fatalf("process open: %v", err)
	}
	if err := builder.ProcessTradeWithLeverageAndCloseReason("trader-1", "exchange-1", "bybit", "SOLUSDT", "LONG", "close_long", 2, 112, 0.01, 24, nowMs+60_000, "close-1", 5, "exchange_take_profit"); err != nil {
		t.Fatalf("process close: %v", err)
	}

	closed, err := st.Position().GetLatestPositionBySymbol("trader-1", "SOLUSDT", "LONG")
	if err != nil || closed == nil {
		t.Fatalf("get closed position: pos=%v err=%v", closed, err)
	}
	if closed.CloseReason != "exchange_take_profit" {
		t.Fatalf("close reason = %q, want exchange_take_profit", closed.CloseReason)
	}
	if closed.Source != "sync" {
		t.Fatalf("source = %q, want sync", closed.Source)
	}
}

func TestClosePositionWithAccurateDataPreservesProtectedCloseIntent(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "accurate-close-intent.db"))
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
		Symbol:       "TACUSDT",
		Side:         "SHORT",
		Quantity:     100,
		EntryPrice:   0.0342,
		EntryTime:    nowMs,
		Leverage:     20,
		Status:       "OPEN",
		Source:       "sync",
		CreatedAt:    nowMs,
		UpdatedAt:    nowMs,
	}); err != nil {
		t.Fatalf("create open position: %v", err)
	}
	pos, err := st.Position().GetOpenPositionBySymbol("trader-1", "TACUSDT", "SHORT")
	if err != nil || pos == nil {
		t.Fatalf("get open position: pos=%v err=%v", pos, err)
	}
	if err := st.Position().MarkPositionCloseIntent(pos.ID, "hard_loss_close", "system_protector"); err != nil {
		t.Fatalf("mark close intent: %v", err)
	}
	if err := st.Position().ClosePositionWithAccurateData(pos.ID, 0.03499, "close-1", nowMs+5000, -0.16, 0.01, "sync"); err != nil {
		t.Fatalf("accurate close: %v", err)
	}

	closed, err := st.Position().GetLatestPositionBySymbol("trader-1", "TACUSDT", "SHORT")
	if err != nil || closed == nil {
		t.Fatalf("get closed position: pos=%v err=%v", closed, err)
	}
	if closed.CloseReason != "hard_loss_close" {
		t.Fatalf("close reason = %q, want hard_loss_close", closed.CloseReason)
	}
	if closed.Source != "system_protector" {
		t.Fatalf("source = %q, want system_protector", closed.Source)
	}
}
