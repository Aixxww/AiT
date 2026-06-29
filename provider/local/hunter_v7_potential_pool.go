package local

import (
	"math"
	"sort"
)

const defaultV7PotentialPoolLimit = 20

func BuildV7PotentialPool(universe []V7SymbolContext, rawSignals []V7SignalOutput, limit int) []V7PotentialCandidate {
	if limit <= 0 {
		limit = defaultV7PotentialPoolLimit
	}
	if len(universe) == 0 {
		return nil
	}
	baseline4h := computeBTCETHBaseline4h(universe)
	matched := v7MatchedSetupsBySymbol(rawSignals)
	out := make([]V7PotentialCandidate, 0, len(universe))
	for i := range universe {
		ctx := &universe[i]
		if ctx.Symbol == "" || ctx.CurrentPrice <= 0 {
			continue
		}
		candidate := buildV7PotentialCandidate(ctx, baseline4h, matched[ctx.Symbol])
		if candidate.OpportunityPotentialScore <= 0 {
			continue
		}
		out = append(out, candidate)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].OpportunityPotentialScore != out[j].OpportunityPotentialScore {
			return out[i].OpportunityPotentialScore > out[j].OpportunityPotentialScore
		}
		if out[i].Amplitude24h != out[j].Amplitude24h {
			return out[i].Amplitude24h > out[j].Amplitude24h
		}
		return out[i].Symbol < out[j].Symbol
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func buildV7PotentialCandidate(ctx *V7SymbolContext, baseline4h float64, matchedSetups []V7SetupType) V7PotentialCandidate {
	oi1h, oi4h := v7PotentialOIDeltas(ctx)
	funding, lsr := v7PotentialFundingCrowdingInputs(ctx)
	components := V7PotentialComponents{
		Amplitude:        v7PotentialAmplitudeScore(ctx.Amplitude24h),
		Velocity:         v7PotentialVelocityScore(ctx.Velocity5m, ctx.Velocity15m),
		VolumeBurst:      v7PotentialVolumeBurstScore(ctx.VolumeBurst5m, ctx.VolumeBurst15m),
		OIDelta:          v7PotentialOIDeltaScore(ctx.OI5m, oi1h, oi4h),
		FundingCrowding:  v7PotentialFundingCrowdingScore(funding, lsr),
		RelativeStrength: v7PotentialRelativeStrengthScore(ctx.Change4h - baseline4h),
	}
	score := components.Amplitude*0.25 +
		components.Velocity*0.20 +
		components.VolumeBurst*0.15 +
		components.OIDelta*0.15 +
		components.FundingCrowding*0.10 +
		components.RelativeStrength*0.15

	return V7PotentialCandidate{
		Symbol:                    ctx.Symbol,
		Direction:                 v7PotentialDirection(ctx),
		OpportunityPotentialScore: clampFloat(score, 0, 100),
		Components:                components,
		Amplitude24h:              ctx.Amplitude24h,
		Velocity5m:                ctx.Velocity5m,
		Velocity15m:               ctx.Velocity15m,
		VolumeBurst5m:             ctx.VolumeBurst5m,
		VolumeBurst15m:            ctx.VolumeBurst15m,
		OIDelta1h:                 oi1h,
		OIDelta4h:                 oi4h,
		FundingRate:               funding,
		RelativeStrength4h:        ctx.Change4h - baseline4h,
		MatchedModule:             len(matchedSetups) > 0,
		MatchedSetups:             append([]V7SetupType{}, matchedSetups...),
		TrackingWindows:           []string{"30m", "60m"},
		AuditUse:                  "potential_pool_mfe_mae_module_hit_audit",
	}
}

func v7MatchedSetupsBySymbol(rawSignals []V7SignalOutput) map[string][]V7SetupType {
	out := make(map[string][]V7SetupType)
	seen := make(map[string]bool)
	for _, sig := range rawSignals {
		if sig.Symbol == "" || sig.SetupType == "" || sig.SetupType == V7SetupModuleNoMatch {
			continue
		}
		key := sig.Symbol + "|" + string(sig.SetupType)
		if seen[key] {
			continue
		}
		seen[key] = true
		out[sig.Symbol] = append(out[sig.Symbol], sig.SetupType)
	}
	for symbol := range out {
		sort.Slice(out[symbol], func(i, j int) bool { return out[symbol][i] < out[symbol][j] })
	}
	return out
}

func v7PotentialAmplitudeScore(amplitude float64) float64 {
	return clampFloat(amplitude/20*100, 0, 100)
}

func v7PotentialVelocityScore(v5, v15 float64) float64 {
	s5 := math.Abs(v5) / 3 * 100
	s15 := math.Abs(v15) / 5 * 100
	if s15 > s5 {
		return clampFloat(s15, 0, 100)
	}
	return clampFloat(s5, 0, 100)
}

func v7PotentialVolumeBurstScore(b5, b15 float64) float64 {
	burst := b5
	if b15 > burst {
		burst = b15
	}
	return clampFloat((burst-1)/2*100, 0, 100)
}

func v7PotentialOIDeltaScore(oi5m, oi1h, oi4h float64) float64 {
	s5 := math.Abs(oi5m) / 3 * 100
	s1 := math.Abs(oi1h) / 10 * 100
	s4 := math.Abs(oi4h) / 20 * 100
	return clampFloat(math.Max(s5, math.Max(s1, s4)), 0, 100)
}

func v7PotentialFundingCrowdingScore(funding, lsr float64) float64 {
	fundingScore := math.Abs(funding) / 0.0015 * 100
	lsrScore := 0.0
	if lsr > 0 {
		lsrScore = math.Abs(lsr-1) / 0.8 * 100
	}
	return clampFloat(math.Max(fundingScore, lsrScore), 0, 100)
}

func v7PotentialRelativeStrengthScore(relative4h float64) float64 {
	return clampFloat(math.Abs(relative4h)/10*100, 0, 100)
}

func v7PotentialDirection(ctx *V7SymbolContext) V7Direction {
	momentum := ctx.Change4h*0.35 + ctx.Velocity15m*0.40 + ctx.Velocity5m*0.25
	if ctx.TakerBuy15m > 0 {
		momentum += (ctx.TakerBuy15m - 0.5) * 8
	}
	funding, lsr := v7PotentialFundingCrowdingInputs(ctx)
	if funding > 0.001 && lsr > 1.2 && momentum < 2 {
		return V7DirShort
	}
	if funding < -0.001 && lsr > 0 && lsr < 0.85 && momentum > -2 {
		return V7DirLong
	}
	if momentum < 0 {
		return V7DirShort
	}
	return V7DirLong
}

func v7PotentialOIDeltas(ctx *V7SymbolContext) (float64, float64) {
	if ctx == nil || ctx.Snapshot == nil {
		return 0, 0
	}
	return ctx.Snapshot.OIDelta1h, ctx.Snapshot.OIDelta4h
}

func v7PotentialFundingCrowdingInputs(ctx *V7SymbolContext) (float64, float64) {
	if ctx == nil || ctx.Snapshot == nil {
		return 0, 0
	}
	lsr := ctx.Snapshot.LSR
	if ctx.Snapshot.LSRPrev > 0 {
		lsr = ctx.Snapshot.LSRPrev
	}
	return ctx.Snapshot.FundingRate, lsr
}
