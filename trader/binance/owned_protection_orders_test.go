package binance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// protectionBook: a hedge-mode book with protective and non-protective
// orders on both sides, owned and manual.
func protectionBook() *ownedMockExchange {
	own := ownedOrderPrefix
	return &ownedMockExchange{
		orders: map[int64][2]string{
			1: {own + "long-legacy-sl", "LONG"},   // owned LONG STOP_MARKET -> cancel
			2: {own + "long-limit", "LONG"},       // owned LONG LIMIT (not protective) -> keep
			3: {own + "short-legacy-sl", "SHORT"}, // owned SHORT stop -> keep (other side)
		},
		algos: map[int64][2]string{
			11: {own + "long-sl", "LONG"},   // owned LONG STOP_MARKET -> cancel
			12: {own + "long-tp", "LONG"},   // owned LONG TAKE_PROFIT_MARKET -> cancel unless stopsOnly
			13: {own + "short-sl", "SHORT"}, // owned SHORT stop -> keep (hedge protection)
			14: {"manual_long_sl", "LONG"},  // manual LONG stop -> keep
		},
		types: map[int64]string{
			1: "STOP_MARKET", 2: "LIMIT", 3: "STOP_MARKET",
			11: "STOP_MARKET", 12: "TAKE_PROFIT_MARKET", 13: "STOP_MARKET", 14: "STOP_MARKET",
		},
		ignoreCancel: map[int64]bool{},
	}
}

// F27: only owned protective orders on the managed side are cancelled.
func TestCancelOwnedProtectionOrders_SideScopedProtectiveOnly(t *testing.T) {
	m := protectionBook()
	ft := newOwnedMockTrader(t, m)

	require.NoError(t, ft.CancelOwnedProtectionOrders("BTCUSDT", "LONG", false))

	assert.ElementsMatch(t, []int64{1, 11, 12}, m.canceled)
	assert.Contains(t, m.orders, int64(2), "owned non-protective order kept")
	assert.Contains(t, m.orders, int64(3), "opposite-side legacy stop kept")
	assert.Contains(t, m.algos, int64(13), "opposite-side algo stop (hedge protection) kept")
	assert.Contains(t, m.algos, int64(14), "manual stop kept")
}

// F27: the dynamic stop update keeps the take-profit.
func TestCancelOwnedProtectionOrders_StopsOnlyKeepsTakeProfit(t *testing.T) {
	m := protectionBook()
	ft := newOwnedMockTrader(t, m)

	require.NoError(t, ft.CancelOwnedProtectionOrders("BTCUSDT", "LONG", true))

	assert.ElementsMatch(t, []int64{1, 11}, m.canceled)
	assert.Contains(t, m.algos, int64(12), "take-profit kept in stops-only mode")
}

// F27: a surviving order makes the cancel unconfirmed.
func TestCancelOwnedProtectionOrders_SurvivorIsError(t *testing.T) {
	m := protectionBook()
	m.ignoreCancel[11] = true
	ft := newOwnedMockTrader(t, m)

	err := ft.CancelOwnedProtectionOrders("BTCUSDT", "LONG", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not confirmed")
}

// F28: any stop on the side (owned or manual) counts as protection; a
// take-profit or the other side's stop does not.
func TestProtectiveStopOpen(t *testing.T) {
	m := protectionBook()
	ft := newOwnedMockTrader(t, m)
	open, err := ft.ProtectiveStopOpen("BTCUSDT", "LONG")
	require.NoError(t, err)
	assert.True(t, open)

	// Only the LONG take-profit and the SHORT stops remain.
	delete(m.orders, 1)
	delete(m.algos, 11)
	delete(m.algos, 14)
	open, err = ft.ProtectiveStopOpen("BTCUSDT", "LONG")
	require.NoError(t, err)
	assert.False(t, open, "take-profit and opposite-side stops are not LONG protection")

	// A manual stop alone protects.
	m.algos[14] = [2]string{"manual_long_sl", "LONG"}
	open, err = ft.ProtectiveStopOpen("BTCUSDT", "LONG")
	require.NoError(t, err)
	assert.True(t, open)

	m.failListAlgos = true
	_, err = ft.ProtectiveStopOpen("BTCUSDT", "LONG")
	require.Error(t, err, "list failure means the protection state is unknown")
}
