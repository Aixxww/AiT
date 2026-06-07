package trader

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/store"
)

type contextTestTrader struct {
	balance      map[string]interface{}
	balanceErr   error
	positions    []map[string]interface{}
	positionsErr error
}

func (t *contextTestTrader) GetBalance() (map[string]interface{}, error) {
	if t.balanceErr != nil {
		return nil, t.balanceErr
	}
	return t.balance, nil
}

func (t *contextTestTrader) GetPositions() ([]map[string]interface{}, error) {
	if t.positionsErr != nil {
		return nil, t.positionsErr
	}
	return t.positions, nil
}

func (t *contextTestTrader) OpenLong(string, float64, int) (map[string]interface{}, error) {
	return nil, nil
}
func (t *contextTestTrader) OpenShort(string, float64, int) (map[string]interface{}, error) {
	return nil, nil
}
func (t *contextTestTrader) CloseLong(string, float64) (map[string]interface{}, error) {
	return nil, nil
}
func (t *contextTestTrader) CloseShort(string, float64) (map[string]interface{}, error) {
	return nil, nil
}
func (t *contextTestTrader) SetLeverage(string, int) error { return nil }
func (t *contextTestTrader) SetMarginMode(string, bool) error {
	return nil
}
func (t *contextTestTrader) GetMarketPrice(string) (float64, error) { return 100, nil }
func (t *contextTestTrader) SetStopLoss(string, string, float64, float64) error {
	return nil
}
func (t *contextTestTrader) SetTakeProfit(string, string, float64, float64) error {
	return nil
}
func (t *contextTestTrader) CancelStopLossOrders(string) error   { return nil }
func (t *contextTestTrader) CancelTakeProfitOrders(string) error { return nil }
func (t *contextTestTrader) CancelAllOrders(string) error        { return nil }
func (t *contextTestTrader) CancelStopOrders(string) error       { return nil }
func (t *contextTestTrader) FormatQuantity(string, float64) (string, error) {
	return "1", nil
}
func (t *contextTestTrader) GetOrderStatus(string, string) (map[string]interface{}, error) {
	return nil, nil
}
func (t *contextTestTrader) GetClosedPnL(time.Time, int) ([]ClosedPnLRecord, error) {
	return nil, nil
}
func (t *contextTestTrader) GetOpenOrders(string) ([]OpenOrder, error) {
	return nil, nil
}

func newContextTestAutoTrader(ft *contextTestTrader) *AutoTrader {
	cfg := &store.StrategyConfig{
		CoinSource: store.CoinSourceConfig{
			SourceType:  "static",
			StaticCoins: []string{},
		},
		RiskControl: store.RiskControlConfig{
			BTCETHMaxLeverage:  10,
			AltcoinMaxLeverage: 5,
		},
	}
	return &AutoTrader{
		id:                    "context-test",
		name:                  "Context Test",
		config:                AutoTraderConfig{ScanInterval: time.Minute, StrategyConfig: cfg},
		trader:                ft,
		strategyEngine:        kernel.NewStrategyEngine(cfg),
		initialBalance:        100,
		startTime:             time.Now(),
		positionFirstSeenTime: make(map[string]int64),
		peakPnLCache:          make(map[string]float64),
	}
}

func TestBuildTradingContextUsesCachedBalanceWhenBalanceAPIFails(t *testing.T) {
	ft := &contextTestTrader{
		balance: map[string]interface{}{
			"totalWalletBalance":    100.0,
			"totalUnrealizedProfit": 2.0,
			"availableBalance":      75.0,
			"totalEquity":           102.0,
		},
		positions: []map[string]interface{}{},
	}
	at := newContextTestAutoTrader(ft)

	ctx, err := at.buildTradingContext()
	if err != nil {
		t.Fatalf("initial context failed: %v", err)
	}
	if ctx.IsDegraded {
		t.Fatalf("initial context should not be degraded")
	}

	ft.balance = nil
	ft.balanceErr = errors.New("unexpected EOF")
	ctx, err = at.buildTradingContext()
	if err != nil {
		t.Fatalf("degraded context should use cached balance, got error: %v", err)
	}
	if !ctx.IsDegraded || !ctx.AccountDataStale || ctx.PositionDataStale || !ctx.DisableOpenOrders {
		t.Fatalf("unexpected degradation flags: degraded=%v account_stale=%v position_stale=%v disable_open=%v",
			ctx.IsDegraded, ctx.AccountDataStale, ctx.PositionDataStale, ctx.DisableOpenOrders)
	}
	if ctx.Account.TotalEquity != 102 || ctx.Account.AvailableBalance != 75 {
		t.Fatalf("cached account not used correctly: %+v", ctx.Account)
	}
	if len(ctx.DegradationReasons) == 0 || !strings.Contains(ctx.DegradationReasons[0], "unexpected EOF") {
		t.Fatalf("missing balance degradation reason: %+v", ctx.DegradationReasons)
	}
}

func TestBuildTradingContextUsesCachedPositionsWhenPositionsAPIFails(t *testing.T) {
	ft := &contextTestTrader{
		balance: map[string]interface{}{
			"totalWalletBalance":    100.0,
			"totalUnrealizedProfit": 0.0,
			"availableBalance":      80.0,
			"totalEquity":           100.0,
		},
		positions: []map[string]interface{}{
			{
				"symbol":           "SOLUSDT",
				"side":             "LONG",
				"entryPrice":       100.0,
				"markPrice":        101.0,
				"positionAmt":      1.0,
				"unRealizedProfit": 1.0,
				"liquidationPrice": 50.0,
				"leverage":         5.0,
			},
		},
	}
	at := newContextTestAutoTrader(ft)

	if _, err := at.buildTradingContext(); err != nil {
		t.Fatalf("initial context failed: %v", err)
	}

	ft.positions = nil
	ft.positionsErr = errors.New("connection reset by peer")
	ctx, err := at.buildTradingContext()
	if err != nil {
		t.Fatalf("degraded context should use cached positions, got error: %v", err)
	}
	if !ctx.IsDegraded || ctx.AccountDataStale || !ctx.PositionDataStale || !ctx.DisableOpenOrders {
		t.Fatalf("unexpected degradation flags: degraded=%v account_stale=%v position_stale=%v disable_open=%v",
			ctx.IsDegraded, ctx.AccountDataStale, ctx.PositionDataStale, ctx.DisableOpenOrders)
	}
	if len(ctx.Positions) != 1 || ctx.Positions[0].Symbol != "SOLUSDT" {
		t.Fatalf("cached positions not used correctly: %+v", ctx.Positions)
	}
	if len(ctx.DegradationReasons) == 0 || !strings.Contains(ctx.DegradationReasons[0], "connection reset by peer") {
		t.Fatalf("missing positions degradation reason: %+v", ctx.DegradationReasons)
	}
}

func TestBuildTradingContextFailsWhenCachedBalanceIsTooOld(t *testing.T) {
	ft := &contextTestTrader{
		balanceErr: errors.New("timeout"),
		positions:  []map[string]interface{}{},
	}
	at := newContextTestAutoTrader(ft)
	at.cacheContextBalance(map[string]interface{}{
		"totalWalletBalance":    100.0,
		"totalUnrealizedProfit": 0.0,
		"availableBalance":      80.0,
		"totalEquity":           100.0,
	})
	at.contextCacheMutex.Lock()
	at.lastContextBalanceAt = time.Now().Add(-31 * time.Minute)
	at.contextCacheMutex.Unlock()

	_, err := at.buildTradingContext()
	if err == nil || !strings.Contains(err.Error(), "failed to get account balance") {
		t.Fatalf("expected stale cache to fail balance fetch, got: %v", err)
	}
}
