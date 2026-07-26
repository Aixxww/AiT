package local

import "math"

// ============================================================================
// Hunter v8 - OI Invisible Accumulation Evidence (v8-SPEC P2-1)
// ============================================================================
// Normalizes OI accumulation evidence across accumulation, squeeze breakout,
// whale-flow, and watch modules. The output is intentionally conservative:
// it adds shared reason codes and a small bounded score boost, but does not
// bypass execution gates or convert watch-only signals into executable ones.

type V7OIAccumulationEvidence struct {
	Score                   float64
	OI1h                    float64
	OI4h                    float64
	InvisibleAccumulation   bool
	OIPriceDivergence       bool
	OI4hStealthBuild        bool
	OI1hConfirming          bool
	BuildWithoutPriceMarkup bool
	FundingNotCrowded       bool
	BBCompressed            bool
	VolumeBurstAtBreakout   bool
	TakerBuyRatioAbove055   bool
	LSRBalancedAccumulation bool
}

func assessV7OIAccumulation(ctx *V7SymbolContext) V7OIAccumulationEvidence {
	var ev V7OIAccumulationEvidence
	if ctx == nil || ctx.Snapshot == nil || ctx.CurrentPrice <= 0 {
		return ev
	}
	snap := ctx.Snapshot
	ev.OI1h = snap.OIDelta1h
	ev.OI4h = snap.OIDelta4h

	priceQuiet1h := math.Abs(ctx.Change1h) <= 1.5
	priceNotMarkedUp24h := ctx.Change24h <= 12 && ctx.Change24h >= -12
	priceNotMarkedUp4h := ctx.Change4h <= 5 && ctx.Change4h >= -8

	ev.OI1hConfirming = ev.OI1h >= 2
	ev.OI4hStealthBuild = ev.OI4h >= 5 && ev.OI4h <= 35 && priceNotMarkedUp24h
	ev.BuildWithoutPriceMarkup = (ev.OI1h >= 3 || ev.OI4h >= 8) && priceNotMarkedUp24h && priceNotMarkedUp4h
	ev.OIPriceDivergence = (ev.OI1h >= 5 && ctx.Change1h <= 2) || (ev.OI4h >= 8 && ctx.Change4h <= 4)
	ev.FundingNotCrowded = math.Abs(snap.FundingRate) <= 0.0005
	ev.BBCompressed = ctx.BBWidthPercentile > 0 && ctx.BBWidthPercentile <= 15
	ev.VolumeBurstAtBreakout = ctx.VolumeBurst5m >= 2 || ctx.VolumeBurst15m >= 1.8
	ev.TakerBuyRatioAbove055 = ctx.TakerBuy15m > 0.55 || ctx.TakerBuy5m > 0.55
	ev.LSRBalancedAccumulation = snap.LSR >= 0.85 && snap.LSR <= 1.15

	if ev.OI1hConfirming {
		ev.Score += boundedScore(ev.OI1h, 2, 12, 16)
	}
	if ev.OI4hStealthBuild {
		ev.Score += boundedScore(ev.OI4h, 5, 25, 20)
	}
	if ev.OIPriceDivergence {
		ev.Score += 16
	}
	if ev.BuildWithoutPriceMarkup {
		ev.Score += 12
	}
	if ev.FundingNotCrowded {
		ev.Score += 8
	}
	if ev.BBCompressed {
		ev.Score += 10
	}
	if ev.VolumeBurstAtBreakout {
		ev.Score += 8
	}
	if ev.TakerBuyRatioAbove055 {
		ev.Score += 8
	}
	if ev.LSRBalancedAccumulation {
		ev.Score += 6
	}

	ev.InvisibleAccumulation = ev.Score >= 45 &&
		ev.FundingNotCrowded &&
		(ev.OIPriceDivergence || (ev.OI4hStealthBuild && ev.OI1hConfirming)) &&
		(ev.BBCompressed || ev.BuildWithoutPriceMarkup || priceQuiet1h)

	return ev
}

func ApplyV7OIAccumulationEvidence(sig *V7SignalOutput, ctx *V7SymbolContext) V7OIAccumulationEvidence {
	ev := assessV7OIAccumulation(ctx)
	if sig == nil || ev.Score <= 0 {
		return ev
	}

	if ev.InvisibleAccumulation {
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "oi_invisible_accumulation_detected")
	}
	if ev.OI4hStealthBuild {
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "oi_4h_stealth_build")
	}
	if ev.OI1hConfirming {
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "oi_1h_confirming_accumulation")
	}
	if ev.BuildWithoutPriceMarkup {
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "oi_build_without_price_markup")
	}
	if ev.FundingNotCrowded {
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "funding_not_crowded")
	}
	if ev.BBCompressed {
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "bb_compressed")
	}
	if ev.VolumeBurstAtBreakout {
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "volume_burst_at_breakout")
	}
	if ev.TakerBuyRatioAbove055 {
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "taker_buy_ratio_above_0.55")
	}
	if ev.LSRBalancedAccumulation {
		sig.ReasonCodes = appendIfMissing(sig.ReasonCodes, "lsr_balanced_accumulation")
	}

	if ev.InvisibleAccumulation {
		boost := math.Min(6, ev.Score/12)
		sig.SetupScore = clampFloat(sig.SetupScore+boost, 0, 100)
	}
	return ev
}
