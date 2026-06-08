package binance

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Aixxww/AiT/store"
)

func TestResolveSyncedTradeLeverageUsesExchangeActivePosition(t *testing.T) {
	leverage := resolveSyncedTradeLeverage(
		nil,
		map[string]int{positionLeverageKey("MANTAUSDT", "SHORT"): 20},
		"trader-1",
		"MANTAUSDT",
		"SHORT",
		"close_short",
	)

	if leverage != 20 {
		t.Fatalf("expected active exchange leverage 20, got %d", leverage)
	}
}

func TestResolveSyncedTradeLeverageFallsBackToLocalOpenPosition(t *testing.T) {
	st := newSyncLeverageTestStore(t)
	nowMs := time.Now().UTC().UnixMilli()

	if err := st.Position().CreateOpenPosition(&store.TraderPosition{
		TraderID:           "trader-1",
		ExchangeID:         "exchange-1",
		ExchangeType:       "binance",
		ExchangePositionID: "sync_MANTAUSDT_SHORT",
		Symbol:             "MANTAUSDT",
		Side:               "SHORT",
		Quantity:           552.8,
		EntryPrice:         0.07741,
		EntryTime:          nowMs,
		Leverage:           20,
		Status:             "OPEN",
		Source:             "sync",
		CreatedAt:          nowMs,
		UpdatedAt:          nowMs,
	}); err != nil {
		t.Fatalf("create open position: %v", err)
	}

	leverage := resolveSyncedTradeLeverage(
		st.Position(),
		map[string]int{},
		"trader-1",
		"MANTAUSDT",
		"SHORT",
		"close_short",
	)

	if leverage != 20 {
		t.Fatalf("expected local open position leverage 20, got %d", leverage)
	}
}

func TestResolveSyncedTradeLeverageFallsBackToLatestClosedPosition(t *testing.T) {
	st := newSyncLeverageTestStore(t)
	nowMs := time.Now().UTC().UnixMilli()

	if err := st.Position().CreateOpenPosition(&store.TraderPosition{
		TraderID:           "trader-1",
		ExchangeID:         "exchange-1",
		ExchangeType:       "binance",
		ExchangePositionID: "sync_MANTAUSDT_SHORT",
		Symbol:             "MANTAUSDT",
		Side:               "SHORT",
		Quantity:           552.8,
		EntryPrice:         0.07741,
		EntryTime:          nowMs,
		Leverage:           20,
		Status:             "OPEN",
		Source:             "sync",
		CreatedAt:          nowMs,
		UpdatedAt:          nowMs,
	}); err != nil {
		t.Fatalf("create open position: %v", err)
	}

	pos, err := st.Position().GetOpenPositionBySymbol("trader-1", "MANTAUSDT", "SHORT")
	if err != nil {
		t.Fatalf("get open position: %v", err)
	}
	if pos == nil {
		t.Fatal("expected open position")
	}
	if err := st.Position().ClosePositionFully(pos.ID, 0.07727, "close-1", nowMs+1000, 0.10, 0.04, "sync"); err != nil {
		t.Fatalf("close position: %v", err)
	}

	leverage := resolveSyncedTradeLeverage(
		st.Position(),
		map[string]int{},
		"trader-1",
		"MANTAUSDT",
		"SHORT",
		"close_short",
	)

	if leverage != 20 {
		t.Fatalf("expected latest closed position leverage 20, got %d", leverage)
	}
}

func newSyncLeverageTestStore(t *testing.T) *store.Store {
	t.Helper()

	st, err := store.New(filepath.Join(t.TempDir(), "sync-leverage.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})
	return st
}
