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

// AUD-D1 (fixed in #93): the staged giveback fires at cur <= 0 (mark at or
// below entry for a long). The breakeven stop entry*(1+0.15%) would sit above
// mark; the rebuild now clamps it to the protective side of mark, so the
// exchange accepts it and the remainder is not emergency-closed.
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
	if raw := rebuildStop(state, "long", entry, e.cfg.GivebackBreakevenBufferFrac); raw <= mark {
		t.Fatalf("setup: the unclamped breakeven stop %.4f should be above mark %.4f", raw, mark)
	}
	clamped, ok := clampStopToProtectiveSide("long", mark, e.cfg.GivebackBreakevenBufferFrac)
	if !ok || !isStopOnProtectiveSide("long", clamped, mark) {
		t.Fatalf("clamped stop %.4f must be below mark %.4f", clamped, mark)
	}

	// End to end: the giveback partial places a valid stop and keeps the remainder.
	v := newProtectorSimVenue("AAAUSDT", "long", entry, 10, 10, 98, 104)
	at := newProtectorWalkTrader(v, nil)
	at.protectionState["AAAUSDT_long"] = state
	v.mark = mark
	closedAll, err := at.executeProtectionActionWith(e, "AAAUSDT", "long", 10, mark, entry, protectionGivebackClose)
	if closedAll || err != nil {
		t.Fatalf("giveback must keep the remainder with a valid stop: closedAll=%v err=%v orders=%v", closedAll, err, v.orders)
	}
	if v.stop <= 0 || v.stop >= mark || v.qty != 5 {
		t.Fatalf("stop %.4f must be below mark %.4f, qty %.4f", v.stop, mark, v.qty)
	}
	// No valid clamp (mark unknown in the clamp): a deliberate close.
	if _, ok := clampStopToProtectiveSide("long", 0, 0.0015); ok {
		t.Fatal("no mark: clamping must be impossible")
	}
}

// AUD-D2 (fixed in #93): leverage 0 or a non-float leverage no longer
// switches off the ROE rules. Int and float are accepted; <= 0 is unknown and
// uses the protector default (10) with a warning.
func TestAudit_D2_ZeroLeverageSwitchesOffROERules(t *testing.T) {
	def := store.EffectiveProtector(nil).DefaultLeverage
	for _, tc := range []struct {
		raw  interface{}
		want int
	}{{0.0, def}, {-3.0, def}, {nil, def}, {"", def}, {20, 20}, {int64(15), 15}, {20.0, 20}, {"12", 12}} {
		pos := map[string]interface{}{"leverage": tc.raw}
		if tc.raw == nil {
			pos = map[string]interface{}{}
		}
		if got := positionLeverage(pos, def, "XUSDT", "long"); got != tc.want {
			t.Fatalf("leverage %#v -> %d, want %d", tc.raw, got, tc.want)
		}
	}
	// With the default leverage, a -15% price move fires the hard-loss close.
	lev := positionLeverage(map[string]interface{}{"leverage": 0.0}, def, "XUSDT", "long")
	cur := calculateLeveragedPnLPct("long", 100, 85, lev)
	e := defaultProtectorEngine()
	now := time.Now()
	state := &positionProtectionState{OpenedAt: now.Add(-5 * time.Minute)}
	if action, _ := e.choose(state, protectorPos{Side: "long", Leverage: lev}, cur, -15, now); action != protectionHardLossClose {
		t.Fatalf("-15%% move with unknown leverage -> %q, want the hard-loss close", action)
	}
}

// AUD-D3 (#93): ROE-unit TP0 / profit-floor levels shrink with leverage. At
// 20x, TP0 (8% ROE) closes 35% on a 0.4% price move. #93 adds owner-set
// minimum price moves (tp0_min_move, profit_floor_min_move) as TP1 has.
// Unset, the behaviour is unchanged; set, the floor holds.
func TestAudit_D3_TP0At20xFiresOnFourTenthsPercentMove(t *testing.T) {
	e := defaultProtectorEngine()
	now := time.Now()
	entry, mark := 100.0, 100.4
	cur := calculateLeveragedPnLPct("long", entry, mark, 20)
	if math.Abs(cur-8) > 1e-9 {
		t.Fatalf("ROE %.4f", cur)
	}
	p := protectorPos{Side: "long", Leverage: 20}
	// Unset (default): unchanged, TP0 fires on the 0.4% move.
	if action, _ := e.choose(&positionProtectionState{OpenedAt: now.Add(-time.Hour)}, p, cur, 0.4, now); action != protectionTP0 {
		t.Fatalf("unset floor: expected TP0 at %.2f%% ROE, got %v", cur, action)
	}
	// Set: a 1.0% minimum price move holds TP0 back at 0.4%, and lets it fire at 1.0%.
	cfg := store.EffectiveProtector(&store.ProtectorConfig{
		TP0MinMove:         store.ProtectorLevel{Value: 1.0, Unit: store.ProtectorUnitPricePct},
		ProfitFloorMinMove: store.ProtectorLevel{Value: 0.5, Unit: store.ProtectorUnitPricePct},
	})
	ef := newProtectorEngine(cfg)
	if action, _ := ef.choose(&positionProtectionState{OpenedAt: now.Add(-time.Hour)}, p, cur, 0.4, now); action == protectionTP0 {
		t.Fatal("tp0_min_move 1.0%: TP0 must not fire on a 0.4% move")
	}
	cur1 := calculateLeveragedPnLPct("long", entry, 101.0, 20)
	if action, _ := ef.choose(&positionProtectionState{OpenedAt: now.Add(-time.Hour)}, p, cur1, 1.0, now); action != protectionTP0 {
		t.Fatalf("tp0_min_move 1.0%%: TP0 must fire on a 1.0%% move, got %v", action)
	}
	// Profit floor: a 6% ROE peak at 20x is a 0.3% move, below the 0.5% floor.
	if stop := ef.profitFloorStop(p, entry, 6); stop != 0 {
		t.Fatalf("profit_floor_min_move 0.5%%: no floor stop on a 0.3%% peak move, got %.4f", stop)
	}
	if stop := e.profitFloorStop(p, entry, 6); stop <= 0 {
		t.Fatal("unset floor: the profit floor stop is unchanged")
	}
	if stop := ef.profitFloorStop(p, entry, 12); stop <= entry {
		t.Fatalf("profit_floor_min_move 0.5%%: a 0.6%% peak move keeps the floor, got %.4f", stop)
	}
}
