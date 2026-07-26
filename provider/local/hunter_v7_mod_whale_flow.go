package local

import "math"

// ============================================================================
// Module: Whale Flow Reversal
// ============================================================================
// Detects stealth accumulation by large players through OI-price divergence:
// OI rising while price is flat or declining suggests informed buyers building
// positions.  When combined with neutral-to-low funding and improving taker
// buy flow, it signals an impending directional move.

type whaleFlowReversalModule struct{}

func (m *whaleFlowReversalModule) Name() string           { return "whale_flow_reversal" }
func (m *whaleFlowReversalModule) SetupType() V7SetupType { return V7SetupWhaleFlow }
func (m *whaleFlowReversalModule) Direction() V7Direction { return V7DirLong }

func (m *whaleFlowReversalModule) Match(ctx *V7SymbolContext, regime V7MarketRegime) bool {
	if ctx == nil || ctx.Snapshot == nil {
		return false
	}
	price := ctx.CurrentPrice
	if price <= 0 {
		return false
	}
	snap := ctx.Snapshot

	// OI must be growing meaningfully
	oiDelta := snap.OIDelta1h
	if oiDelta < 5 {
		return false
	}

	// Funding must be neutral (whales aren't crowded)
	if math.Abs(snap.FundingRate) > 0.0005 {
		return false
	}

	// Must have sufficient liquidity
	if snap.QuoteVolume24h < 30_000_000 {
		return false
	}

	return true
}

func (m *whaleFlowReversalModule) Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput {
	if !m.Match(ctx, regime) {
		return nil
	}

	price := ctx.CurrentPrice
	snap := ctx.Snapshot
	base := 50.0

	// 1. OI-price divergence: OI up + price flat/down = accumulation
	oiDelta := snap.OIDelta1h
	priceChange := ctx.Change1h
	oiPriceDiv := oiDelta - priceChange*2 // crude divergence metric
	if oiPriceDiv > 5 {
		base += 10 // clear divergence — accumulation
	}
	if oiPriceDiv > 10 {
		base += 5
	}

	// 2. Taker buy ratio: turning bullish
	takerBuy := ctx.TakerBuy15m
	if takerBuy > 0.53 {
		base += 7
	}
	if takerBuy > 0.57 {
		base += 5
	}

	// 3. LSR direction: improving (or at least not deteriorating)
	if snap.LSR > 1.0 {
		base += 5
	}
	if snap.LSRPrev > 0 && snap.LSR > snap.LSRPrev {
		base += 3 // LSR improving
	}

	// 4. Funding neutral is good (already filtered, but score it)
	if math.Abs(snap.FundingRate) < 0.0002 {
		base += 4
	}

	// 5. Regime fitness: rotation/range/compression are best
	switch regime {
	case V7RegimeRotation, V7RegimeRange, V7RegimeCompression:
		base += 6
	case V7RegimePanicDump, V7RegimeTrendDown:
		base -= 8
	}

	// Standard ATR-based TP/SL
	risk := ctx.ATR1h * 1.0
	if risk <= 0 {
		risk = price * 0.01
	}
	stop := price - risk
	tp0 := price + risk*0.8
	tp1 := price + risk*1.5
	tp2 := price + risk*3.0

	// Determine direction dynamically — mostly LONG but check
	dir := V7DirLong
	if priceChange < -3 && takerBuy < 0.48 {
		// Strong selling despite OI build — may be short accumulation
		dir = V7DirShort
		stop = price + risk
		tp0 = price - risk*0.8
		tp1 = price - risk*1.5
		tp2 = price - risk*3.0
	}

	s := newV7Signal(ctx, regime, V7SetupWhaleFlow, dir, V7EntryWaitConfirm, "C")
	s.score = base
	// Geometry is unconditional in this module (no >0 guards) — kept verbatim.
	s.sig.EntryZone = V7PriceZone{Lower: price - ctx.ATR15m*0.5, Upper: price + ctx.ATR15m*0.3}
	s.sig.Invalidation = V7InvalidationRule{Price: stop, Reason: "ATR-based whale flow stop"}
	s.sig.Targets = []V7Target{
		{Price: tp0, Reason: "TP0 quick"},
		{Price: tp1, Reason: "TP1 swing"},
		{Price: tp2, Reason: "TP2 extended"},
	}

	s.reason("whale_flow_detected")
	if math.Abs(priceChange) < 2 && oiDelta > 5 {
		s.reason("stealth_accumulation_breakout")
	}

	sig := s.finishWithTiming(0, 45+takerBuy*50+math.Min(oiDelta, 20))
	ApplyV7OIAccumulationEvidence(sig, ctx)

	return sig
}
