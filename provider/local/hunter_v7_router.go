package local

import (
	"fmt"
	"log"
	"sort"
	"strings"
)

// ============================================================================
// Hunter v7 — Signal Router
// ============================================================================
// The router is the core dispatcher: it runs all signal modules against all
// symbols, applies regime weights, computes final scores, filters, and outputs
// a sorted list of actionable signals.

// V7Router dispatches symbols through all registered signal modules.
type V7Router struct {
	modules []V7SignalModule
}

// NewV7Router creates a router with all 10 signal modules registered.
func NewV7Router() *V7Router {
	r := &V7Router{}
	r.modules = []V7SignalModule{
		&pullbackLongModule{},
		&shortSqueezeLongModule{},
		&trendBreakoutLongModule{},
		&leaderMomentumLongModule{},
		&panicReversalLongModule{},
		&accumulationBreakoutLongModule{},
		&distributionShortModule{},
		&squeezeShortModule{},
		&breakdownMomentumShortModule{},
		&rangeReversionModule{},
		&fundingReversalModule{},
		&displacementMomentumLongModule{},
		&rangeExpansionEventModule{},
		// v8 new modules (Phase 2)
		&intradayScalpLongModule{},
		&volatilitySqueezeBreakoutModule{},
		&whaleFlowReversalModule{},
		&altLadderMomentumLongModule{},
		&altLadderBreakdownShortModule{},
		&mmsBottomWakeLongModule{},
		&mmsTrendRideLongModule{},
		&mmsSqueezeEngineLongModule{},
	}
	return r
}

// Route runs all modules against the universe and returns sorted LLM-facing signals.
func (r *V7Router) Route(universe []V7SymbolContext, regime V7MarketRegime, cfg V7Config) []V7SignalOutput {
	return r.RouteDetailed(universe, regime, cfg).OutputSignals
}

// RouteDetailed runs all modules and returns both raw and LLM-facing signals.
// v7SignalPass is one named enhancement stage applied to every signal a
// module emits. RouteDetailed's per-signal sequence is the pass slice below —
// order is data, and adding a stage means adding a row (U6.5).
type v7SignalPass struct {
	name  string
	apply func(sig *V7SignalOutput, ctx *V7SymbolContext)
}

// buildV7SignalPasses assembles the per-cycle pipeline. Cycle-scoped
// components (boosters, sector rotation, BTC/ETH baseline) are captured once
// here, exactly as the former inline sequence created them per route call.
func buildV7SignalPasses(universe []V7SymbolContext, regime V7MarketRegime, cfg V7Config) []v7SignalPass {
	btcETHBaseline4h := computeBTCETHBaseline4h(universe)
	timingBooster := DefaultTimingBooster()
	panicOverride := DefaultPanicWeightOverride()
	fundingFastTrack := DefaultFundingFastTrack()
	resonanceScorer := DefaultResonanceScorer()
	sectorRotation := NewSectorRotationAnalyzer(universe)

	return []v7SignalPass{
		{"attach_context", func(sig *V7SignalOutput, ctx *V7SymbolContext) {
			sig.ExecutionContext = ctx.ExecutionContext
			sig.DataFreshness = ctx.DataFreshness
			// Propagate quote volume for adaptive OI threshold in prompt-data filter
			if ctx.Snapshot != nil {
				sig.QuoteVolume24h = ctx.Snapshot.QuoteVolume24h
			}
			if len(sig.RequiredConfirms) == 0 {
				sig.RequiredConfirms = defaultV7Confirmations(sig)
			}
		}},
		{"regime_weight", func(sig *V7SignalOutput, ctx *V7SymbolContext) {
			weight := regimeModuleWeight(regime, sig.SetupType)
			sig.SetupScore *= weight
			sig.SetupScore = clampFloat(sig.SetupScore, 0, 100)
			sig.RegimeFitScore = weight * 67 // normalize to 0-100 (1.5 * 67 ≈ 100)
		}},
		// Liquidity must precede the strong-symbol override: the override
		// gate reads it and would otherwise see the zero value.
		{"liquidity", func(sig *V7SignalOutput, ctx *V7SymbolContext) {
			sig.LiquidityScore = AssessLiquidityScore(ctx)
		}},
		// Strong-symbol override: if this symbol significantly outperforms
		// BTC/ETH on 4h, prevent regime weight from suppressing it below 0.8
		{"strong_symbol_override", func(sig *V7SignalOutput, ctx *V7SymbolContext) {
			weight := regimeModuleWeight(regime, sig.SetupType)
			if weight < 0.8 && ctx.Symbol != "BTCUSDT" && ctx.Symbol != "ETHUSDT" {
				symbolRS := ctx.Change4h - btcETHBaseline4h
				if symbolRS > 6 && sig.LiquidityScore >= 50 && ctx.TakerBuy15m >= 0.50 {
					// Re-apply with floor of 0.8
					sig.SetupScore = clampFloat(sig.SetupScore/weight*0.8, 0, 100)
					sig.RegimeFitScore = 0.8 * 67
					sig.ReasonCodes = append(sig.ReasonCodes, "strong_symbol_regime_override")
				}
			}
		}},
		{"risk", func(sig *V7SignalOutput, ctx *V7SymbolContext) {
			sig.RiskScore = AssessV7Risk(sig, ctx)
			sig.RiskLevel = ClassifyV7RiskLevel(sig.RiskScore)
		}},
		{"multi_tp", ApplyMultiTimeframeTP},
		{"timing_booster", func(sig *V7SignalOutput, ctx *V7SymbolContext) {
			timingBooster.EnhanceTiming(sig, ctx)
		}},
		{"funding_fasttrack", func(sig *V7SignalOutput, ctx *V7SymbolContext) {
			if fundingFastTrack.ShouldFastTrack(sig, ctx) {
				fundingFastTrack.ApplyFastTrack(sig)
			}
		}},
		{"sector_rotation", func(sig *V7SignalOutput, ctx *V7SymbolContext) {
			sectorRotation.EnhanceSignal(sig, ctx, regime)
		}},
		{"resonance", func(sig *V7SignalOutput, ctx *V7SymbolContext) {
			resonanceScorer.ApplyToSignal(sig)
		}},
		// Translate post-enhancement scores into executable signal quality.
		// This must run after TP/timing/fast-track/resonance adjustments so
		// readiness tiers and blocked gates reflect the final signal.
		{"finalize_execution", func(sig *V7SignalOutput, ctx *V7SymbolContext) {
			finalizeV7SignalForExecution(sig, ctx, cfg)
		}},
		{"freshness_risk", applyV7FreshnessRisk},
		// AI priority: the composite ranking score, the panic-combo timing
		// override and the resonance bonus — the former three scattered
		// writes — land in one pass.
		{"ai_priority", func(sig *V7SignalOutput, ctx *V7SymbolContext) {
			sig.AIPriority = CalcAIPriority(sig, cfg)
			sig.AIPriority = panicOverride.AdjustAIPriority(sig.AIPriority, sig.TimingScore, sig.MarketRegime, sig.SetupType)
			sig.AIPriority = clampFloat(sig.AIPriority+sig.ResonanceBonus, 0, 100)
		}},
		{"risk_filter", func(sig *V7SignalOutput, ctx *V7SymbolContext) {
			if sig.RiskScore >= 90 {
				sig.Status = V7StatusFiltered
				sig.RiskTags = append(sig.RiskTags, "risk_filtered")
			}
		}},
		{"liquidity_filter", func(sig *V7SignalOutput, ctx *V7SymbolContext) {
			if sig.LiquidityScore < 30 {
				sig.Status = V7StatusFiltered
				sig.RiskTags = append(sig.RiskTags, "liquidity_filtered")
			}
		}},
		{"readiness_refresh", refreshV7ExecutionReadiness},
	}
}

func (r *V7Router) RouteDetailed(universe []V7SymbolContext, regime V7MarketRegime, cfg V7Config) V7RouteResult {
	passes := buildV7SignalPasses(universe, regime, cfg)

	var allSignals []V7SignalOutput

	// Regime-aware breaker: decide once per module per cycle which
	// regime-disfavoured modules sit this cycle out (see hunter_v7_module_breaker.go).
	skippedModules := make(map[V7SetupType]bool)
	if cfg.WatchStateManager != nil {
		for _, mod := range r.modules {
			setup := mod.SetupType()
			if regimeModuleWeight(regime, setup) >= v7BreakerDisfavourWeight {
				continue
			}
			if cfg.WatchStateManager.ShouldSkipModule(regime, setup) {
				skippedModules[setup] = true
			}
		}
		if len(skippedModules) > 0 {
			names := make([]string, 0, len(skippedModules))
			for setup := range skippedModules {
				names = append(names, string(setup))
			}
			sort.Strings(names)
			log.Printf("⛔ Hunter v7 breaker: skipping dry disfavoured modules this cycle: %s", strings.Join(names, ","))
		}
	}

	for i := range universe {
		ctx := &universe[i]
		for _, mod := range r.modules {
			// Regime weight gate: skip modules with very low weight
			weight := regimeModuleWeight(regime, mod.SetupType())
			if weight < 0.2 {
				continue
			}
			if skippedModules[mod.SetupType()] {
				continue
			}

			// Fast pre-filter
			if !mod.Match(ctx, regime) {
				continue
			}

			// Full scoring
			sig := mod.Score(ctx, regime)
			if sig == nil {
				continue
			}
			for i := range passes {
				passes[i].apply(sig, ctx)
			}

			allSignals = append(allSignals, *sig)
		}
	}

	// Feed the breaker with this cycle's raw module output before any
	// filtering, so dry-spell counts reflect what modules actually produced.
	if cfg.WatchStateManager != nil {
		cfg.WatchStateManager.RecordModuleConversions(regime, allSignals)
	}

	// Two append idioms coexist in the modules (raw append vs appendIfMissing),
	// so a signal can accumulate duplicate tags. Normalize here — one place —
	// so every consumer (prompt JSON, store, validate) sees each tag once.
	for i := range allSignals {
		allSignals[i].ReasonCodes = dedupeV7Tags(allSignals[i].ReasonCodes)
		allSignals[i].RiskTags = dedupeV7Tags(allSignals[i].RiskTags)
		allSignals[i].RequiredConfirms = dedupeV7Tags(allSignals[i].RequiredConfirms)
	}

	// Resolve conflicts (same symbol, opposite directions)
	allSignals = ResolveV7Conflicts(allSignals)

	confirmed := filterV7SignalsForLLM(allSignals, cfg)
	confirmed = applyV7CorrelationFilter(confirmed, cfg)
	watches := BuildV7PreMoveRadar(universe, regime, cfg)
	out := appendV7WatchSignals(confirmed, watches, cfg)
	raw := append([]V7SignalOutput{}, allSignals...)
	raw = append(raw, watches...)
	raw = append(raw, buildV7ModuleNoMatchSignals(universe, raw, regime)...)
	potentialPool := BuildV7PotentialPool(universe, raw, defaultV7PotentialPoolLimit)
	logV7RouteDiagnostics(allSignals, confirmed, watches, out, cfg)
	return V7RouteResult{
		RawSignals:       raw,
		ConfirmedSignals: confirmed,
		WatchSignals:     watches,
		OutputSignals:    out,
		PotentialPool:    potentialPool,
	}
}

// v7SignalDataIsStale decides whether a signal rests on data too old to trade.
// Kline close time is real market time and is unaffected by how long our own
// snapshot took, so it is the primary evidence; price/snapshot age fall back to
// a cadence-aware threshold.
func v7SignalDataIsStale(fresh V7DataFreshness) bool {
	threshold := fresh.StaleThresholdMs
	if threshold <= 0 {
		threshold = 45_000
	}
	// A live 1m feed always has an in-progress candle, which latestKlineAgeMs
	// reports as 0. A non-zero age means the newest candle has already closed
	// and nothing arrived after it.
	if fresh.Kline1mAgeMs > 120_000 {
		return true
	}
	if fresh.PriceAgeMs > threshold {
		return true
	}
	return fresh.SnapshotAgeMs > threshold+15_000
}

func applyV7FreshnessRisk(sig *V7SignalOutput, ctx *V7SymbolContext) {
	if sig == nil || ctx == nil {
		return
	}
	fresh := ctx.DataFreshness
	if !v7SignalDataIsStale(fresh) {
		return
	}
	sig.RiskTags = appendIfMissing(sig.RiskTags, "stale_data_risk")
	if containsV7String(sig.RiskTags, "high_volatility") || sig.SetupType == V7SetupRangeExpansion {
		sig.ExecutionQuality = V7ExecWatchOnly
		sig.Status = V7StatusWaitConfirm
		if sig.ExecutionReadiness != nil && sig.ExecutionReadiness.Tier == V7ReadinessExecutable {
			sig.ExecutionReadiness.Tier = V7ReadinessReviewable
			sig.ExecutionReadiness.BlockedGate = "stale_data_risk"
			sig.ExecutionReadiness.Reason = "stale high-volatility signal requires fresh micro confirmation"
			sig.ExecutionReadiness.NextConfirm = appendIfMissing(sig.ExecutionReadiness.NextConfirm, "fresh_micro_confirmed")
		}
		sig.RequiredConfirms = appendIfMissing(sig.RequiredConfirms, "fresh_micro_confirmed")
	}
}

func refreshV7ExecutionReadiness(sig *V7SignalOutput, ctx *V7SymbolContext) {
	if sig == nil || ctx == nil {
		return
	}
	readiness := CalculateV7ExecutionReadiness(sig, ctx)
	sig.ExecutionReadiness = &readiness
}

func buildV7ModuleNoMatchSignals(universe []V7SymbolContext, existing []V7SignalOutput, regime V7MarketRegime) []V7SignalOutput {
	if len(universe) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(existing))
	for _, sig := range existing {
		if sig.Symbol != "" {
			seen[sig.Symbol] = true
		}
	}
	var out []V7SignalOutput
	for i := range universe {
		ctx := &universe[i]
		if ctx.Symbol == "" || seen[ctx.Symbol] {
			continue
		}
		sig := V7SignalOutput{
			Symbol:           ctx.Symbol,
			SetupType:        V7SetupModuleNoMatch,
			Status:           V7StatusFiltered,
			ExecutionQuality: V7ExecWatchOnly,
			MarketShape:      V7ShapeNoiseNoTrade,
			EntrySignal:      V7EntrySignalNoTrade,
			MarketRegime:     regime,
			LiquidityScore:   AssessLiquidityScore(ctx),
			PriceCtx:         buildPriceCtx(ctx),
			DerivativesCtx:   buildDerivCtx(ctx),
			ExecutionContext: ctx.ExecutionContext,
		}
		noMatchReason := classifyV7ModuleNoMatchReason(ctx)
		sig.ReasonCodes = []string{"no_setup_matched", noMatchReason, string(V7ShapeNoiseNoTrade), string(V7EntrySignalNoTrade)}
		sig.RiskTags = []string{"module_no_match"}
		if ctx.Snapshot != nil {
			sig.QuoteVolume24h = ctx.Snapshot.QuoteVolume24h
		}
		out = append(out, sig)
	}
	return out
}

func classifyV7ModuleNoMatchReason(ctx *V7SymbolContext) string {
	if ctx == nil {
		return "no_match_unknown"
	}
	liq := AssessLiquidityScore(ctx)
	if liq > 0 && liq < 50 {
		return "no_match_low_liquidity"
	}
	if ctx.Amplitude24h > 0 && ctx.Amplitude24h < 3 && ctx.RangeExpansion1h < 1.1 {
		return "no_match_low_amplitude"
	}
	if mathAbs(ctx.Velocity15m) > 4 || mathAbs(ctx.Change1h) > 6 {
		return "no_match_late_move"
	}
	if ctx.BBWidthPercentile > 0 && ctx.BBWidthPercentile < 1.25 && ctx.Snapshot != nil && ctx.Snapshot.OIDelta1h > 0 {
		return "no_match_pre_breakout_candidate"
	}
	if ctx.Snapshot != nil && mathAbs(ctx.Snapshot.OIDelta1h) >= 8 && mathAbs(ctx.Change1h) < 2 {
		return "no_match_oi_flow_candidate"
	}
	if ctx.Snapshot != nil && ((ctx.Snapshot.OIDelta1h > 3 && ctx.Change1h < -1) || (ctx.Snapshot.OIDelta1h < -3 && ctx.Change1h > 1)) {
		return "no_match_conflict_price_oi"
	}
	return "no_match_mid_range_noise"
}

func mathAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// CalcAIPriority computes the composite AI priority score.
// Formula: setup×0.35 + timing×0.20 + regime_fit×0.20 + liquidity×0.15 - risk×0.10
func CalcAIPriority(sig *V7SignalOutput, cfg V7Config) float64 {
	var base float64
	if cfg.Aggressive {
		// Aggressive: more weight on setup and timing
		base = sig.SetupScore*0.40 +
			sig.TimingScore*0.25 +
			sig.RegimeFitScore*0.15 +
			sig.LiquidityScore*0.10 -
			sig.RiskScore*0.10
	} else {
		// Balanced (default)
		base = sig.SetupScore*0.35 +
			sig.TimingScore*0.20 +
			sig.RegimeFitScore*0.20 +
			sig.LiquidityScore*0.15 -
			sig.RiskScore*0.10
	}
	base += v7SetupExpectancyBonus(sig)
	base += v7ExecutionQualityBonus(sig.ExecutionQuality)
	return clampFloat(base, 0, 100)
}

func filterV7SignalsForLLM(signals []V7SignalOutput, cfg V7Config) []V7SignalOutput {
	minPriority := cfg.MinAIPriority
	if minPriority <= 0 {
		minPriority = 55
	}
	fallbackMinPriority := cfg.FallbackMinAIPriority
	if fallbackMinPriority <= 0 || fallbackMinPriority >= minPriority {
		fallbackMinPriority = minPriority - 10
	}
	if fallbackMinPriority < 0 {
		fallbackMinPriority = 0
	}
	minOutput := cfg.MinOutput
	if minOutput <= 0 {
		minOutput = 3
	}
	maxOutput := cfg.MaxOutput
	if maxOutput > 0 && minOutput > maxOutput {
		minOutput = maxOutput
	}

	eligible := make([]V7SignalOutput, 0, len(signals))
	for _, sig := range signals {
		if sig.Status == V7StatusFiltered {
			continue
		}
		eligible = append(eligible, sig)
	}
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].AIPriority > eligible[j].AIPriority
	})

	filtered := make([]V7SignalOutput, 0, len(eligible))
	used := make([]bool, len(eligible))
	for i, sig := range eligible {
		if sig.AIPriority >= minPriority || sig.Status == V7StatusConflictWatch {
			filtered = append(filtered, sig)
			used[i] = true
		}
	}

	if len(filtered) < minOutput {
		for i, sig := range eligible {
			if len(filtered) >= minOutput {
				break
			}
			if used[i] || sig.AIPriority < fallbackMinPriority {
				continue
			}
			if sig.Status == V7StatusCandidate {
				sig.Status = V7StatusWaitConfirm
			}
			sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "candidate_floor_context")
			sig.RiskTags = appendIfMissing(sig.RiskTags, "context_only_low_priority")
			filtered = append(filtered, sig)
			used[i] = true
		}
	}

	if !hasV7OpenReviewSignal(filtered) {
		filtered = appendV7ReviewableFloorSignals(filtered, eligible, used, cfg, minOutput)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].AIPriority > filtered[j].AIPriority
	})
	if maxOutput > 0 && len(filtered) > maxOutput {
		filtered = diversifyV7Signals(filtered, maxOutput)
	}
	return filtered
}

func applyV7CorrelationFilter(signals []V7SignalOutput, cfg V7Config) []V7SignalOutput {
	if len(signals) == 0 {
		return signals
	}
	maxPerTheme := cfg.CorrelationMaxPerTheme
	if maxPerTheme < 0 {
		return signals
	}
	if maxPerTheme == 0 {
		maxPerTheme = 3
	}
	minOutput := cfg.MinOutput
	if minOutput <= 0 {
		minOutput = 3
	}
	maxOutput := cfg.MaxOutput
	if maxOutput > 0 && minOutput > maxOutput {
		minOutput = maxOutput
	}

	filtered := NewCorrelationFilter(maxPerTheme).FilterByCorrelation(signals)
	if len(filtered) >= minOutput || len(filtered) >= len(signals) {
		return filtered
	}

	seen := make(map[string]struct{}, len(filtered))
	for _, sig := range filtered {
		seen[v7SignalIdentity(sig)] = struct{}{}
	}
	for _, sig := range signals {
		if len(filtered) >= minOutput {
			break
		}
		key := v7SignalIdentity(sig)
		if _, ok := seen[key]; ok {
			continue
		}
		sig.RiskTags = appendIfMissing(sig.RiskTags, "correlation_floor_context")
		filtered = append(filtered, sig)
		seen[key] = struct{}{}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].AIPriority > filtered[j].AIPriority
	})
	return filtered
}

func v7SignalIdentity(sig V7SignalOutput) string {
	return sig.Symbol + "\x00" + string(sig.SetupType) + "\x00" + string(sig.Direction)
}

func hasV7OpenReviewSignal(signals []V7SignalOutput) bool {
	for _, sig := range signals {
		switch sig.ExecutionQuality {
		case V7ExecReady, V7ExecNearConfirm:
			if sig.Status != V7StatusFiltered && sig.RiskScore < 65 {
				return true
			}
		}
	}
	return false
}

func appendV7ReviewableFloorSignals(filtered, eligible []V7SignalOutput, used []bool, cfg V7Config, minOutput int) []V7SignalOutput {
	if minOutput <= 0 {
		minOutput = 1
	}
	maxRescue := 1
	if cfg.Aggressive && minOutput >= 3 {
		maxRescue = 2
	}

	rescued := 0
	for i := range filtered {
		if rescued >= maxRescue {
			break
		}
		if !isV7ReviewableFloorCandidate(filtered[i]) {
			continue
		}
		filtered[i] = promoteV7ReviewableFloorSignal(filtered[i], cfg)
		rescued++
	}

	for i, sig := range eligible {
		if rescued >= maxRescue {
			break
		}
		if i < len(used) && used[i] {
			continue
		}
		if !isV7ReviewableFloorCandidate(sig) {
			continue
		}

		sig = promoteV7ReviewableFloorSignal(sig, cfg)
		filtered = append(filtered, sig)
		if i < len(used) {
			used[i] = true
		}
		rescued++
	}

	return filtered
}

func promoteV7ReviewableFloorSignal(sig V7SignalOutput, cfg V7Config) V7SignalOutput {
	sig.Status = V7StatusWaitConfirm
	sig.ExecutionQuality = V7ExecNearConfirm
	sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "reviewable_floor_rescue")
	sig.RiskTags = appendIfMissing(sig.RiskTags, "fallback_reviewable_needs_live_confirm")
	sig.AIPriority = CalcAIPriority(&sig, cfg)
	return sig
}

func isV7ReviewableFloorCandidate(sig V7SignalOutput) bool {
	if sig.Status == V7StatusFiltered {
		return false
	}
	switch sig.SetupType {
	case V7SetupPreBreakoutWatch, V7SetupPreSqueezeWatch, V7SetupPreDistribution, V7SetupAccumulationWatch:
		return false
	}
	switch sig.ExecutionQuality {
	case V7ExecInvalidRR, V7ExecChaseRisk, V7ExecReady, V7ExecNearConfirm:
		return false
	}
	if sig.RiskScore >= 55 {
		return false
	}
	if sig.LiquidityScore > 0 && sig.LiquidityScore < 50 {
		return false
	}
	if sig.AIPriority < 40 {
		return false
	}

	switch sig.SetupType {
	case V7SetupFundingReversal:
		if sig.Direction == V7DirShort {
			return sig.TimingScore >= 60
		}
		return sig.TimingScore >= 65 && sig.Confidence != "C"
	case V7SetupPanicReversalLong:
		if sig.SetupScore >= 65 &&
			sig.TimingScore >= 30 &&
			containsV7String(sig.ReasonCodes, "strong_reclaim") &&
			(containsV7String(sig.ReasonCodes, "selling_decelerating") ||
				containsV7String(sig.ReasonCodes, "selling_exhaustion")) {
			return true
		}
		return sig.SetupScore >= 65 &&
			sig.TimingScore >= 35 &&
			sig.RiskScore < 45 &&
			!strings.EqualFold(sig.Confidence, "C") &&
			(containsV7String(sig.ReasonCodes, "flow_taker_buy_strong") ||
				containsV7String(sig.ReasonCodes, "flow_taker_buy_aggressive") ||
				containsV7String(sig.ReasonCodes, "strong_reclaim"))
	case V7SetupPullbackLong, V7SetupDistributionShort, V7SetupRangeReversion:
		return sig.SetupScore >= 70 && sig.TimingScore >= 50
	default:
		return false
	}
}

func logV7RouteDiagnostics(raw, confirmed, watches, out []V7SignalOutput, cfg V7Config) {
	minPriority := cfg.MinAIPriority
	if minPriority <= 0 {
		minPriority = 55
	}
	fallbackMinPriority := cfg.FallbackMinAIPriority
	if fallbackMinPriority <= 0 || fallbackMinPriority >= minPriority {
		fallbackMinPriority = minPriority - 10
	}

	ready, nearConfirm, watchOnly, invalidRR, filtered := 0, 0, 0, 0, 0
	for _, sig := range raw {
		switch sig.ExecutionQuality {
		case V7ExecReady:
			ready++
		case V7ExecNearConfirm:
			nearConfirm++
		case V7ExecWatchOnly, V7ExecChaseRisk:
			watchOnly++
		case V7ExecInvalidRR:
			invalidRR++
		}
		if sig.Status == V7StatusFiltered {
			filtered++
		}
	}
	if hasV7ExecutableOutput(out, minPriority) {
		return
	}

	eligible := 0
	var top *V7SignalOutput
	for i := range raw {
		if raw[i].Status == V7StatusFiltered {
			continue
		}
		eligible++
		if top == nil || raw[i].AIPriority > top.AIPriority {
			top = &raw[i]
		}
	}

	topText := "none"
	if top != nil {
		topText = top.Symbol + " " + string(top.Direction) +
			" setup=" + string(top.SetupType) +
			" status=" + string(top.Status) +
			" quality=" + string(top.ExecutionQuality) +
			" priority=" + formatOneDecimal(top.AIPriority) +
			" risk=" + formatOneDecimal(top.RiskScore) +
			" timing=" + formatOneDecimal(top.TimingScore)
	}
	log.Printf("🔎 Hunter v7 route diag: raw=%d eligible=%d confirmed=%d watches=%d output=%d ready=%d near=%d watch=%d invalid_rr=%d filtered=%d min=%.1f fallback=%.1f top=%s",
		len(raw), eligible, len(confirmed), len(watches), len(out), ready, nearConfirm, watchOnly, invalidRR, filtered, minPriority, fallbackMinPriority, topText)
}

func hasV7ExecutableOutput(signals []V7SignalOutput, minPriority float64) bool {
	for _, sig := range signals {
		if sig.ExecutionQuality == V7ExecReady {
			return true
		}
		if sig.ExecutionQuality == V7ExecNearConfirm && sig.AIPriority >= minPriority && sig.RiskScore < 55 {
			return true
		}
	}
	return false
}

func formatOneDecimal(v float64) string {
	return fmt.Sprintf("%.1f", v)
}

func appendIfMissing(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func containsV7String(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// computeBTCETHBaseline4h extracts the max 4h change from BTC/ETH in the universe.
// This baseline is used for the strong-symbol relative strength override.
func computeBTCETHBaseline4h(universe []V7SymbolContext) float64 {
	btc4h := -999.0
	eth4h := -999.0
	for _, ctx := range universe {
		switch ctx.Symbol {
		case "BTCUSDT":
			btc4h = ctx.Change4h
		case "ETHUSDT":
			eth4h = ctx.Change4h
		}
	}
	if btc4h == -999 && eth4h == -999 {
		return 0 // No BTC/ETH data available
	}
	if btc4h > eth4h {
		return btc4h
	}
	return eth4h
}

func defaultV7Confirmations(sig *V7SignalOutput) []string {
	if sig == nil {
		return nil
	}

	switch sig.EntryMode {
	case V7EntryImmediate:
		return nil
	case V7EntryBreakout:
		return []string{
			"5m_or_15m_close_through_breakout_level",
			"oi_or_volume_expands_with_price",
			"no_failed_breakout_back_inside_range",
		}
	case V7EntryFastConfirm:
		if sig.Direction == V7DirShort {
			return []string{
				"5m_or_15m_close_below_trigger",
				"taker_buy_15m_lt_0_48",
				"no_immediate_reclaim_of_trigger_level",
			}
		}
		return []string{
			"5m_or_15m_close_above_trigger",
			"taker_buy_15m_gt_0_52",
			"no_immediate_loss_of_trigger_level",
		}
	case V7EntryWaitReclaim:
		if sig.Direction == V7DirShort {
			return []string{
				"live_price_in_entry_zone",
				"5m_close_below_ema20_or_entry_zone_mid",
				"taker_buy_15m_lt_0_48",
				"no_new_high_after_rejection",
			}
		}
		return []string{
			"live_price_in_entry_zone",
			"5m_close_above_ema20_or_entry_zone_mid",
			"taker_buy_15m_gt_0_52",
			"no_new_low_after_reclaim",
		}
	case V7EntryWaitBreakout:
		return []string{
			"5m_or_15m_close_above_entry_zone",
			"oi_continues_inflow",
			"bb_width_expansion_starts",
		}
	case V7EntryWaitReject:
		if sig.Direction == V7DirShort {
			return []string{
				"5m_or_15m_rejection_at_resistance_or_entry_zone",
				"taker_buy_15m_lt_0_48",
				"no_new_high_after_rejection",
			}
		}
		return []string{
			"5m_or_15m_rejection_at_support_or_entry_zone",
			"taker_buy_15m_gt_0_52",
			"no_new_low_after_rejection",
		}
	case V7EntryRangeEdge:
		if sig.Direction == V7DirShort {
			return []string{
				"enter_only_near_range_top",
				"5m_or_15m_rejection_from_range_top",
				"stop_above_range_high",
			}
		}
		return []string{
			"enter_only_near_range_bottom",
			"5m_or_15m_reclaim_from_range_bottom",
			"stop_below_range_low",
		}
	case V7EntryWaitPriceReversal:
		if sig.Direction == V7DirShort {
			return []string{
				"live_price_in_entry_zone",
				"5m_close_below_ema20_or_entry_zone_mid",
				"taker_buy_15m_lt_0_45",
				"no_new_high_after_reversal_signal",
			}
		}
		return []string{
			"live_price_in_entry_zone",
			"5m_close_above_ema20_or_entry_zone_mid",
			"taker_buy_15m_gt_0_52",
			"no_new_low_after_reversal_signal",
		}
	case V7EntryMomentumTrailing:
		return []string{
			"5m_price_holds_ema20_or_trailing_support",
			"momentum_not_exhausted",
			"taker_flow_not_flipping_against_direction",
		}
	default:
		if sig.Direction == V7DirShort {
			return []string{"directional_15m_close_short", "taker_flow_confirms_short", "risk_level_not_extreme"}
		}
		return []string{"directional_15m_close_long", "taker_flow_confirms_long", "risk_level_not_extreme"}
	}
}

// dedupeV7Tags removes duplicate tags while preserving first-seen order.
func dedupeV7Tags(tags []string) []string {
	if len(tags) < 2 {
		return tags
	}
	seen := make(map[string]bool, len(tags))
	out := tags[:0]
	for _, tag := range tags {
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	return out
}
