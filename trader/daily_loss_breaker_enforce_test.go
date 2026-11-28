package trader

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/store"
)

// M1 / Unit U3 integration tests: drive the real open and close paths
// (executeDecisionWithRecord, executeOpenLongWithRecord,
// executeOpenShortWithRecord, registerHunterV7ConditionalEntry,
// checkHunterV7ConditionalEntries -> executeHunterV7PendingEntry) with the
// breaker latched tripped, verifying that mode=armed blocks every open before
// any order reaches the exchange while mode=shadow proceeds and closes/holds
// are never blocked.

type dailyLossRoundTripFunc func(*http.Request) (*http.Response, error)

func (f dailyLossRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// stubCoinAnkMarketTransport intercepts market.GetWithExchange's CoinAnk kline
// HTTP calls in-memory so executeOpen*WithRecord / executeClose*WithRecord can
// run end-to-end without network access. OI and funding requests return 404
// immediately (market.GetWithExchange treats both as optional).
func stubCoinAnkMarketTransport(t *testing.T, finalPrice float64) {
	t.Helper()
	// 30 ascending bars with non-zero volume ending at finalPrice so
	// isStaleData returns false and EMA/MACD/RSI/1h change compute cleanly.
	bars := make([][]float64, 30)
	baseMs := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC).UnixMilli()
	for i := range bars {
		c := finalPrice - float64(len(bars)-1-i)*0.02
		o := c - 0.01
		h := c + 0.05
		l := o - 0.05
		start := float64(baseMs + int64(i)*180_000)
		end := float64(baseMs + int64(i+1)*180_000 - 1)
		bars[i] = []float64{start, end, o, c, h, l, 1000.0 + float64(i), 100.0, 50.0}
	}
	payload, err := json.Marshal(map[string]interface{}{
		"success": true,
		"data":    bars,
	})
	if err != nil {
		t.Fatalf("marshal stub klines: %v", err)
	}

	orig := http.DefaultTransport
	http.DefaultTransport = dailyLossRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "api.coinank.com" && req.URL.Path == "/api/kline/list/open" {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader(payload)),
				Request:    req,
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = orig })
}

func newDailyLossEnforceTrader(t *testing.T, mode string, clock *time.Time, st *store.Store) (*AutoTrader, *lifecycleFake) {
	t.Helper()
	ft := &lifecycleFake{
		bookPrice:  100,
		fillPrice:  100,
		reportFill: true,
	}
	ft.balance = map[string]interface{}{
		"availableBalance":   800.0,
		"totalEquity":        1000.0,
		"totalWalletBalance": 1000.0,
	}
	at := newLifecycleTrader(t, ft, st)
	at.id = "u3-breaker-trader"
	at.isRunning = true
	at.config.StrategyConfig.CoinSource.SourceType = "hunter_v7"
	at.config.StrategyConfig.CoinSource.Hunter = &store.HunterConfig{
		V7ConditionalEntry: &store.V7ConditionalEntryConfig{Enabled: true},
	}
	at.config.StrategyConfig.RiskControl.MaxPositions = 3
	at.config.StrategyConfig.RiskControl.MinPositionSize = 12
	at.config.StrategyConfig.RiskControl.MinRiskRewardRatio = 1.2
	at.config.StrategyConfig.RiskControl.MinStopLossPriceMovePct = 2.0
	at.config.StrategyConfig.RiskControl.MaxTakeProfitPriceMovePct = 3.5
	at.config.StrategyConfig.RiskControl.MaxEntryPriceDeviationPct = 0.5
	at.config.StrategyConfig.RiskControl.MaxSingleTradeLossPct = 6.0
	at.config.StrategyConfig.RiskControl.MaxMarginUsage = 0.7
	at.config.StrategyConfig.RiskControl.AltcoinMaxPositionValueRatio = 10
	at.config.StrategyConfig.RiskControl.BTCETHMaxPositionValueRatio = 10
	at.config.StrategyConfig.RiskControl.DailyLossBreaker = &store.DailyLossBreakerConfig{Mode: mode}
	at.dailyLoss.now = func() time.Time { return *clock }
	return at, ft
}

func tripBreaker(t *testing.T, at *AutoTrader) dailyLossEvaluation {
	t.Helper()
	at.evaluateDailyLoss(1000, false)
	ev := at.evaluateDailyLoss(840, false) // 16% loss >= 15% limit
	if ev.Status != dailyLossStatusTripped {
		t.Fatalf("expected breaker to trip at 16%% loss, got %+v", ev)
	}
	return ev
}

func enforceOpenDecision(side string) *kernel.Decision {
	if side == "short" {
		return &kernel.Decision{
			Symbol:                   "SOLUSDT",
			Action:                   "open_short",
			Leverage:                 5,
			PositionSizeUSD:          100,
			Price:                    100,
			StopLoss:                 102.2,
			TakeProfit:               96.6,
			Confidence:               80,
			SelectedHunterV7SignalID: "sig-short-1",
		}
	}
	return &kernel.Decision{
		Symbol:                   "SOLUSDT",
		Action:                   "open_long",
		Leverage:                 5,
		PositionSizeUSD:          100,
		Price:                    100,
		StopLoss:                 97.8,
		TakeProfit:               103.4,
		Confidence:               80,
		SelectedHunterV7SignalID: "sig-long-1",
	}
}

func enforceConditionalDecision(side string) (*kernel.Context, *kernel.Decision) {
	if side == "short" {
		d := &kernel.Decision{
			Symbol:                    "IGNUSDT",
			Action:                    "open_short",
			Leverage:                  5,
			PositionSizeUSD:           100,
			Price:                     100,
			StopLoss:                  102.2,
			TakeProfit:                96.6,
			Confidence:                80,
			SelectedHunterV7SignalID:  "ign-short-1",
			HunterV7MinRR:             1.2,
			HunterV7MinStopPct:        2.0,
			HunterV7TriggerPrice:      100,
			HunterV7ConditionalEntry:  true,
			HunterV7InvalidationPrice: 102.2,
		}
		ctx := &kernel.Context{CandidateCoins: []kernel.CandidateCoin{{
			Symbol:             "IGNUSDT",
			Direction:          "SHORT",
			V7SignalID:         "ign-short-1",
			V7SetupType:        "ignition_early_short",
			V7ExecutionTier:    "EXECUTABLE",
			V7ConditionalEntry: true,
			V7TriggerPrice:     100,
		}}}
		return ctx, d
	}
	d := &kernel.Decision{
		Symbol:                    "IGNUSDT",
		Action:                    "open_long",
		Leverage:                  5,
		PositionSizeUSD:           100,
		Price:                     100,
		StopLoss:                  97.8,
		TakeProfit:                103.4,
		Confidence:                80,
		SelectedHunterV7SignalID:  "ign-long-1",
		HunterV7MinRR:             1.2,
		HunterV7MinStopPct:        2.0,
		HunterV7TriggerPrice:      100,
		HunterV7ConditionalEntry:  true,
		HunterV7InvalidationPrice: 97.8,
	}
	ctx := &kernel.Context{CandidateCoins: []kernel.CandidateCoin{{
		Symbol:             "IGNUSDT",
		Direction:          "LONG",
		V7SignalID:         "ign-long-1",
		V7SetupType:        "ignition_early_long",
		V7ExecutionTier:    "EXECUTABLE",
		V7ConditionalEntry: true,
		V7TriggerPrice:     100,
	}}}
	return ctx, d
}

// End-to-end proof for cycle direct opens (auto_trader_orders.go:84 -> :162 / :319 ->
// placeOpenOrderWithLifecycle -> openMarketPosition -> exchange):
// armed rejects before any exchange call; shadow executes and places the order.
func TestDailyLossBreakerEnforce_CycleOpensArmedVsShadow(t *testing.T) {
	stubCoinAnkMarketTransport(t, 100.0)
	clock := time.Date(2026, 11, 20, 2, 0, 0, 0, time.UTC)

	for _, side := range []string{"long", "short"} {
		t.Run("armed_blocks_"+side, func(t *testing.T) {
			at, ft := newDailyLossEnforceTrader(t, store.DailyLossBreakerArmed, &clock, nil)
			ev := tripBreaker(t, at)
			if !ev.Blocks {
				t.Fatalf("armed trip must set Blocks=true: %+v", ev)
			}

			rec := &store.DecisionAction{}
			err := at.executeDecisionWithRecord(&kernel.Context{}, enforceOpenDecision(side), rec)
			if err == nil || !strings.Contains(err.Error(), "daily_loss_breaker") || !strings.Contains(err.Error(), "[armed]") {
				t.Fatalf("armed %s open: got err=%v, want daily_loss_breaker armed error", side, err)
			}
			if ft.opens != 0 || ft.positionsRd != 0 || len(ft.live) != 0 || len(ft.stopLossCalls) != 0 {
				t.Fatalf("armed %s open must not touch exchange: opens=%d posReads=%d live=%d stops=%d",
					side, ft.opens, ft.positionsRd, len(ft.live), len(ft.stopLossCalls))
			}
		})

		t.Run("shadow_proceeds_"+side, func(t *testing.T) {
			at, ft := newDailyLossEnforceTrader(t, store.DailyLossBreakerShadow, &clock, nil)
			ev := tripBreaker(t, at)
			if ev.Blocks {
				t.Fatalf("shadow trip must keep Blocks=false: %+v", ev)
			}

			rec := &store.DecisionAction{}
			err := at.executeDecisionWithRecord(&kernel.Context{}, enforceOpenDecision(side), rec)
			if err != nil {
				t.Fatalf("shadow %s open should proceed to exchange, got err=%v", side, err)
			}
			if ft.opens != 1 || len(ft.live) != 1 || ft.live[0]["side"] != side || len(ft.stopLossCalls) != 1 {
				t.Fatalf("shadow %s open must reach exchange and place stop: opens=%d live=%+v stops=%d",
					side, ft.opens, ft.live, len(ft.stopLossCalls))
			}
		})
	}
}

// End-to-end proof for conditional-entry registration
// (auto_trader_orders.go:84 -> registerHunterV7ConditionalEntry):
// armed rejects before registration and touches nothing; shadow registers the pending entry.
func TestDailyLossBreakerEnforce_ConditionalRegistrationArmedVsShadow(t *testing.T) {
	clock := time.Date(2026, 11, 20, 2, 0, 0, 0, time.UTC)

	for _, side := range []string{"long", "short"} {
		t.Run("armed_blocks_registration_"+side, func(t *testing.T) {
			at, ft := newDailyLossEnforceTrader(t, store.DailyLossBreakerArmed, &clock, nil)
			tripBreaker(t, at)

			ctx, d := enforceConditionalDecision(side)
			rec := &store.DecisionAction{}
			err := at.executeDecisionWithRecord(ctx, d, rec)
			if err == nil || !strings.Contains(err.Error(), "daily_loss_breaker") {
				t.Fatalf("armed conditional registration (%s): got %v, want daily_loss_breaker", side, err)
			}
			if rec.ConditionalPending || len(at.pendingConditionalEntries()) != 0 || ft.opens != 0 || ft.positionsRd != 0 {
				t.Fatalf("armed must not register pending entry or call exchange: rec=%+v pending=%d opens=%d posReads=%d",
					rec, len(at.pendingConditionalEntries()), ft.opens, ft.positionsRd)
			}
		})

		t.Run("shadow_registers_"+side, func(t *testing.T) {
			at, ft := newDailyLossEnforceTrader(t, store.DailyLossBreakerShadow, &clock, nil)
			tripBreaker(t, at)

			ctx, d := enforceConditionalDecision(side)
			rec := &store.DecisionAction{}
			err := at.executeDecisionWithRecord(ctx, d, rec)
			if err != nil {
				t.Fatalf("shadow conditional registration (%s) must succeed, got %v", side, err)
			}
			if !rec.ConditionalPending || len(at.pendingConditionalEntries()) != 1 || ft.opens != 0 {
				t.Fatalf("shadow must queue 1 pending entry without immediate market open: rec=%+v pending=%d opens=%d",
					rec, len(at.pendingConditionalEntries()), ft.opens)
			}
		})
	}
}

// End-to-end proof for conditional-entry fills
// (checkHunterV7ConditionalEntries -> executeHunterV7PendingEntry with real
// executeOpenLongWithRecord / executeOpenShortWithRecord, auto_trader_orders.go:162 & :319):
// an entry registered BEFORE the breaker trips is still blocked at trigger time
// in armed mode (no order reaches the exchange, pending row cancelled), while
// in shadow mode the trigger fires and places the exchange order.
func TestDailyLossBreakerEnforce_ConditionalFillArmedVsShadow(t *testing.T) {
	stubCoinAnkMarketTransport(t, 100.0)
	st := newLifecycleStore(t)

	for _, side := range []string{"long", "short"} {
		for _, mode := range []string{store.DailyLossBreakerArmed, store.DailyLossBreakerShadow} {
			name := fmt.Sprintf("%s_%s", mode, side)
			t.Run(name, func(t *testing.T) {
				t0 := time.Date(2026, 11, 20, 2, 0, 30, 0, time.UTC)
				clock := t0
				at, ft := newDailyLossEnforceTrader(t, mode, &clock, st)
				at.id = "u3-cond-" + name
				// Leave at.conditional.execute == nil so executeHunterV7PendingEntry
				// invokes the real executeOpenLongWithRecord / executeOpenShortWithRecord.

				// Step 1: register the pending entry while equity is healthy.
				at.evaluateDailyLoss(1000, false)
				ctx, d := enforceConditionalDecision(side)
				regRec := &store.DecisionAction{}
				if err := at.registerHunterV7ConditionalEntry(ctx, d, side, regRec, t0); err != nil {
					t.Fatalf("setup registration failed: %v", err)
				}
				if len(at.pendingConditionalEntries()) != 1 {
					t.Fatalf("expected 1 pending entry, got %d", len(at.pendingConditionalEntries()))
				}

				// Step 2: later in the same UTC day, equity drops >= 15% and latches tripped.
				clock = t0.Add(time.Minute)
				if ev := at.evaluateDailyLoss(840, false); ev.Status != dailyLossStatusTripped {
					t.Fatalf("expected tripped evaluation, got %+v", ev)
				}

				// Step 3: a completed 1m candle closes through the trigger price (100).
				m := t0.Truncate(time.Minute)
				var trigBar datafetch.Kline
				if side == "long" {
					trigBar = bar(m, 100.4, 99.7, 100.2) // closes >= 100, above invalidation 97.8
				} else {
					trigBar = bar(m, 100.3, 99.6, 99.8) // closes <= 100, below invalidation 102.2
				}
				at.conditional.bars = func(string) []datafetch.Kline { return []datafetch.Kline{trigBar} }
				at.conditional.latest = func(string) *datafetch.SymbolSnapshot {
					return &datafetch.SymbolSnapshot{Symbol: "IGNUSDT", Price: 100.0, Timestamp: time.Now()}
				}
				posReadsBeforeFire := ft.positionsRd

				at.checkHunterV7ConditionalEntries(m.Add(time.Minute + 5*time.Second))

				if len(at.pendingConditionalEntries()) != 0 {
					t.Fatalf("triggered entry must be removed from pending queue")
				}
				recs, err := st.Decision().GetLatestRecords(at.id, 5)
				if err != nil || len(recs) == 0 {
					t.Fatalf("expected conditional decision record, err=%v len=%d", err, len(recs))
				}
				latest := recs[0]

				if mode == store.DailyLossBreakerArmed {
					if ft.opens != 0 || len(ft.live) != 0 || len(ft.stopLossCalls) != 0 || ft.positionsRd != posReadsBeforeFire {
						t.Fatalf("armed conditional fill must not reach exchange: opens=%d live=%d stops=%d posReads=%d->%d",
							ft.opens, len(ft.live), len(ft.stopLossCalls), posReadsBeforeFire, ft.positionsRd)
					}
					if latest.Success || len(latest.Decisions) != 1 || !strings.Contains(latest.Decisions[0].Error, "daily_loss_breaker") {
						t.Fatalf("armed conditional fill record must record daily_loss_breaker error: %+v", latest)
					}
					rows, err := st.Decision().ListLifecycles(at.id, store.LifecycleStatusEntryCancelled)
					if err != nil || len(rows) != 1 || !strings.Contains(rows[0].Note, "daily_loss_breaker") {
						t.Fatalf("armed conditional fill must cancel lifecycle row with breaker reason: err=%v rows=%+v", err, rows)
					}
				} else {
					if ft.opens != 1 || len(ft.live) != 1 || ft.live[0]["side"] != side || len(ft.stopLossCalls) != 1 {
						t.Fatalf("shadow conditional fill must place order and stop: opens=%d live=%+v stops=%d",
							ft.opens, ft.live, len(ft.stopLossCalls))
					}
					if !latest.Success || len(latest.Decisions) != 1 || !latest.Decisions[0].Success {
						t.Fatalf("shadow conditional fill record must succeed: %+v", latest)
					}
					rows, err := st.Decision().ListLifecycles(at.id, store.LifecycleStatusEntryFired)
					if err != nil || len(rows) != 1 {
						t.Fatalf("shadow conditional fill must mark pending row fired: err=%v rows=%+v", err, rows)
					}
				}
			})
		}
	}
}

// End-to-end proof that in armed + tripped mode, close_long, close_short, hold,
// and wait through executeDecisionWithRecord are NEVER blocked and closes
// actually execute against the exchange.
func TestDailyLossBreakerEnforce_ArmedAllowsCloseAndHold(t *testing.T) {
	stubCoinAnkMarketTransport(t, 100.0)
	clock := time.Date(2026, 11, 20, 3, 0, 0, 0, time.UTC)

	at, ft := newDailyLossEnforceTrader(t, store.DailyLossBreakerArmed, &clock, nil)
	ft.live = []map[string]interface{}{
		{"symbol": "SOLUSDT", "side": "long", "positionAmt": 1.0, "entryPrice": 100.0, "markPrice": 100.0, "leverage": 5.0},
		{"symbol": "ETHUSDT", "side": "short", "positionAmt": -2.0, "entryPrice": 100.0, "markPrice": 100.0, "leverage": 5.0},
	}
	if ev := tripBreaker(t, at); !ev.Blocks {
		t.Fatalf("expected armed breaker to block opens: %+v", ev)
	}

	if err := at.executeDecisionWithRecord(&kernel.Context{}, &kernel.Decision{Symbol: "SOLUSDT", Action: "close_long"}, &store.DecisionAction{}); err != nil {
		t.Fatalf("armed breaker must not block close_long: %v", err)
	}
	if ft.closes != 1 || len(ft.live) != 1 || ft.live[0]["symbol"] != "ETHUSDT" {
		t.Fatalf("close_long must reach exchange and close SOLUSDT: closes=%d live=%+v", ft.closes, ft.live)
	}

	if err := at.executeDecisionWithRecord(&kernel.Context{}, &kernel.Decision{Symbol: "ETHUSDT", Action: "close_short"}, &store.DecisionAction{}); err != nil {
		t.Fatalf("armed breaker must not block close_short: %v", err)
	}
	if ft.closes != 2 || len(ft.live) != 0 {
		t.Fatalf("close_short must reach exchange and close ETHUSDT: closes=%d live=%+v", ft.closes, ft.live)
	}

	for _, action := range []string{"hold", "wait"} {
		if err := at.executeDecisionWithRecord(&kernel.Context{}, &kernel.Decision{Symbol: "SOLUSDT", Action: action}, &store.DecisionAction{}); err != nil {
			t.Fatalf("armed breaker must not block %s: %v", action, err)
		}
	}
}

// Live-config pinning (AIK11 strategy.json, 2026-11-28 postmortem M1):
// verifies the exact live risk_control.daily_loss_breaker block, the exact
// relaunch change ("shadow" -> "armed"), and that "enforce" is invalid and
// degrades to shadow rather than armed.
func TestDailyLossBreakerEnforce_LiveStrategyConfigPin(t *testing.T) {
	// Exact risk_control block from ~/workspace/ait-live-postmortem-20261128/strategy.json.
	const liveStrategyConfigJSON = `{
		"coin_source": {"source_type": "hunter_v7"},
		"risk_control": {
			"max_positions": 2,
			"btc_eth_max_leverage": 20,
			"altcoin_max_leverage": 20,
			"btc_eth_max_position_value_ratio": 10,
			"altcoin_max_position_value_ratio": 10,
			"max_margin_usage": 0.7,
			"min_position_size": 12,
			"min_risk_reward_ratio": 1.2,
			"min_confidence": 70,
			"max_entry_price_deviation_pct": 0.5,
			"max_single_trade_loss_pct": 6,
			"max_take_profit_price_move_pct": 3.5,
			"min_stop_loss_price_move_pct": 2,
			"setup_guard": {"funding_reversal": {"max_zone_pos_long": 35}},
			"daily_loss_breaker": {"mode": "shadow"}
		}
	}`

	var cfg store.StrategyConfig
	if err := json.Unmarshal([]byte(liveStrategyConfigJSON), &cfg); err != nil {
		t.Fatalf("unmarshal live strategy config: %v", err)
	}
	if err := store.ValidateTradingConfig(&cfg); err != nil {
		t.Fatalf("live strategy config should validate: %v", err)
	}
	eff := store.EffectiveDailyLossBreaker(cfg.RiskControl.DailyLossBreaker)
	if eff.Mode != store.DailyLossBreakerShadow || eff.MaxDailyLossPct != 15.0 || eff.NearTripFraction != 0.8 {
		t.Fatalf("live resolved breaker = %+v, want mode=shadow max_daily_loss_pct=15.0 near_trip_fraction=0.8", eff)
	}
	resolved := store.ResolveHunterV7(&cfg)
	if resolved.DailyLossBreaker != eff {
		t.Fatalf("ResolveHunterV7 breaker = %+v, want %+v", resolved.DailyLossBreaker, eff)
	}

	// Confirm the live config does NOT block when tripped.
	clock := time.Date(2026, 11, 28, 6, 0, 0, 0, time.UTC)
	atLive := &AutoTrader{
		id:     "aik11-live-pin",
		config: AutoTraderConfig{StrategyConfig: &cfg},
	}
	atLive.dailyLoss.now = func() time.Time { return clock }
	atLive.evaluateDailyLoss(121.78, false)
	evLive := atLive.evaluateDailyLoss(103.50, false) // 15.01% loss
	if evLive.Status != dailyLossStatusTripped || evLive.Blocks {
		t.Fatalf("live shadow config: ev=%+v, want tripped with Blocks=false", evLive)
	}
	if err := atLive.enforceDailyLossBreaker("SOLUSDT", "open_long"); err != nil {
		t.Fatalf("live shadow config must not block: %v", err)
	}

	// Exact relaunch change: risk_control.daily_loss_breaker.mode = "armed".
	relaunchCfg := cfg
	relaunchCfg.RiskControl.DailyLossBreaker = &store.DailyLossBreakerConfig{Mode: store.DailyLossBreakerArmed}
	if err := store.ValidateTradingConfig(&relaunchCfg); err != nil {
		t.Fatalf("relaunch armed config should validate: %v", err)
	}
	effRelaunch := store.EffectiveDailyLossBreaker(relaunchCfg.RiskControl.DailyLossBreaker)
	if effRelaunch.Mode != store.DailyLossBreakerArmed || effRelaunch.MaxDailyLossPct != 15.0 || effRelaunch.NearTripFraction != 0.8 {
		t.Fatalf("relaunch resolved breaker = %+v, want mode=armed max_daily_loss_pct=15.0 near_trip_fraction=0.8", effRelaunch)
	}
	atRelaunch := &AutoTrader{
		id:     "aik11-relaunch-pin",
		config: AutoTraderConfig{StrategyConfig: &relaunchCfg},
	}
	atRelaunch.dailyLoss.now = func() time.Time { return clock }
	atRelaunch.evaluateDailyLoss(121.78, false)
	evRelaunch := atRelaunch.evaluateDailyLoss(103.50, false) // 15.01% loss
	if evRelaunch.Status != dailyLossStatusTripped || !evRelaunch.Blocks {
		t.Fatalf("relaunch armed config: ev=%+v, want tripped with Blocks=true", evRelaunch)
	}
	if err := atRelaunch.enforceDailyLossBreaker("SOLUSDT", "open_long"); err == nil || !strings.Contains(err.Error(), "daily_loss_breaker") {
		t.Fatalf("relaunch armed config must block opens: got %v", err)
	}

	// Guard against plan-v2.md's informal "enforce" wording: "enforce" is
	// rejected by validation and silently falls back to "shadow" (never armed).
	badCfg := cfg
	badCfg.RiskControl.DailyLossBreaker = &store.DailyLossBreakerConfig{Mode: "enforce"}
	if err := store.ValidateTradingConfig(&badCfg); err == nil {
		t.Fatal(`mode "enforce" must fail ValidateTradingConfig (valid modes: off, shadow, armed)`)
	}
	if badEff := store.EffectiveDailyLossBreaker(badCfg.RiskControl.DailyLossBreaker); badEff.Mode != store.DailyLossBreakerShadow {
		t.Fatalf(`mode "enforce" resolved to %q, want fallback to "shadow"`, badEff.Mode)
	}
}
