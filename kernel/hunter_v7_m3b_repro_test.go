package kernel

import (
	"math"
	"testing"
	"time"

	"github.com/Aixxww/AiT/market"
	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// Unit U2 (M3b) — Offline reproduction fixtures for the 9 AI-skip
// backend_rr_infeasible verdicts observed in trader AIK11's 2026-11-28 live
// run (strategy "猎手v7 (导入)").
//
// Every numeric value below comes verbatim from the read-only postmortem data
// pack (strategy.json, dr.jsonl, sig.jsonl; sig cycle = dr cycle - 2). No live
// prompt-final prices are invented: cases whose prompt-final data.CurrentPrice
// is absent from the data pack are tested explicitly at recovered inputs
// (P_scan and EntryZone.Lower) and at their algebraic threshold boundaries.

// aik11LiveStrategyConfig returns the exact risk-control geometry from
// strategy.json in the 2026-11-28 live postmortem data pack.
func aik11LiveStrategyConfig() *store.StrategyConfig {
	return &store.StrategyConfig{
		Language: "zh",
		CoinSource: store.CoinSourceConfig{
			SourceType: "hunter_v7",
			Hunter: &store.HunterConfig{
				V7MaxOutput:     18,
				V7MinAIPriority: 50,
				V7Aggressive:    true,
				V7T2Enabled:     true,
			},
		},
		RiskControl: store.RiskControlConfig{
			MinRiskRewardRatio:        1.2,
			MaxEntryPriceDeviationPct: 0.5,
			MaxTakeProfitPriceMovePct: 3.5,
			MinStopLossPriceMovePct:   2.0,
		},
	}
}

type m3bCase struct {
	drCycle      int
	sigCycle     int
	symbol       string
	direction    string
	setupType    string
	stack        string
	signalClass  string
	zoneLower    float64
	zoneUpper    float64
	invalidation float64
	tp0          float64
	tp1          float64
	tp2          float64
	triggerPrice float64
	// scanPrice is recovered algebraically from sig.jsonl's deterministic
	// entry-zone construction (alt_ladder_breakdown_short: 0.35/1.10;
	// breakdown_momentum_short: 0.45/1.45; BTWUSDT: simultaneous sc=37
	// alt_ladder_momentum_long row where inv/0.978 == 1.1982 exactly).
	scanPrice float64
}

func (c m3bCase) candidateCoin(priceLast float64, rrRef float64) CandidateCoin {
	coin := CandidateCoin{
		Symbol:             c.symbol,
		Direction:          c.direction,
		V7SetupType:        c.setupType,
		V7Stack:            c.stack,
		V7SignalClass:      c.signalClass,
		V7EntryZone:        local.V7PriceZone{Lower: c.zoneLower, Upper: c.zoneUpper},
		V7Invalidation:     local.V7InvalidationRule{Price: c.invalidation},
		V7TP0Price:         c.tp0,
		V7TP1Price:         c.tp1,
		V7TP2Price:         c.tp2,
		V7TriggerPrice:     c.triggerPrice,
		V7RRReferencePrice: rrRef,
		// Native syncV7TargetsWithMultiTimeframeTP and T2 ToV7SignalOutput
		// both populate V7Targets with TP1 and TP2 (never TP0).
		V7Targets: []local.V7Target{
			{Price: c.tp1, Reason: "tp1"},
			{Price: c.tp2, Reason: "tp2"},
		},
	}
	if priceLast > 0 {
		coin.V7PriceContext = &local.V7PriceContext{Last: priceLast}
	}
	return coin
}

var m3bAllNineCases = []m3bCase{
	{
		drCycle: 3, sigCycle: 1, symbol: "ICNTUSDT", direction: "SHORT",
		setupType: "alt_ladder_breakdown_short", stack: "", signalClass: store.V7ClassImpulse,
		zoneLower: 0.29695500396590774, zoneUpper: 0.3008678486444835,
		invalidation: 0.30610473672439553,
		tp0: 0.2943051284664826, tp1: 0.27874944460898965, tp2: 0.26261985925202486,
		scanPrice: 0.2982,
	},
	{
		drCycle: 14, sigCycle: 12, symbol: "THEUSDT", direction: "SHORT",
		setupType: "alt_ladder_breakdown_short", stack: "", signalClass: store.V7ClassImpulse,
		zoneLower: 0.02998424608785048, zoneUpper: 0.03056804409746326,
		invalidation: 0.03092425,
		tp0: 0.029417549232054644, tp1: 0.028008783406623394, tp2: 0.026194894057796615,
		scanPrice: 0.03017,
	},
	{
		drCycle: 19, sigCycle: 17, symbol: "LISTAUSDT", direction: "SHORT",
		setupType: "breakdown_momentum_short", stack: "", signalClass: store.V7ClassImpulse,
		zoneLower: 0.087752427, zoneUpper: 0.08851794,
		invalidation: 0.08992578,
		tp0: 0.08714729371562796, tp1: 0.08117056365150323, tp2: 0.07559886777270582,
		scanPrice: 0.08799,
	},
	{
		drCycle: 20, sigCycle: 18, symbol: "BULLAUSDT", direction: "SHORT",
		setupType: "alt_ladder_breakdown_short", stack: "", signalClass: store.V7ClassImpulse,
		zoneLower: 0.06151175998284031, zoneUpper: 0.06222908575105649,
		invalidation: 0.06318914296609329,
		tp0: 0.06081544678763248, tp1: 0.05421083787846275, tp2: 0.048083169887674235,
		scanPrice: 0.06174,
	},
	{
		drCycle: 23, sigCycle: 21, symbol: "BIGTIMEUSDT", direction: "SHORT",
		setupType: "breakdown_momentum_short", stack: "", signalClass: store.V7ClassImpulse,
		zoneLower: 0.005704556, zoneUpper: 0.00575432,
		invalidation: 0.005845840000000001,
		tp0: 0.005636316399999999, tp1: 0.005403886945647293, tp2: 0.005143446102165128,
		scanPrice: 0.00572,
	},
	{
		drCycle: 30, sigCycle: 28, symbol: "ICNTUSDT", direction: "SHORT",
		setupType: "alt_ladder_breakdown_short", stack: "", signalClass: store.V7ClassImpulse,
		zoneLower: 0.28743524967392775, zoneUpper: 0.290153036413012,
		invalidation: 0.2946426,
		tp0: 0.28657971659786685, tp1: 0.2701994823322294, tp2: 0.255323753763237,
		scanPrice: 0.2883,
	},
	{
		drCycle: 33, sigCycle: 31, symbol: "LISTAUSDT", direction: "SHORT",
		setupType: "alt_ladder_breakdown_short", stack: "", signalClass: store.V7ClassImpulse,
		zoneLower: 0.08633701639200003, zoneUpper: 0.08691210773142852,
		invalidation: 0.08842344,
		tp0: 0.08546276137600016, tp1: 0.08063685682485919, tp2: 0.07584669263537512,
		scanPrice: 0.08652,
	},
	{
		drCycle: 39, sigCycle: 37, symbol: "BTWUSDT", direction: "LONG",
		setupType: "momentum_continuation_long", stack: local.V7StackT2, signalClass: store.V7ClassImpulse,
		zoneLower: 1.1863, zoneUpper: 1.1874863,
		invalidation: 1.1756233,
		tp0: 0, tp1: 1.2088397, tp2: 1.2230753,
		triggerPrice: 1.1874863,
		scanPrice:    1.1982,
	},
	{
		drCycle: 41, sigCycle: 39, symbol: "INITUSDT", direction: "SHORT",
		setupType: "alt_ladder_breakdown_short", stack: "", signalClass: store.V7ClassImpulse,
		zoneLower: 0.08480658619273516, zoneUpper: 0.08557160101556752,
		invalidation: 0.0869211,
		tp0: 0.0838034884874724, tp1: 0.07798799999999999, tp2: 0.07132568502217965,
		scanPrice: 0.08505,
	},
}

func TestHunterV7M3B_LiveStrategyGeometryResolution(t *testing.T) {
	liveGeom := HunterV7ExecutionGeometryForConfig(aik11LiveStrategyConfig())
	if liveGeom.MinRR != 1.2 || liveGeom.MinSLMovePct != 2.0 || liveGeom.MaxTPMovePct != 3.5 || liveGeom.MaxDriftPct != 0.5 {
		t.Fatalf("live geometry mismatch: %+v", liveGeom)
	}
	if liveGeom.T2MinRR != 1.2 || liveGeom.T2MinSLMovePct != 0.5 {
		t.Fatalf("live T2 defaults mismatch: T2MinRR=%v T2MinSL=%v", liveGeom.T2MinRR, liveGeom.T2MinSLMovePct)
	}

	// Cross-check U1 default structural geometry (when MaxTP is unset/3.0 -> 3.25%).
	defaultCfg := &store.StrategyConfig{
		CoinSource:  store.CoinSourceConfig{SourceType: "hunter_v7"},
		RiskControl: store.RiskControlConfig{MinRiskRewardRatio: 1.2, MinStopLossPriceMovePct: 2.0, MaxEntryPriceDeviationPct: 0.5},
	}
	defaultGeom := HunterV7ExecutionGeometryForConfig(defaultCfg)
	if defaultGeom.MaxTPMovePct != 3.25 {
		t.Fatalf("default MaxTPMovePct = %v, want 3.25", defaultGeom.MaxTPMovePct)
	}
}

// Directly reproducible cases (zero invented prices):
//   - C3  ICNTUSDT (TP_CAP): clearing 5m_or_15m_close_below_trigger requires
//     P_live <= EntryZone.Lower; at EntryZone.Lower itself, rawRisk = 3.081% > 2.917%,
//     so MaxTP=3.5% caps TP2 (11.56%) and forces capped RR = 1.136 < 1.2.
//   - C14 THEUSDT  (TP_CAP): same mechanism; at EntryZone.Lower, rawRisk = 3.135% > 2.917%,
//     capped RR = 1.116 < 1.2 while uncapped raw RR(tp2) = 4.03.
//   - C39 BTWUSDT  (RAW_GEOMETRY): T2 signal passes assembly at EntryZone.Upper
//     (1.1874863, RR = 3.00), then hunterV7CandidateWithLiveMarketPrice clears
//     V7RRReferencePrice at prompt-final and reprices at recovered sc=37 scan
//     price 1.1982 -> rawRisk = 1.884% (above T2 MinSL 0.5%), TP2 = 2.076%
//     (uncapped < 3.5%), raw RR = 1.102 < 1.2.
func TestHunterV7M3B_ReproducibleLiveVerdicts(t *testing.T) {
	g := HunterV7ExecutionGeometryForConfig(aik11LiveStrategyConfig())
	now := time.Date(2026, 11, 28, 4, 12, 15, 0, time.UTC)

	// C3 ICNTUSDT and C14 THEUSDT at recovered EntryZone.Lower (no PriceContext.Last invented).
	for _, tc := range []struct {
		idx         int
		wantRawRisk float64
		wantRR      float64
	}{
		{0, 3.0812, 1.1359}, // C3 ICNTUSDT
		{1, 3.1350, 1.1164}, // C14 THEUSDT
	} {
		c := m3bAllNineCases[tc.idx]
		coin := c.candidateCoin(0, 0) // falls back to entry_zone_lower
		res := hunterV7BackendCappedRREval(coin, g, true)
		reason := hunterV7BackendCappedRRWaitReason(coin, g)
		if !res.ok || reason != "backend_rr_infeasible" {
			t.Fatalf("C%d %s: ok=%v reason=%q, want ok=true backend_rr_infeasible", c.drCycle, c.symbol, res.ok, reason)
		}
		if res.priceSource != hunterV7RefSourceEntryZoneLower {
			t.Fatalf("C%d %s: priceSource=%q, want %q", c.drCycle, c.symbol, res.priceSource, hunterV7RefSourceEntryZoneLower)
		}
		if res.minStopFloorApplied {
			t.Fatalf("C%d %s: minStopFloorApplied=true, want false (rawRisk=%.4f%%)", c.drCycle, c.symbol, res.rawRiskPct)
		}
		if !res.bestTargetCapped || res.maxTP != 3.5 || res.bestRewardPct != 3.5 {
			t.Fatalf("C%d %s: expected TP_CAP at 3.5%%, got capped=%v maxTP=%v bestReward=%v", c.drCycle, c.symbol, res.bestTargetCapped, res.maxTP, res.bestRewardPct)
		}
		if uncappedRR := res.bestTargetRaw / res.rawRiskPct; uncappedRR < res.minRR {
			t.Fatalf("C%d %s: uncapped RR=%.3f < floor %.2f, expected feasible raw geometry", c.drCycle, c.symbol, uncappedRR, res.minRR)
		}
		if math.Abs(res.rawRiskPct-tc.wantRawRisk) > 0.01 || math.Abs(res.rr-tc.wantRR) > 0.01 {
			t.Fatalf("C%d %s: rawRisk=%.4f%% rr=%.4f, want ~%.4f%% / ~%.4f", c.drCycle, c.symbol, res.rawRiskPct, res.rr, tc.wantRawRisk, tc.wantRR)
		}
		rec := buildHunterV7RRVerdictRecord(coin, g, reason, HunterV7RRVerdictStagePromptFinal, c.drCycle, now)
		assertRRVerdictInternallyConsistent(t, rec)
	}

	// C39 BTWUSDT (T2 LONG): passes assembly at zone upper (V7RRReferencePrice),
	// fails prompt-final RAW_GEOMETRY at recovered sc=37 price 1.1982.
	btw := m3bAllNineCases[7]
	assemblyCoin := btw.candidateCoin(btw.scanPrice, btw.zoneUpper)
	if r := hunterV7BackendCappedRRWaitReason(assemblyCoin, g); r != "" {
		t.Fatalf("C39 BTWUSDT assembly should pass at zoneUpper %.7f, got %q", btw.zoneUpper, r)
	}
	promptCoin := hunterV7CandidateWithLiveMarketPrice(assemblyCoin, &market.Data{CurrentPrice: btw.scanPrice})
	res := hunterV7BackendCappedRREval(promptCoin, g, true)
	reason := hunterV7BackendCappedRRWaitReason(promptCoin, g)
	if !res.ok || reason != "backend_rr_infeasible" {
		t.Fatalf("C39 BTWUSDT prompt-final: ok=%v reason=%q, want ok=true backend_rr_infeasible", res.ok, reason)
	}
	if res.priceSource != hunterV7RefSourcePriceContextLast || res.price != 1.1982 {
		t.Fatalf("C39 BTWUSDT price=%v (%q), want 1.1982 (%q)", res.price, res.priceSource, hunterV7RefSourcePriceContextLast)
	}
	if res.resolvedMinSL != 0.5 || res.minSLSource != hunterV7RiskSourceT2Config {
		t.Fatalf("C39 BTWUSDT resolvedMinSL=%v (%q), want 0.5 (%q)", res.resolvedMinSL, res.minSLSource, hunterV7RiskSourceT2Config)
	}
	if res.minStopFloorApplied || res.bestTargetCapped {
		t.Fatalf("C39 BTWUSDT must be RAW_GEOMETRY (neither floor nor cap), got floor=%v cap=%v", res.minStopFloorApplied, res.bestTargetCapped)
	}
	if math.Abs(res.rawRiskPct-1.8842) > 0.01 || math.Abs(res.bestRewardPct-2.0761) > 0.01 || math.Abs(res.rr-1.1018) > 0.01 {
		t.Fatalf("C39 BTWUSDT terms: risk=%.4f%% reward=%.4f%% rr=%.4f", res.rawRiskPct, res.bestRewardPct, res.rr)
	}
	rec := buildHunterV7RRVerdictRecord(promptCoin, g, reason, HunterV7RRVerdictStagePromptFinal, btw.drCycle, now)
	assertRRVerdictInternallyConsistent(t, rec)
}

// The remaining 6 native short cases (C19 LISTA, C20 BULLA, C23 BIGTIME,
// C30 ICNT, C33 LISTA, C41 INIT) pass at every recovered price (P_scan and
// EntryZone.Lower) under live strategy.json (MaxTP=3.5%). Exact recomputation
// is UNRECOVERABLE because prompt-final data.CurrentPrice is absent from
// dr.jsonl/sig.jsonl; however, one epsilon below P_fail = inv/(1 + 3.5/120)
// every one fails strictly on TP_CAP (with uncapped raw RR > 3.2).
func TestHunterV7M3B_UnrecoverablePromptLivePriceCases(t *testing.T) {
	liveGeom := HunterV7ExecutionGeometryForConfig(aik11LiveStrategyConfig())
	defaultGeom := HunterV7ExecutionGeometryForConfig(&store.StrategyConfig{
		CoinSource:  store.CoinSourceConfig{SourceType: "hunter_v7"},
		RiskControl: store.RiskControlConfig{MinRiskRewardRatio: 1.2, MinStopLossPriceMovePct: 2.0, MaxEntryPriceDeviationPct: 0.5},
	})

	unrecoverableIdxs := []int{2, 3, 4, 5, 6, 8} // C19, C20, C23, C30, C33, C41
	for _, idx := range unrecoverableIdxs {
		c := m3bAllNineCases[idx]

		// At recovered P_scan: passes live 3.5% cap.
		if r := hunterV7BackendCappedRRWaitReason(c.candidateCoin(c.scanPrice, 0), liveGeom); r != "" {
			t.Fatalf("C%d %s at P_scan=%.6f unexpectedly failed live geometry: %q", c.drCycle, c.symbol, c.scanPrice, r)
		}
		// At recovered EntryZone.Lower: still passes live 3.5% cap -> proves
		// prompt-final data.CurrentPrice is required to reproduce the failure.
		atZLow := c.candidateCoin(0, 0)
		if r := hunterV7BackendCappedRRWaitReason(atZLow, liveGeom); r != "" {
			t.Fatalf("C%d %s at EntryZone.Lower=%.6f unexpectedly failed live 3.5%% geometry: %q", c.drCycle, c.symbol, c.zoneLower, r)
		}
		// Cross-check U1 default 3.25% cap at EntryZone.Lower: only C20 BULLA
		// (rawRisk@zlow = 2.727% > 2.708%) fails at EntryZone.Lower under 3.25%.
		gotDefault := hunterV7BackendCappedRRWaitReason(atZLow, defaultGeom)
		wantDefault := ""
		if c.drCycle == 20 {
			wantDefault = "backend_rr_infeasible"
		}
		if gotDefault != wantDefault {
			t.Fatalf("C%d %s at EntryZone.Lower under default 3.25%% cap: got %q, want %q", c.drCycle, c.symbol, gotDefault, wantDefault)
		}

		// Conditional TP_CAP cliff: just below P_fail = inv/(1 + 3.5/120),
		// TP_CAP fails with tp1 already capped at 3.5% (strict '>' keeps tp1 as
		// best target; uncapped raw RR(tp1) >= 1.66 > 1.2) and minStopFloorApplied == false.
		pFail := c.invalidation / (1.0 + liveGeom.MaxTPMovePct/(100.0*liveGeom.MinRR))
		atCliff := c.candidateCoin(pFail*0.9999, 0)
		res := hunterV7BackendCappedRREval(atCliff, liveGeom, true)
		if !res.ok || hunterV7BackendCappedRRWaitReason(atCliff, liveGeom) != "backend_rr_infeasible" {
			t.Fatalf("C%d %s below P_fail=%.6f must fail backend_rr_infeasible: %+v", c.drCycle, c.symbol, pFail, res)
		}
		if res.minStopFloorApplied || !res.bestTargetCapped || res.bestTargetLabel != "tp1" || res.bestTargetRaw/res.rawRiskPct < 1.6 {
			t.Fatalf("C%d %s at cliff expected TP_CAP on tp1 with rawRR>=1.6, got floor=%v cap=%v label=%q rawRR=%.2f",
				c.drCycle, c.symbol, res.minStopFloorApplied, res.bestTargetCapped, res.bestTargetLabel, res.bestTargetRaw/res.rawRiskPct)
		}
	}
}

// Algebraic proof that the 2.0% min-stop floor (and T2's 0.5% floor) bound
// ZERO of the 9 cases: whenever rawRisk < MinSL triggers the floor+buffer,
// bestRewardPct is capped at 3.5% and cappedRR >= 1.667 (native) or 5.833 (T2).
func TestHunterV7M3B_MinStopFloorNeverBoundAnyCase(t *testing.T) {
	g := HunterV7ExecutionGeometryForConfig(aik11LiveStrategyConfig())
	for _, c := range m3bAllNineCases {
		var pTriggerFloor float64
		if c.direction == "SHORT" {
			// rawRisk = 1.99% < 2.0% native MinSL -> floor applies (effRisk = 2.10%).
			pTriggerFloor = c.invalidation / 1.0199
		} else {
			// rawRisk = 0.49% < 0.5% T2 MinSL -> floor applies (effRisk = 0.60%).
			pTriggerFloor = c.invalidation / (1.0 - 0.0049)
		}
		coin := c.candidateCoin(pTriggerFloor, 0)
		res := hunterV7BackendCappedRREval(coin, g, true)
		if !res.ok || !res.minStopFloorApplied {
			t.Fatalf("C%d %s: expected floor applied at P=%.6f, got ok=%v floor=%v", c.drCycle, c.symbol, pTriggerFloor, res.ok, res.minStopFloorApplied)
		}
		if r := hunterV7BackendCappedRRWaitReason(coin, g); r != "" {
			t.Fatalf("C%d %s: STOP_FLOOR must not fail backend RR, got %q (rr=%.3f)", c.drCycle, c.symbol, r, res.rr)
		}
	}
}

// Empirical verification from dr.jsonl C17 (WLFIUSDT, sig sc=15) that AIK11's
// live run used strategy.json's MaxTPMovePct=3.5% (not U1's default 3.25%):
// WLFI reached the LLM as EXECUTABLE at live price 0.03494 (rawRisk = 2.902%,
// which passes 3.5/1.2 = 2.917% with RR=1.206, and would fail 3.25/1.2 = 2.708%).
func TestHunterV7M3B_EmpiricalLiveTPCap_C17WLFI(t *testing.T) {
	liveGeom := HunterV7ExecutionGeometryForConfig(aik11LiveStrategyConfig())
	defaultGeom := HunterV7ExecutionGeometryForConfig(&store.StrategyConfig{
		CoinSource:  store.CoinSourceConfig{SourceType: "hunter_v7"},
		RiskControl: store.RiskControlConfig{MinRiskRewardRatio: 1.2, MinStopLossPriceMovePct: 2.0, MaxEntryPriceDeviationPct: 0.5},
	})
	wlfi := CandidateCoin{
		Symbol:         "WLFIUSDT",
		Direction:      "SHORT",
		V7SetupType:    "alt_ladder_breakdown_short",
		V7SignalClass:  store.V7ClassImpulse,
		V7EntryZone:    local.V7PriceZone{Lower: 0.0351052, Upper: 0.0353403},
		V7Invalidation: local.V7InvalidationRule{Price: 0.035954},
		V7PriceContext: &local.V7PriceContext{Last: 0.03494}, // quoted verbatim in dr.jsonl C17
		V7Targets:      []local.V7Target{{Price: 0.0320}, {Price: 0.0300}},
	}
	if r := hunterV7BackendCappedRRWaitReason(wlfi, liveGeom); r != "" {
		t.Fatalf("C17 WLFI must pass live 3.5%% cap, got %q", r)
	}
	if r := hunterV7BackendCappedRRWaitReason(wlfi, defaultGeom); r != "backend_rr_infeasible" {
		t.Fatalf("C17 WLFI must fail default 3.25%% cap, got %q", r)
	}
}
