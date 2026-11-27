package trader

import (
	"os"
	"strings"
	"testing"
)

// #159: max_margin_usage is a real code-enforced gate. The gate reads the
// balance map the open path already fetched; equity is account-level
// (cross margin), used margin is the account's initial margin in use.

// Over the limit: used 800 of 1000 equity, a 600 USD open at 5x adds 120
// initial margin -> projected 92% > 90% limit.
func TestEnforceMaxMarginUsageRejectsOverLimit(t *testing.T) {
	at := testRiskAutoTrader() // MaxMarginUsage 0.9
	balance := map[string]interface{}{
		"totalEquity":        1000.0,
		"totalInitialMargin": 800.0,
		"availableBalance":   200.0,
	}
	err := at.enforceMaxMarginUsage(balance, 600, 5)
	if err == nil || !strings.Contains(err.Error(), "margin_usage_exceeded") {
		t.Fatalf("expected margin_usage_exceeded rejection, got: %v", err)
	}
}

// Under the limit passes, and exactly at the limit passes (only exceeding
// rejects): used 800, +80 initial margin = 88%; +100 = exactly 90%.
func TestEnforceMaxMarginUsageAllowsUnderLimitAndBoundary(t *testing.T) {
	at := testRiskAutoTrader()
	balance := map[string]interface{}{
		"totalEquity":        1000.0,
		"totalInitialMargin": 800.0,
		"availableBalance":   200.0,
	}
	if err := at.enforceMaxMarginUsage(balance, 400, 5); err != nil {
		t.Fatalf("88%% projected usage must pass, got: %v", err)
	}
	if err := at.enforceMaxMarginUsage(balance, 500, 5); err != nil {
		t.Fatalf("exactly 90%% projected usage must pass, got: %v", err)
	}
}

// Without an explicit totalInitialMargin key the used margin is derived
// as equity - availableBalance (equity = wallet + unrealized PnL).
func TestEnforceMaxMarginUsageDerivesUsedMarginFromAvailable(t *testing.T) {
	at := testRiskAutoTrader()
	balance := map[string]interface{}{
		"totalWalletBalance":    900.0,
		"totalUnrealizedProfit": 100.0,
		"availableBalance":      200.0,
	}
	// equity 1000, used 800: same numbers as the explicit-key case.
	if err := at.enforceMaxMarginUsage(balance, 400, 5); err != nil {
		t.Fatalf("88%% projected usage must pass, got: %v", err)
	}
	err := at.enforceMaxMarginUsage(balance, 600, 5)
	if err == nil || !strings.Contains(err.Error(), "margin_usage_exceeded") {
		t.Fatalf("expected margin_usage_exceeded rejection, got: %v", err)
	}
}

// Adapter-shaped maps after the agy adversarial review fix-ups: Bitget
// exposes totalEquity under the canonical key (its legacy snake_case key
// left the gate on the fallback path), and Gate's futures "total" already
// includes unrealised PnL, so equity must be read from totalEquity
// directly — deriving it as totalWalletBalance + uPnL would double-count.
func TestEnforceMaxMarginUsageAdapterShapedBalances(t *testing.T) {
	at := testRiskAutoTrader()

	// Bitget shape: equity 1000 (wallet 900 + uPnL 100), available 200.
	bitget := map[string]interface{}{
		"totalEquity":           1000.0,
		"totalWalletBalance":    900.0,
		"totalUnrealizedProfit": 100.0,
		"availableBalance":      200.0,
	}
	if err := at.enforceMaxMarginUsage(bitget, 400, 5); err != nil {
		t.Fatalf("bitget shape: 88%% projected usage must pass, got: %v", err)
	}

	// Gate shape: total 1000 already includes uPnL +100, available 600.
	// Correct: equity 1000, used 400; +500 new margin = 90% boundary pass.
	// Double-counted equity (1100) would make used 500 and reject this.
	gateBal := map[string]interface{}{
		"totalEquity":           1000.0,
		"totalWalletBalance":    1000.0,
		"totalUnrealizedProfit": 100.0,
		"availableBalance":      600.0,
	}
	if err := at.enforceMaxMarginUsage(gateBal, 2500, 5); err != nil {
		t.Fatalf("gate shape: 90%% boundary must pass (equity must not double-count uPnL), got: %v", err)
	}
	err := at.enforceMaxMarginUsage(gateBal, 2600, 5)
	if err == nil || !strings.Contains(err.Error(), "margin_usage_exceeded") {
		t.Fatalf("gate shape: expected margin_usage_exceeded rejection, got: %v", err)
	}
}

// Fail closed: unusable figures reject with the distinct
// margin_usage_data_unavailable code (never margin_usage_exceeded, never
// a silent pass).
func TestEnforceMaxMarginUsageFailsClosedWithoutFigures(t *testing.T) {
	at := testRiskAutoTrader()
	cases := []struct {
		name    string
		balance map[string]interface{}
		size    float64
		lev     int
	}{
		{"nil balance", nil, 100, 5},
		{"empty balance", map[string]interface{}{}, 100, 5},
		{"no margin or available figure", map[string]interface{}{"totalEquity": 1000.0}, 100, 5},
		{"zero equity", map[string]interface{}{"totalWalletBalance": 0.0, "availableBalance": 0.0}, 100, 5},
		{"available above equity", map[string]interface{}{"totalEquity": 1000.0, "availableBalance": 1200.0}, 100, 5},
		{"zero leverage", map[string]interface{}{"totalEquity": 1000.0, "totalInitialMargin": 100.0}, 100, 0},
	}
	for _, tc := range cases {
		err := at.enforceMaxMarginUsage(tc.balance, tc.size, tc.lev)
		if err == nil || !strings.Contains(err.Error(), "margin_usage_data_unavailable") {
			t.Fatalf("%s: expected margin_usage_data_unavailable rejection, got: %v", tc.name, err)
		}
		if strings.Contains(err.Error(), "margin_usage_exceeded") {
			t.Fatalf("%s: fail-closed code must be distinct from the over-limit code: %v", tc.name, err)
		}
	}
}

// A configured 0 (or negative) disables the gate: even a wildly
// over-limit open passes, and missing figures no longer block.
func TestEnforceMaxMarginUsageZeroDisablesGate(t *testing.T) {
	for _, limit := range []float64{0, -0.5} {
		at := testRiskAutoTrader()
		at.config.StrategyConfig.RiskControl.MaxMarginUsage = limit
		balance := map[string]interface{}{
			"totalEquity":        1000.0,
			"totalInitialMargin": 990.0,
			"availableBalance":   10.0,
		}
		if err := at.enforceMaxMarginUsage(balance, 100000, 1); err != nil {
			t.Fatalf("limit %v: gate disabled, over-limit open must pass, got: %v", limit, err)
		}
		if err := at.enforceMaxMarginUsage(nil, 100, 5); err != nil {
			t.Fatalf("limit %v: gate disabled, missing figures must not block, got: %v", limit, err)
		}
	}
}

// No strategy config: the gate is a no-op, like the other enforce*
// helpers.
func TestEnforceMaxMarginUsageWithoutStrategyConfig(t *testing.T) {
	at := &AutoTrader{}
	if err := at.enforceMaxMarginUsage(nil, 100, 5); err != nil {
		t.Fatalf("no strategy config must not block, got: %v", err)
	}
}

// Scope guard (#159): the gate is wired into both open executors — after
// the post-preflight risk recheck, on the final position size — and into
// nothing else. Close, stop-loss and take-profit paths never consult it.
// (The executors read live market data, so as with #129 they are checked
// at the source level.)
func TestMaxMarginUsageGateWiringOpensOnly(t *testing.T) {
	src, err := os.ReadFile("auto_trader_orders.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	fnBody := func(fn string) string {
		i := strings.Index(body, "func (at *AutoTrader) "+fn+"(")
		if i < 0 {
			t.Fatalf("%s not found", fn)
		}
		b := body[i:]
		if j := strings.Index(b[1:], "\nfunc "); j >= 0 {
			b = b[:j+1]
		}
		return b
	}
	for _, fn := range []string{"executeOpenLongWithRecord", "executeOpenShortWithRecord"} {
		b := fnBody(fn)
		recheck := strings.Index(b, "recheckRiskAfterPreflight(")
		gate := strings.Index(b, "enforceMaxMarginUsage(")
		qty := strings.Index(b, "quantity := actualPositionSize / executionPrice")
		if recheck < 0 || gate < 0 || qty < 0 || !(recheck < gate && gate < qty) {
			t.Fatalf("%s: recheck@%d gate@%d quantity@%d: the margin gate must sit between the recheck and the quantity", fn, recheck, gate, qty)
		}
	}
	for _, fn := range []string{"executeCloseLongWithRecord", "executeCloseShortWithRecord"} {
		if strings.Contains(fnBody(fn), "enforceMaxMarginUsage(") {
			t.Fatalf("%s must never consult the margin usage gate", fn)
		}
	}
	if n := strings.Count(body, "enforceMaxMarginUsage("); n != 2 {
		t.Fatalf("expected exactly 2 gate call sites in auto_trader_orders.go, found %d", n)
	}
	// No other non-test trader source may call the gate (protection,
	// grid and lifecycle paths place SL/TP and closes).
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") ||
			name == "auto_trader_orders.go" || name == "auto_trader_risk.go" {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "enforceMaxMarginUsage(") {
			t.Fatalf("%s must not call the margin usage gate", name)
		}
	}
}
