package kernel

import (
	"fmt"
	"strings"
	"time"

	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// HunterV7RRVerdictRecorder receives one structured diagnostic record whenever
// a Hunter v7 candidate's tier reason becomes backend_rr_infeasible,
// rr_below_floor, or rr_unknown.
type HunterV7RRVerdictRecorder func(record store.HunterV7RRVerdictRecord)

// Stage values for store.HunterV7RRVerdictRecord.Stage.
const (
	HunterV7RRVerdictStageAssembly    = "assembly"
	HunterV7RRVerdictStagePromptFinal = "prompt_final"
)

// Reference price sources reported by hunterV7ReferencePriceWithSource.
const (
	hunterV7RefSourceRRReferencePrice = "rr_reference_price"
	hunterV7RefSourcePriceContextLast = "price_context_last"
	hunterV7RefSourceEntryZoneLower   = "entry_zone_lower"
	hunterV7RefSourceEntryZoneUpper   = "entry_zone_upper"
	hunterV7RefSourceNone             = "none"
)

// Geometry resolution layers reported by hunterV7CandidateRiskWithSources and
// hunterV7BackendCappedRREval.
const (
	hunterV7RiskSourceStrategy       = "strategy"
	hunterV7RiskSourceT2Config       = "t2_config"
	hunterV7RiskSourceT2Default      = "t2_default"
	hunterV7RiskSourceClassProfile   = "class_profile"
	hunterV7RiskSourceSetupOverride  = "setup_override"
	hunterV7RiskSourceBackendDefault = "backend_default"
	hunterV7RiskSourceUnset          = "unset"
)

func isHunterV7RRInfeasibleReason(reason string) bool {
	return reason == "backend_rr_infeasible" ||
		reason == hunterV7RRBelowFloorReason ||
		reason == hunterV7RRUnknownReason
}

// hunterV7ReferencePriceWithSource returns the reference price used by backend
// RR and stop-distance checks together with which field supplied it.
func hunterV7ReferencePriceWithSource(coin CandidateCoin) (float64, string) {
	if coin.V7RRReferencePrice > 0 {
		return coin.V7RRReferencePrice, hunterV7RefSourceRRReferencePrice
	}
	if coin.V7PriceContext != nil && coin.V7PriceContext.Last > 0 {
		return coin.V7PriceContext.Last, hunterV7RefSourcePriceContextLast
	}
	if strings.EqualFold(coin.Direction, "SHORT") && coin.V7EntryZone.Lower > 0 {
		return coin.V7EntryZone.Lower, hunterV7RefSourceEntryZoneLower
	}
	if strings.EqualFold(coin.Direction, "LONG") && coin.V7EntryZone.Upper > 0 {
		return coin.V7EntryZone.Upper, hunterV7RefSourceEntryZoneUpper
	}
	return 0, hunterV7RefSourceNone
}

// hunterV7CandidateRiskWithSources is hunterV7CandidateRisk plus the resolution
// layer that produced each value (strategy / t2_config / t2_default /
// class_profile / setup_override / unset).
func hunterV7CandidateRiskWithSources(coin CandidateCoin, geometry HunterV7ExecutionGeometry) (minRR, minSL, maxTP float64, minRRSource, minSLSource, maxTPSource string) {
	minRR, minSL, maxTP = geometry.MinRR, geometry.MinSLMovePct, geometry.MaxTPMovePct
	minRRSource = hunterV7RiskSourceUnset
	if minRR > 0 {
		minRRSource = hunterV7RiskSourceStrategy
	}
	minSLSource = hunterV7RiskSourceUnset
	if minSL > 0 {
		minSLSource = hunterV7RiskSourceStrategy
	}
	maxTPSource = hunterV7RiskSourceUnset
	if maxTP > 0 {
		maxTPSource = hunterV7RiskSourceStrategy
	}
	stack := store.V7StackNative
	if coin.V7Stack == local.V7StackT2 {
		stack = store.V7StackT2
		minRR, minSL = hunterV7T2MinRR(geometry), hunterV7T2MinSLMovePct(geometry)
		if geometry.T2MinRR > 0 {
			minRRSource = hunterV7RiskSourceT2Config
		} else {
			minRRSource = hunterV7RiskSourceT2Default
		}
		if geometry.T2MinSLMovePct > 0 {
			minSLSource = hunterV7RiskSourceT2Config
		} else {
			minSLSource = hunterV7RiskSourceT2Default
		}
	}
	profiles := geometry.ClassProfiles
	if profiles == nil {
		profiles = store.DefaultV7ClassProfiles()
	}
	p := profiles[coin.V7SignalClass].ForStack(stack)
	if p.MinRiskRewardRatio > 0 {
		minRR = p.MinRiskRewardRatio
		minRRSource = hunterV7RiskSourceClassProfile
	}
	if p.MinStopLossPriceMovePct > 0 {
		minSL = p.MinStopLossPriceMovePct
		minSLSource = hunterV7RiskSourceClassProfile
	}
	if p.MaxTakeProfitPriceMovePct > 0 {
		maxDrift, _ := hunterV7CandidateDrift(coin, geometry)
		maxTP = HunterV7EffectiveExecutionGeometry(p.MaxTakeProfitPriceMovePct, minSL, maxDrift, minRR, true).MaxTPMovePct
		maxTPSource = hunterV7RiskSourceClassProfile
	}
	g := geometry
	g.MinRR, g.MinSLMovePct, g.MaxTPMovePct = minRR, minSL, maxTP
	g = hunterV7SetupExecutionGeometry(coin, g)
	if g.MinRR != minRR {
		minRR = g.MinRR
		minRRSource = hunterV7RiskSourceSetupOverride
	}
	if g.MaxTPMovePct != maxTP {
		maxTP = g.MaxTPMovePct
		maxTPSource = hunterV7RiskSourceSetupOverride
	}
	return minRR, minSL, maxTP, minRRSource, minSLSource, maxTPSource
}

type hunterV7BackendCappedRRResult struct {
	price       float64
	priceSource string

	resolvedMinRR float64
	resolvedMinSL float64
	resolvedMaxTP float64
	minRRSource   string
	minSLSource   string
	maxTPSource   string

	minRR float64
	minSL float64
	maxTP float64

	rawRiskPct          float64
	effectiveRiskPct    float64
	minStopFloorApplied bool

	targets          []store.HunterV7RRTargetTerm
	bestRewardPct    float64
	bestTargetIndex  int
	bestTargetLabel  string
	bestTargetPrice  float64
	bestTargetRaw    float64
	bestTargetCapped bool

	rr float64
	ok bool
}

// hunterV7BackendCappedRREval is the single implementation shared by
// hunterV7BackendCappedRR and buildHunterV7RRVerdictRecord, guaranteeing the
// persisted instrumentation terms can never drift from the verdict math.
// When collectTargets is false, per-target term slices are skipped to avoid
// extra allocations on the hot path.
func hunterV7BackendCappedRREval(coin CandidateCoin, geometry HunterV7ExecutionGeometry, collectTargets bool) hunterV7BackendCappedRRResult {
	res := hunterV7BackendCappedRRResult{bestTargetIndex: -1}
	res.price, res.priceSource = hunterV7ReferencePriceWithSource(coin)

	// Fast early exit when not collecting diagnostics: preserve exact work of
	// hunterV7BackendCappedRR.
	if !collectTargets && (res.price <= 0 || coin.V7Invalidation.Price <= 0 || len(coin.V7Targets) == 0) {
		return res
	}

	// U6: the candidate's own values (class profile over the stack values,
	// then the per-setup relaxation) — the same values the validator and the
	// executor receive through the candidate stamps.
	res.resolvedMinRR, res.resolvedMinSL, res.resolvedMaxTP, res.minRRSource, res.minSLSource, res.maxTPSource = hunterV7CandidateRiskWithSources(coin, geometry)
	res.minRR, res.minSL, res.maxTP = res.resolvedMinRR, res.resolvedMinSL, res.resolvedMaxTP
	if res.minRR <= 0 {
		res.minRR = store.DefaultMinRiskRewardRatio // #77: unset only
		res.minRRSource = hunterV7RiskSourceBackendDefault
	}
	if res.minSL <= 0 {
		res.minSL = hunterV7BackendMinStopPct
		res.minSLSource = hunterV7RiskSourceBackendDefault
	}
	if res.maxTP <= 0 {
		res.maxTP = HunterV7EffectiveExecutionGeometry(0, 0, 0, res.minRR, true).MaxTPMovePct
		res.maxTPSource = hunterV7RiskSourceBackendDefault
	}

	if res.price <= 0 || coin.V7Invalidation.Price <= 0 || len(coin.V7Targets) == 0 {
		return res
	}

	riskPct := 0.0
	switch {
	case strings.EqualFold(coin.Direction, "SHORT"):
		riskPct = (coin.V7Invalidation.Price - res.price) / res.price * 100
	case strings.EqualFold(coin.Direction, "LONG"):
		riskPct = (res.price - coin.V7Invalidation.Price) / res.price * 100
	default:
		return res
	}
	res.rawRiskPct = riskPct
	if riskPct <= 0 {
		return res
	}

	effectiveRiskPct := riskPct
	if effectiveRiskPct < res.minSL {
		effectiveRiskPct = res.minSL + hunterV7BackendStopRepairBuffer
		res.minStopFloorApplied = true
	}
	res.effectiveRiskPct = effectiveRiskPct

	if collectTargets {
		res.targets = make([]store.HunterV7RRTargetTerm, 0, len(coin.V7Targets))
	}
	bestRewardPct := 0.0
	for idx, target := range coin.V7Targets {
		rawRewardPct := hunterV7DirectionalRewardPct(res.price, target.Price, coin.Direction)
		cappedRewardPct := rawRewardPct
		capped := false
		if rawRewardPct > 0 && cappedRewardPct > res.maxTP {
			cappedRewardPct = res.maxTP
			capped = true
		}
		var label string
		if collectTargets {
			label = hunterV7RRTargetLabel(coin, idx, target.Price)
			res.targets = append(res.targets, store.HunterV7RRTargetTerm{
				Index:           idx,
				Label:           label,
				Price:           target.Price,
				Reason:          target.Reason,
				RawRewardPct:    rawRewardPct,
				CappedRewardPct: cappedRewardPct,
				Capped:          capped,
			})
		}
		if rawRewardPct <= 0 {
			continue
		}
		if cappedRewardPct > bestRewardPct {
			bestRewardPct = cappedRewardPct
			res.bestTargetIndex = idx
			res.bestTargetLabel = label
			res.bestTargetPrice = target.Price
			res.bestTargetRaw = rawRewardPct
			res.bestTargetCapped = capped
		}
	}
	res.bestRewardPct = bestRewardPct
	if bestRewardPct <= 0 {
		return res
	}
	res.rr = bestRewardPct / effectiveRiskPct
	res.ok = true
	return res
}

func hunterV7RRTargetLabel(coin CandidateCoin, idx int, price float64) string {
	if price > 0 {
		switch {
		case coin.V7TP0Price > 0 && price == coin.V7TP0Price:
			return "tp0"
		case coin.V7TP1Price > 0 && price == coin.V7TP1Price:
			return "tp1"
		case coin.V7TP2Price > 0 && price == coin.V7TP2Price:
			return "tp2"
		}
	}
	return fmt.Sprintf("target_%d", idx)
}

func buildHunterV7RRVerdictRecord(coin CandidateCoin, geometry HunterV7ExecutionGeometry, reason, stage string, cycleNum int, ts time.Time) store.HunterV7RRVerdictRecord {
	res := hunterV7BackendCappedRREval(coin, geometry, true)
	targets := res.targets
	if targets == nil {
		targets = []store.HunterV7RRTargetTerm{}
	}
	return store.HunterV7RRVerdictRecord{
		CycleNumber: cycleNum,
		Timestamp:   ts,
		Stage:       stage,
		Symbol:      coin.Symbol,
		Direction:   coin.Direction,
		SetupType:   coin.V7SetupType,
		SignalID:    coin.V7SignalID,
		Stack:       coin.V7Stack,
		SignalClass: coin.V7SignalClass,

		ReferencePrice:       res.price,
		ReferencePriceSource: res.priceSource,
		InvalidationPrice:    coin.V7Invalidation.Price,

		RawRiskPct:          res.rawRiskPct,
		EffectiveRiskPct:    res.effectiveRiskPct,
		MinSLMovePct:        res.minSL,
		MinStopFloorApplied: res.minStopFloorApplied,
		StopRepairBufferPct: hunterV7BackendStopRepairBuffer,

		TargetRewardsJSON:      store.ToJSON(targets),
		MaxTPMovePct:           res.maxTP,
		BestRewardPct:          res.bestRewardPct,
		BestTargetIndex:        res.bestTargetIndex,
		BestTargetLabel:        res.bestTargetLabel,
		BestTargetPrice:        res.bestTargetPrice,
		BestTargetRawRewardPct: res.bestTargetRaw,
		BestTargetCapped:       res.bestTargetCapped,

		CappedRR:   res.rr,
		MinRRFloor: res.minRR,
		OK:         res.ok,
		Reason:     reason,

		ResolvedMinRR:        res.resolvedMinRR,
		ResolvedMinSLMovePct: res.resolvedMinSL,
		ResolvedMaxTPMovePct: res.resolvedMaxTP,
		MinRRSource:          res.minRRSource,
		MinSLSource:          res.minSLSource,
		MaxTPSource:          res.maxTPSource,
	}
}

// SetV7RRVerdictRecorder installs the optional callback that persists
// structured computation terms whenever a candidate's tier reason becomes
// backend_rr_infeasible, rr_below_floor, or rr_unknown.
func (e *StrategyEngine) SetV7RRVerdictRecorder(recorder HunterV7RRVerdictRecorder) {
	if e != nil {
		e.v7RRVerdictRecorder = recorder
	}
}

func (e *StrategyEngine) recordV7RRVerdicts(candidates []CandidateCoin, stage string, cycleNum int, ts time.Time) {
	if len(candidates) == 0 {
		return
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	for i := range candidates {
		if candidates[i].V7RRVerdict == nil {
			continue
		}
		rec := *candidates[i].V7RRVerdict
		rec.Stage = stage
		rec.CycleNumber = cycleNum
		rec.Timestamp = ts
		candidates[i].V7RRVerdict = &rec
		if e != nil && e.v7RRVerdictRecorder != nil {
			e.v7RRVerdictRecorder(rec)
		}
	}
}
