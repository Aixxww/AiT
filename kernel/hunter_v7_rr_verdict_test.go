package kernel

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/Aixxww/AiT/market"
	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newRRVerdictTestStore(t *testing.T) *store.HunterV7SignalStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	if err := db.AutoMigrate(&store.HunterV7RRVerdictRecord{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return store.NewHunterV7SignalStore(db)
}

func attachRRVerdictStore(t *testing.T, engine *StrategyEngine, st *store.HunterV7SignalStore) {
	t.Helper()
	engine.SetV7RRVerdictRecorder(func(rec store.HunterV7RRVerdictRecord) {
		if err := st.CreateRRVerdict(&rec); err != nil {
			t.Errorf("CreateRRVerdict: %v", err)
		}
	})
}

func assertRRVerdictInternallyConsistent(t *testing.T, rec store.HunterV7RRVerdictRecord) {
	t.Helper()
	if rec.Symbol == "" || rec.Direction == "" || rec.SetupType == "" || rec.Reason == "" || rec.Stage == "" {
		t.Fatalf("missing identity fields: %+v", rec)
	}
	if rec.StopRepairBufferPct != hunterV7BackendStopRepairBuffer {
		t.Fatalf("stop_repair_buffer_pct = %v, want %v", rec.StopRepairBufferPct, hunterV7BackendStopRepairBuffer)
	}
	var targets []store.HunterV7RRTargetTerm
	if err := json.Unmarshal([]byte(rec.TargetRewardsJSON), &targets); err != nil {
		t.Fatalf("invalid target_rewards_json %q: %v", rec.TargetRewardsJSON, err)
	}
	if !rec.OK {
		if rec.CappedRR != 0 {
			t.Fatalf("ok=false must have capped_rr=0, got %v", rec.CappedRR)
		}
		return
	}
	if rec.ReferencePrice <= 0 || rec.InvalidationPrice <= 0 {
		t.Fatalf("ok=true requires positive reference and invalidation prices: %+v", rec)
	}
	wantRawRisk := (rec.ReferencePrice - rec.InvalidationPrice) / rec.ReferencePrice * 100
	if rec.Direction == "SHORT" {
		wantRawRisk = (rec.InvalidationPrice - rec.ReferencePrice) / rec.ReferencePrice * 100
	}
	if math.Abs(rec.RawRiskPct-wantRawRisk) > 1e-9 {
		t.Fatalf("raw_risk_pct = %.12f, recomputed %.12f", rec.RawRiskPct, wantRawRisk)
	}
	wantFloorApplied := rec.RawRiskPct < rec.MinSLMovePct
	if rec.MinStopFloorApplied != wantFloorApplied {
		t.Fatalf("min_stop_floor_applied = %v, want %v (raw=%.6f minSL=%.6f)", rec.MinStopFloorApplied, wantFloorApplied, rec.RawRiskPct, rec.MinSLMovePct)
	}
	wantEffRisk := rec.RawRiskPct
	if wantFloorApplied {
		wantEffRisk = rec.MinSLMovePct + rec.StopRepairBufferPct
	}
	if math.Abs(rec.EffectiveRiskPct-wantEffRisk) > 1e-9 {
		t.Fatalf("effective_risk_pct = %.12f, recomputed %.12f", rec.EffectiveRiskPct, wantEffRisk)
	}
	if len(targets) == 0 {
		t.Fatalf("ok=true must record per-target terms")
	}
	bestReward := 0.0
	bestIdx := -1
	for _, tg := range targets {
		capped := tg.RawRewardPct
		if capped > 0 && capped > rec.MaxTPMovePct {
			capped = rec.MaxTPMovePct
		}
		if math.Abs(tg.CappedRewardPct-capped) > 1e-9 {
			t.Fatalf("target[%d] capped_reward_pct = %.12f, want %.12f", tg.Index, tg.CappedRewardPct, capped)
		}
		if tg.RawRewardPct > 0 && capped > bestReward {
			bestReward = capped
			bestIdx = tg.Index
		}
	}
	if math.Abs(rec.BestRewardPct-bestReward) > 1e-9 {
		t.Fatalf("best_reward_pct = %.12f, recomputed %.12f", rec.BestRewardPct, bestReward)
	}
	if rec.BestTargetIndex != bestIdx {
		t.Fatalf("best_target_index = %d, recomputed %d", rec.BestTargetIndex, bestIdx)
	}
	recomputedRR := bestReward / wantEffRisk
	if math.Abs(rec.CappedRR-recomputedRR) > 1e-9 {
		t.Fatalf("capped_rr = %.12f, recomputed %.12f", rec.CappedRR, recomputedRR)
	}
	if rec.CappedRR >= rec.MinRRFloor {
		t.Fatalf("infeasible verdict must have capped_rr (%.6f) < min_rr_floor (%.6f)", rec.CappedRR, rec.MinRRFloor)
	}
}

// (a) Assembly-stage backend_rr_infeasible produces exactly one persisted
// record with all computation terms populated and internally consistent.
func TestHunterV7RRVerdict_InfeasiblePersistsConsistentRecord(t *testing.T) {
	st := newRRVerdictTestStore(t)
	cfg := &store.StrategyConfig{
		CoinSource:  store.CoinSourceConfig{SourceType: "hunter_v7"},
		RiskControl: store.RiskControlConfig{MinStopLossPriceMovePct: 4.0},
	}
	engine := NewStrategyEngine(cfg)
	engine.v7CycleCounter = 12
	attachRRVerdictStore(t, engine, st)

	sig := local.V7SignalOutput{
		SignalID:       "12|GEOUSDT|LONG|trend_breakout_long",
		Symbol:         "GEOUSDT",
		Direction:      local.V7DirLong,
		SetupType:      local.V7SetupTrendBreakoutLong,
		Status:         "candidate",
		RiskLevel:      "MEDIUM",
		MarketRegime:   local.V7RegimeTrendUp,
		AIPriority:     62,
		SetupScore:     60,
		TimingScore:    58,
		LiquidityScore: 60,
		RiskScore:      30,
		PriceCtx:       &local.V7PriceContext{Last: 100},
		Invalidation:   local.V7InvalidationRule{Price: 98.8},
		TP0Price:       102.0,
		TP1Price:       104.5,
		Targets: []local.V7Target{
			{Price: 102.0, Reason: "tp0_structure"},
			{Price: 104.5, Reason: "tp1_extension"},
		},
	}

	cands := engine.hunterV7SignalsToCandidateCoins([]local.V7SignalOutput{sig}, "BOTH")
	if len(cands) != 1 || cands[0].V7ExecutionTier != "WATCH" || cands[0].V7TierReason != "backend_rr_infeasible" {
		t.Fatalf("unexpected candidate verdict: %+v", cands)
	}

	rows, err := st.QueryRRVerdicts(10)
	if err != nil {
		t.Fatalf("QueryRRVerdicts: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("persisted rows = %d, want exactly 1", len(rows))
	}
	rec := rows[0]
	assertRRVerdictInternallyConsistent(t, rec)
	if rec.CycleNumber != 12 || rec.Stage != HunterV7RRVerdictStageAssembly {
		t.Fatalf("cycle/stage = %d/%q, want 12/assembly", rec.CycleNumber, rec.Stage)
	}
	if rec.SignalID != sig.SignalID || rec.ReferencePriceSource != hunterV7RefSourcePriceContextLast {
		t.Fatalf("signal/ref = %q/%q", rec.SignalID, rec.ReferencePriceSource)
	}
	if !rec.MinStopFloorApplied || rec.MinSLMovePct != 4.0 || rec.BestTargetLabel != "tp1" || rec.BestTargetPrice != 104.5 {
		t.Fatalf("unexpected floor/target terms: %+v", rec)
	}
	if rec.ResolvedMinRR != 1.2 || rec.ResolvedMinSLMovePct != 4.0 || rec.MinRRSource != hunterV7RiskSourceStrategy || rec.MinSLSource != hunterV7RiskSourceStrategy {
		t.Fatalf("unexpected resolved geometry: %+v", rec)
	}
}

// (b) Passing candidates produce NO persisted record (at assembly or prompt-final).
func TestHunterV7RRVerdict_PassingProducesNoRecord(t *testing.T) {
	st := newRRVerdictTestStore(t)
	cfg := &store.StrategyConfig{
		Language:   "en",
		CoinSource: store.CoinSourceConfig{SourceType: "hunter_v7"},
	}
	engine := NewStrategyEngine(cfg)
	engine.v7CycleCounter = 3
	attachRRVerdictStore(t, engine, st)

	sig := local.V7SignalOutput{
		SignalID:       "3|PASSUSDT|LONG|trend_breakout_long",
		Symbol:         "PASSUSDT",
		Direction:      local.V7DirLong,
		SetupType:      local.V7SetupTrendBreakoutLong,
		Status:         "candidate",
		RiskLevel:      "MEDIUM",
		MarketRegime:   local.V7RegimeTrendUp,
		AIPriority:     75,
		SetupScore:     75,
		TimingScore:    70,
		LiquidityScore: 70,
		RiskScore:      20,
		PriceCtx:       &local.V7PriceContext{Last: 100},
		Invalidation:   local.V7InvalidationRule{Price: 98.0},
		TP1Price:       103.25,
		Targets:        []local.V7Target{{Price: 103.25}},
	}

	cands := engine.hunterV7SignalsToCandidateCoins([]local.V7SignalOutput{sig}, "BOTH")
	if len(cands) != 1 || cands[0].V7TierReason == "backend_rr_infeasible" || cands[0].V7RRVerdict != nil {
		t.Fatalf("passing candidate must not fail RR or carry V7RRVerdict: tier=%s reason=%s verdict=%+v",
			cands[0].V7ExecutionTier, cands[0].V7TierReason, cands[0].V7RRVerdict)
	}

	ctx := &Context{
		CallCount:      3,
		PromptTime:     time.Date(2026, 11, 28, 12, 5, 0, 0, time.UTC),
		CandidateCoins: cands,
		MarketDataMap:  map[string]*market.Data{"PASSUSDT": hunterV7PromptTestMarketData("PASSUSDT", 100)},
	}
	engine.FinalizeHunterV7Tiers(ctx)

	rows, err := st.QueryRRVerdicts(10)
	if err != nil {
		t.Fatalf("QueryRRVerdicts: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("passing candidate persisted %d records, want 0: %+v", len(rows), rows)
	}
}

// Prompt-final live repricing (#130): a T2 candidate passes at assembly (zone
// edge reference) with 0 records, then fails at FinalizeHunterV7Tiers on live
// price and persists exactly 1 prompt_final record (idempotent across calls).
func TestHunterV7RRVerdict_PromptFinalT2LiveRepriceAndIdempotency(t *testing.T) {
	st := newRRVerdictTestStore(t)
	cfg := &store.StrategyConfig{
		Language: "en",
		CoinSource: store.CoinSourceConfig{
			SourceType: "hunter_v7",
			Hunter: &store.HunterConfig{
				V7T2Enabled:        true,
				V7ConditionalEntry: &store.V7ConditionalEntryConfig{Enabled: true},
			},
		},
	}
	cfg.RiskControl.MinRiskRewardRatio = 1.2
	engine := NewStrategyEngine(cfg)
	engine.v7CycleCounter = 9
	attachRRVerdictStore(t, engine, st)

	sig := condSig("ignition_early_long", local.V7DirLong, "TRIGGERED")
	sig.SignalID = "9|AUSDT|LONG|ignition_early_long"
	cands := engine.hunterV7SignalsToCandidateCoins([]local.V7SignalOutput{sig}, "BOTH")
	if len(cands) != 1 || cands[0].V7ExecutionTier != "REVIEWABLE" {
		t.Fatalf("fixture: expected REVIEWABLE at assembly, got %+v", cands)
	}
	rows, err := st.QueryRRVerdicts(10)
	if err != nil || len(rows) != 0 {
		t.Fatalf("assembly pass must persist 0 rows, got %d (%v)", len(rows), err)
	}

	promptAt := time.Date(2026, 11, 28, 13, 0, 0, 0, time.UTC)
	ctx := &Context{
		CallCount:      9,
		PromptTime:     promptAt,
		CandidateCoins: cands,
		MarketDataMap:  map[string]*market.Data{"AUSDT": hunterV7PromptTestMarketData("AUSDT", 101)},
	}
	engine.FinalizeHunterV7Tiers(ctx)
	engine.FinalizeHunterV7Tiers(ctx) // idempotent: must not double-record

	rows, err = st.QueryRRVerdicts(10)
	if err != nil {
		t.Fatalf("QueryRRVerdicts: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 prompt_final record, got %d", len(rows))
	}
	rec := rows[0]
	assertRRVerdictInternallyConsistent(t, rec)
	if rec.Stage != HunterV7RRVerdictStagePromptFinal || rec.CycleNumber != 9 || !rec.Timestamp.Equal(promptAt) {
		t.Fatalf("stage/cycle/ts = %q/%d/%v", rec.Stage, rec.CycleNumber, rec.Timestamp)
	}
	if rec.Reason != "backend_rr_infeasible" || rec.ReferencePrice != 101 || rec.ReferencePriceSource != hunterV7RefSourcePriceContextLast {
		t.Fatalf("unexpected prompt_final terms: %+v", rec)
	}
	if rec.Stack != local.V7StackT2 || rec.MinRRSource != hunterV7RiskSourceT2Config || rec.MinSLSource != hunterV7RiskSourceT2Config {
		t.Fatalf("unexpected T2 resolution attribution: stack=%q minRRSource=%q minSLSource=%q", rec.Stack, rec.MinRRSource, rec.MinSLSource)
	}
}

// Conditional T2 candidate failing at its trigger price reports
// reference_price_source = "rr_reference_price".
func TestHunterV7RRVerdict_ReferencePriceSourceRRReferencePrice(t *testing.T) {
	st := newRRVerdictTestStore(t)
	cfg := &store.StrategyConfig{
		Language: "en",
		CoinSource: store.CoinSourceConfig{
			SourceType: "hunter_v7",
			Hunter: &store.HunterConfig{
				V7T2Enabled:        true,
				V7ConditionalEntry: &store.V7ConditionalEntryConfig{Enabled: true},
			},
		},
	}
	cfg.RiskControl.MinRiskRewardRatio = 1.2
	engine := NewStrategyEngine(cfg)
	attachRRVerdictStore(t, engine, st)

	c := AssembleHunterV7CandidateCoins([]local.V7SignalOutput{condSig("ignition_early_long", local.V7DirLong, "WATCH")}, "BOTH", engine.hunterV7ExecutionGeometry())[0]
	c.V7TriggerPrice = 102 // at trigger 102, stop 99, TP2 103 -> RR = 1/3 < 1.2
	ctx := &Context{
		CallCount:      4,
		CandidateCoins: []CandidateCoin{c},
		MarketDataMap:  map[string]*market.Data{"AUSDT": hunterV7PromptTestMarketData("AUSDT", 101.5)},
	}
	engine.FinalizeHunterV7Tiers(ctx)

	rows, err := st.QueryRRVerdicts(10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %d (err=%v), want 1", len(rows), err)
	}
	rec := rows[0]
	assertRRVerdictInternallyConsistent(t, rec)
	if rec.ReferencePrice != 102 || rec.ReferencePriceSource != hunterV7RefSourceRRReferencePrice {
		t.Fatalf("ref = %v (%q), want 102 (rr_reference_price)", rec.ReferencePrice, rec.ReferencePriceSource)
	}
}

// Sibling reasons rr_below_floor and rr_unknown also persist exactly one
// internally consistent record each.
func TestHunterV7RRVerdict_SiblingReasonsBelowFloorAndUnknown(t *testing.T) {
	st := newRRVerdictTestStore(t)
	cfg := &store.StrategyConfig{
		Language:   "en",
		CoinSource: store.CoinSourceConfig{SourceType: "hunter_v7"},
	}
	cfg.RiskControl.MinRiskRewardRatio = 1.2
	cfg.RiskControl.MaxTakeProfitPriceMovePct = 30
	engine := NewStrategyEngine(cfg)
	attachRRVerdictStore(t, engine, st)

	below := rrAdmissionCoin("RRBELOWUSDT", 1.19)
	unknown := rrAdmissionCoin("RRUNKUSDT", 1.5)
	unknown.V7Targets = nil

	ctx := &Context{
		CallCount:      5,
		CandidateCoins: []CandidateCoin{below, unknown},
		MarketDataMap: map[string]*market.Data{
			"RRBELOWUSDT": hunterV7PromptTestMarketData("RRBELOWUSDT", 100.5),
			"RRUNKUSDT":   hunterV7PromptTestMarketData("RRUNKUSDT", 100.5),
		},
	}
	engine.FinalizeHunterV7Tiers(ctx)

	rows, err := st.QueryRRVerdictsByCycle(5)
	if err != nil {
		t.Fatalf("QueryRRVerdictsByCycle: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("persisted rows = %d, want 2: %+v", len(rows), rows)
	}
	byReason := map[string]store.HunterV7RRVerdictRecord{}
	for _, r := range rows {
		assertRRVerdictInternallyConsistent(t, r)
		byReason[r.Reason] = r
	}
	bf, ok := byReason[hunterV7RRBelowFloorReason]
	if !ok || !bf.OK || bf.Symbol != "RRBELOWUSDT" || bf.MinRRFloor != 1.2 {
		t.Fatalf("rr_below_floor record mismatch: %+v", bf)
	}
	unk, ok := byReason[hunterV7RRUnknownReason]
	if !ok || unk.OK || unk.Symbol != "RRUNKUSDT" || unk.BestTargetIndex != -1 || unk.MinRRFloor != 1.2 {
		t.Fatalf("rr_unknown record mismatch: %+v", unk)
	}
}

// Geometry resolution attribution records class_profile and setup_override
// layers so downstream analysis can tell which layer set each threshold.
func TestHunterV7RRVerdict_GeometryResolutionSources(t *testing.T) {
	st := newRRVerdictTestStore(t)
	cfg := &store.StrategyConfig{
		Language:   "en",
		CoinSource: store.CoinSourceConfig{SourceType: "hunter_v7"},
		RiskControl: store.RiskControlConfig{
			MinRiskRewardRatio: 1.5,
			ClassProfiles: map[string]store.V7ClassRiskProfile{
				store.V7ClassImpulse: {
					Native: store.V7StackRiskProfile{
						MinRiskRewardRatio:      1.8,
						MinStopLossPriceMovePct: 2.5,
					},
				},
			},
			SetupGeometryOverrides: map[string]store.V7SetupGeometryOverride{
				"displacement_momentum_long": {
					MinRRCap:      1.6,
					MaxTPFloorPct: 12.0,
				},
			},
		},
	}
	engine := NewStrategyEngine(cfg)
	attachRRVerdictStore(t, engine, st)

	// displacement_momentum_long is IMPULSE class:
	// - MinRR: strategy 1.5 -> class_profile 1.8 -> setup_override cap 1.6
	// - MinSL: strategy 2.0 -> class_profile 2.5
	// - MaxTP: strategy -> setup_override floor 12.0
	// With price 100, stop 97 (raw risk 3.0% >= 2.5%), TP1 104 (reward 4.0%):
	// RR = 4.0 / 3.0 = 1.333 < 1.6 -> backend_rr_infeasible.
	sig := local.V7SignalOutput{
		SignalID:       "1|DISPUSDT|LONG|displacement_momentum_long",
		Symbol:         "DISPUSDT",
		Direction:      local.V7DirLong,
		SetupType:      local.V7SetupType("displacement_momentum_long"),
		Status:         "candidate",
		RiskLevel:      "MEDIUM",
		MarketRegime:   local.V7RegimeTrendUp,
		AIPriority:     70,
		SetupScore:     70,
		TimingScore:    65,
		LiquidityScore: 70,
		RiskScore:      20,
		PriceCtx:       &local.V7PriceContext{Last: 100},
		Invalidation:   local.V7InvalidationRule{Price: 97},
		TP1Price:       104,
		Targets:        []local.V7Target{{Price: 104}},
	}
	cands := engine.hunterV7SignalsToCandidateCoins([]local.V7SignalOutput{sig}, "BOTH")
	if len(cands) != 1 || cands[0].V7TierReason != "backend_rr_infeasible" {
		t.Fatalf("expected backend_rr_infeasible, got %+v", cands)
	}
	rows, err := st.QueryRRVerdicts(10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %d (%v), want 1", len(rows), err)
	}
	rec := rows[0]
	assertRRVerdictInternallyConsistent(t, rec)
	if rec.ResolvedMinRR != 1.6 || rec.MinRRSource != hunterV7RiskSourceSetupOverride {
		t.Fatalf("MinRR = %v (%q), want 1.6 (setup_override)", rec.ResolvedMinRR, rec.MinRRSource)
	}
	if rec.ResolvedMinSLMovePct != 2.5 || rec.MinSLSource != hunterV7RiskSourceClassProfile {
		t.Fatalf("MinSL = %v (%q), want 2.5 (class_profile)", rec.ResolvedMinSLMovePct, rec.MinSLSource)
	}
	if rec.ResolvedMaxTPMovePct != 12.0 || rec.MaxTPSource != hunterV7RiskSourceSetupOverride {
		t.Fatalf("MaxTP = %v (%q), want 12.0 (setup_override)", rec.ResolvedMaxTPMovePct, rec.MaxTPSource)
	}
}

// Target cap binding: verify best_target_capped and per-target pre/post cap
// terms when a high target is capped by MaxTPMovePct and causes infeasibility.
func TestHunterV7RRVerdict_TargetCapBinding(t *testing.T) {
	st := newRRVerdictTestStore(t)
	cfg := &store.StrategyConfig{
		Language:   "en",
		CoinSource: store.CoinSourceConfig{SourceType: "hunter_v7"},
	}
	engine := NewStrategyEngine(cfg)
	attachRRVerdictStore(t, engine, st)

	// Default native geometry: MinRR=1.2, MinSL=2.0, MaxDrift=0.5 -> MaxTP=3.25%.
	// Stop at 97.0 (raw risk 3.0% >= 2.0%), targets at 102.0 (2%) and 109.0 (9% raw, capped to 3.25%).
	// Capped RR = 3.25 / 3.0 = 1.0833 < 1.2 -> backend_rr_infeasible despite raw RR = 3.0!
	sig := local.V7SignalOutput{
		SignalID:       "1|CAPUSDT|LONG|trend_breakout_long",
		Symbol:         "CAPUSDT",
		Direction:      local.V7DirLong,
		SetupType:      local.V7SetupTrendBreakoutLong,
		Status:         "candidate",
		RiskLevel:      "MEDIUM",
		MarketRegime:   local.V7RegimeTrendUp,
		AIPriority:     72,
		SetupScore:     70,
		TimingScore:    65,
		LiquidityScore: 70,
		RiskScore:      20,
		PriceCtx:       &local.V7PriceContext{Last: 100},
		Invalidation:   local.V7InvalidationRule{Price: 97},
		TP0Price:       102,
		TP1Price:       109,
		Targets:        []local.V7Target{{Price: 102}, {Price: 109}},
	}
	cands := engine.hunterV7SignalsToCandidateCoins([]local.V7SignalOutput{sig}, "BOTH")
	if len(cands) != 1 || cands[0].V7TierReason != "backend_rr_infeasible" {
		t.Fatalf("expected backend_rr_infeasible, got %+v", cands)
	}
	rows, err := st.QueryRRVerdicts(10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %d (%v), want 1", len(rows), err)
	}
	rec := rows[0]
	assertRRVerdictInternallyConsistent(t, rec)
	if !rec.BestTargetCapped || rec.BestTargetLabel != "tp1" || rec.BestTargetIndex != 1 {
		t.Fatalf("expected capped tp1, got %+v", rec)
	}
	if math.Abs(rec.BestTargetRawRewardPct-9.0) > 1e-9 || math.Abs(rec.BestRewardPct-3.25) > 1e-9 || math.Abs(rec.MaxTPMovePct-3.25) > 1e-9 {
		t.Fatalf("cap terms mismatch: raw=%v best=%v cap=%v", rec.BestTargetRawRewardPct, rec.BestRewardPct, rec.MaxTPMovePct)
	}
}

// Candidates whose final tier reason is NOT an RR reason (even when internal
// probes like #161 single-missing-confirmation call hunterV7BackendCappedRR)
// must produce NO persisted RR verdict record.
func TestHunterV7RRVerdict_NonRRReasonWithProbeProducesNoRecord(t *testing.T) {
	st := newRRVerdictTestStore(t)
	cfg := &store.StrategyConfig{
		Language:    "en",
		CoinSource:  store.CoinSourceConfig{SourceType: "hunter_v7"},
		RiskControl: store.RiskControlConfig{MinStopLossPriceMovePct: 4.0},
	}
	engine := NewStrategyEngine(cfg)
	attachRRVerdictStore(t, engine, st)

	sig := local.V7SignalOutput{
		SignalID:         "1|PROBEUSDT|LONG|trend_breakout_long",
		Symbol:           "PROBEUSDT",
		Direction:        local.V7DirLong,
		SetupType:        local.V7SetupTrendBreakoutLong,
		Status:           "candidate",
		RiskLevel:        "MEDIUM",
		MarketRegime:     local.V7RegimeTrendUp,
		AIPriority:       70,
		SetupScore:       70,
		TimingScore:      65,
		LiquidityScore:   70,
		RiskScore:        20,
		RequiredConfirms: []string{"taker_flow_confirms_long"},
		ConfirmSummary: &local.V7ConfirmationSummary{
			PassedHard:    true,
			MissingReview: []local.V7ConfirmationCheck{{Code: "taker_flow_confirms_long", Severity: local.V7ConfirmReviewWait}},
		},
		PriceCtx:     &local.V7PriceContext{Last: 100},
		Invalidation: local.V7InvalidationRule{Price: 98.8},
		Targets:      []local.V7Target{{Price: 104.5}},
	}
	cands := engine.hunterV7SignalsToCandidateCoins([]local.V7SignalOutput{sig}, "BOTH")
	if len(cands) != 1 || cands[0].V7ExecutionTier != "WATCH" || cands[0].V7TierReason != "confirmation_missing_taker_flow_confirms_long" {
		t.Fatalf("expected confirmation_missing WATCH, got tier=%s reason=%s", cands[0].V7ExecutionTier, cands[0].V7TierReason)
	}
	rows, err := st.QueryRRVerdicts(10)
	if err != nil || len(rows) != 0 {
		t.Fatalf("non-RR reason must persist 0 records, got %d (%v)", len(rows), err)
	}
}

