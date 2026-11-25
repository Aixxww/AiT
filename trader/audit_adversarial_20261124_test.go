package trader

// Adversarial audit of Hunter v7 (issue #69, docs/audit/adversarial-review-20261124.md).
//
// Each test characterises CURRENT behaviour at 461074e that the audit reports
// as a finding. The tests pass today on purpose (the audit changes no trading
// logic, and the suite must stay green); the assertion that encodes the flaw
// is marked "FLAW" and must be inverted by the fix. Finding IDs match the
// report's findings table.

import (
	"math"
	"testing"
	"time"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/store"
)

// AUD-C1 (fixed in #92): in the default geometry_repair mode every repair
// pass measures drift from the LLM's original price, so two in-band moves no
// longer stack past the configured 0.5% limit (+0.10 repair band).
func TestAudit_C1_RepairModeDriftStacksAcrossPasses(t *testing.T) {
	at := rejectModeTrader("") // "" = repair (historical default)
	d := rmDecision(100, 97.5, 104)
	llmPrice := d.Price
	if !at.repairHunterV7DecisionPrice(d, 100.55, "long") {
		t.Fatal("first in-band drift must be refreshed in repair mode")
	}
	if at.repairHunterV7DecisionPrice(d, 101.1, "long") {
		t.Fatalf("second pass re-anchored to %.4f: drift from the LLM price %.2f stacked across passes", d.Price, llmPrice)
	}
	limit := at.decisionMaxEntryPriceDeviationPct(d) + 0.10
	if total := (d.Price - llmPrice) / llmPrice * 100; total > limit {
		t.Fatalf("reference drifted %.3f%% from the LLM price, beyond the %.2f%% repair band", total, limit)
	}
	// The executable price is still checked against the (bounded) reference
	// and rejected.
	if err := at.validateOpenDecision(d, 101.1, "long"); err == nil {
		t.Fatal("an open 1.1% above the LLM price must be rejected with a 0.5% limit")
	}
	// Favourable re-anchoring is kept, bounded from the original price.
	f := rmDecision(100, 97.5, 104)
	if !at.repairHunterV7DecisionPrice(f, 99.0, "long") || f.Price != 99.0 {
		t.Fatalf("favourable drift must still re-anchor, got %.4f", f.Price)
	}
	if at.repairHunterV7DecisionPrice(f, 95.5, "long") {
		t.Fatalf("favourable drift beyond 4%% from the LLM price must not re-anchor (price %.4f)", f.Price)
	}
}

// AUD-C1b (locks correct behaviour): "favourable" drift up to 4% refreshes
// the decision price in repair mode, but a move through the model's own stop
// is still rejected by the SL/TP ordering check, so the invalidated long does
// not open.
func TestAudit_C1b_DriftThroughStopIsStillRejected(t *testing.T) {
	at := rejectModeTrader("")
	d := rmDecision(100, 97.5, 104) // LLM: long at 100, invalidation 97.5
	live := 96.5                    // price already 1% below the LLM's stop
	at.repairHunterV7OpenDecision(d, live, "long")
	if d.Price != live {
		t.Fatalf("favourable drift is expected to refresh the price, got %.4f", d.Price)
	}
	if err := at.validateOpenDecision(d, live, "long"); err == nil {
		t.Fatalf("a long whose stop is above the live price must be rejected (stop %.4f, live %.4f)", d.StopLoss, live)
	}
}

// AUD-C2 (fixed in #92): under geometry_repair=reject a take-profit beyond
// the cap rejects the open instead of being rewritten.
func TestAudit_C2_TakeProfitCapIgnoresRejectMode(t *testing.T) {
	at := rejectModeTrader("reject")
	at.config.StrategyConfig.RiskControl.MaxTakeProfitPriceMovePct = 3
	d := rmDecision(100, 97, 110)
	capped, err := at.capTakeProfitToTP1(d, 100, "long")
	if err == nil || capped {
		t.Fatalf("reject mode must reject a TP beyond the cap (capped=%v err=%v)", capped, err)
	}
	if d.TakeProfit != 110 {
		t.Fatalf("reject mode rewrote take_profit 110 to %.4f", d.TakeProfit)
	}
	// Repair mode keeps capping.
	r := rejectModeTrader("")
	r.config.StrategyConfig.RiskControl.MaxTakeProfitPriceMovePct = 3
	rd := rmDecision(100, 97, 110)
	if capped, err := r.capTakeProfitToTP1(rd, 100, "long"); err != nil || !capped || rd.TakeProfit >= 110 {
		t.Fatalf("repair mode must keep capping (capped=%v err=%v tp=%.4f)", capped, err, rd.TakeProfit)
	}
}

// AUD-E1: the pre-order freshness guard prefers the model-supplied
// signal_age_ms over the backend's own price age, so the model can declare a
// stale high-volatility signal fresh. Ordinary candidates have no age gate.
func TestAudit_E1_FreshnessGuardUsesBackendAge(t *testing.T) {
	cand := &kernel.CandidateCoin{Symbol: "XUSDT", V7SetupType: "range_expansion_event", V7RiskTags: []string{"high_volatility"}}
	cand.V7DataFreshness.PriceAgeMs = 120_000
	if err := validateHunterV7SignalFreshness(cand, &kernel.Decision{Symbol: "XUSDT"}); err == nil {
		t.Fatal("a 120s-old high-volatility signal must be blocked on backend age")
	}
	// Fixed by #87: the model's signal_age_ms no longer overrides the backend age.
	if err := validateHunterV7SignalFreshness(cand, &kernel.Decision{Symbol: "XUSDT", SignalAgeMs: 1}); err == nil {
		t.Fatal("model signal_age_ms=1 must not override a 120s backend age")
	}
	// Still open (needs an owner-set per-class limit, not a code default):
	ordinary := &kernel.CandidateCoin{Symbol: "YUSDT", V7SetupType: "trend_breakout_long"}
	ordinary.V7DataFreshness.PriceAgeMs = 10 * 60 * 1000
	if err := validateHunterV7SignalFreshness(ordinary, &kernel.Decision{Symbol: "YUSDT"}); err != nil {
		t.Fatalf("FLAW no longer present: ordinary candidates now have an age gate (%v); update AUD-E1", err)
	}
}

// AUD-D1: the staged giveback fires at cur <= 0 (mark at or below entry for a
// long), and the stop it then rebuilds is entry*(1+0.15%), i.e. above mark.
// The exchange rejects such a stop ("would immediately trigger"); the old stop
// was already cancelled, so the path ends in the emergency close.
func TestAudit_D1_GivebackRebuildsStopOnTheWrongSideOfMark(t *testing.T) {
	e := defaultProtectorEngine()
	now := time.Now()
	state := &positionProtectionState{PeakPnLPct: 6, OpenedAt: now.Add(-time.Hour)}
	entry, mark := 100.0, 99.99
	cur := calculateLeveragedPnLPct("long", entry, mark, 10)
	action, _ := e.choose(state, protectorPos{Side: "long", Leverage: 10}, cur, (mark-entry)/entry*100, now)
	if action != protectionGivebackClose {
		t.Fatalf("expected the giveback-to-zero rule, got %v", action)
	}
	stop := rebuildStop(state, "long", entry, e.cfg.GivebackBreakevenBufferFrac)
	if stop <= mark {
		t.Fatalf("FLAW no longer present: rebuilt stop %.4f is below mark %.4f; update AUD-D1", stop, mark)
	}
	t.Logf("giveback at mark %.2f rebuilds a LONG stop at %.4f (above mark)", mark, stop)
}

// AUD-D2: ROE is 0 when the position reports leverage 0, which switches off
// every ROE rule (hard loss, early loss, TP0, givebacks, profit floor). The
// DefaultLeverage fallback in checkPositionDrawdown only applies when the
// "leverage" key is not a float64, so a parsed 0.0 (Binance) or an int
// (Gate adapter) never reaches it.
func TestAudit_D2_ZeroLeverageSwitchesOffROERules(t *testing.T) {
	cur := calculateLeveragedPnLPct("long", 100, 85, 0) // -15% price move
	if cur != 0 {
		t.Fatalf("FLAW no longer present: ROE with leverage 0 is %.2f; update AUD-D2", cur)
	}
	e := defaultProtectorEngine()
	now := time.Now()
	state := &positionProtectionState{OpenedAt: now.Add(-5 * time.Minute)}
	action, _ := e.choose(state, protectorPos{Side: "long", Leverage: 0}, cur, -15, now)
	if action == protectionHardLossClose {
		t.Fatal("FLAW no longer present: hard loss fires with leverage 0; update AUD-D2")
	}
	t.Logf("-15%% price move with leverage 0 -> action %q (no hard-loss close)", action)

	// The extraction rule used by checkPositionDrawdown (auto_trader_risk.go:149-152).
	extract := func(pos map[string]interface{}) int {
		leverage := store.EffectiveProtector(nil).DefaultLeverage
		if lev, ok := pos["leverage"].(float64); ok {
			leverage = int(lev)
		}
		return leverage
	}
	if got := extract(map[string]interface{}{"leverage": 0.0}); got != 0 {
		t.Fatalf("parsed 0.0 leverage -> %d", got)
	}
	if got := extract(map[string]interface{}{"leverage": 20}); got != 10 {
		t.Fatalf("int leverage 20 -> %d (default used instead of the real leverage)", got)
	}
}

// AUD-D3: ROE-unit TP0 / profit-floor levels shrink with leverage. At 20x,
// TP0 (8% ROE) closes 35% of the position on a 0.4% price move, about four
// round-trip taker fees.
func TestAudit_D3_TP0At20xFiresOnFourTenthsPercentMove(t *testing.T) {
	e := defaultProtectorEngine()
	now := time.Now()
	state := &positionProtectionState{OpenedAt: now.Add(-time.Hour)}
	entry, mark := 100.0, 100.4
	cur := calculateLeveragedPnLPct("long", entry, mark, 20)
	action, _ := e.choose(state, protectorPos{Side: "long", Leverage: 20}, cur, 0.4, now)
	if action != protectionTP0 {
		t.Fatalf("expected TP0 at %.2f%% ROE, got %v", cur, action)
	}
	if math.Abs(cur-8) > 1e-9 {
		t.Fatalf("ROE %.4f", cur)
	}
	t.Logf("20x: +0.40%% price = %.1f%% ROE -> %v (closes %.0f%%)", cur, action, e.cfg.TP0CloseRatio*100)
}
