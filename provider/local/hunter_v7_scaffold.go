package local

// Hunter v7 module scaffold (U6.1)
//
// Every routing module builds its signal through the same ceremony: an
// 8-field header, ATR-derived entry zones, appendIfMissing reason codes and a
// 5-step finish (score clamp, floor gate, context snapshots, timing score,
// output contract check). The scaffold owns that ceremony so a module is left
// with only its matching predicate and its scoring ladder; modules migrate one
// per commit in U6.2 with the golden replay as the equality gate.

type v7SignalScaffold struct {
	sig   *V7SignalOutput
	ctx   *V7SymbolContext
	score float64
}

// newV7Signal builds the uniform signal header shared by every module.
func newV7Signal(ctx *V7SymbolContext, regime V7MarketRegime, setup V7SetupType, dir V7Direction, entryMode V7EntryMode, confidence string) *v7SignalScaffold {
	return &v7SignalScaffold{
		sig: &V7SignalOutput{
			Symbol:       ctx.Symbol,
			Direction:    dir,
			SetupType:    setup,
			Status:       V7StatusCandidate,
			EntryMode:    entryMode,
			Confidence:   confidence,
			MarketRegime: regime,
		},
		ctx: ctx,
	}
}

// add contributes to the setup score and records the evidence codes for the
// contribution. Codes go through appendIfMissing — the scaffold does not
// allow the duplicate-append idiom some legacy modules used.
func (s *v7SignalScaffold) add(points float64, codes ...string) *v7SignalScaffold {
	s.score += points
	return s.reason(codes...)
}

// reason records evidence codes without touching the score.
func (s *v7SignalScaffold) reason(codes ...string) *v7SignalScaffold {
	for _, code := range codes {
		s.sig.ReasonCodes = appendIfMissing(s.sig.ReasonCodes, code)
	}
	return s
}

// riskTag records risk tags through the same dedupe discipline.
func (s *v7SignalScaffold) riskTag(tags ...string) *v7SignalScaffold {
	for _, tag := range tags {
		s.sig.RiskTags = appendIfMissing(s.sig.RiskTags, tag)
	}
	return s
}

// zoneATR sets the entry zone as offsets from the current price in 15m-ATR
// multiples: price-below*ATR .. price+above*ATR. No-op without ATR data,
// matching the legacy modules' conditional zone blocks.
func (s *v7SignalScaffold) zoneATR(below, above float64) *v7SignalScaffold {
	if s.ctx.ATR15m > 0 && s.ctx.CurrentPrice > 0 {
		s.sig.EntryZone = V7PriceZone{
			Lower: s.ctx.CurrentPrice - s.ctx.ATR15m*below,
			Upper: s.ctx.CurrentPrice + s.ctx.ATR15m*above,
		}
	}
	return s
}

// zonePad sets the entry zone as a symmetric band whose half-width is the
// larger of pctFloor (percent of price) and atrMult×ATR15m — the pad idiom
// several modules hand-rolled.
func (s *v7SignalScaffold) zonePad(pctFloor, atrMult float64) *v7SignalScaffold {
	price := s.ctx.CurrentPrice
	if price <= 0 {
		return s
	}
	pad := price * pctFloor / 100
	if s.ctx.ATR15m > 0 && s.ctx.ATR15m*atrMult > pad {
		pad = s.ctx.ATR15m * atrMult
	}
	if pad > 0 {
		s.sig.EntryZone = V7PriceZone{Lower: price - pad, Upper: price + pad}
	}
	return s
}

// invalidate sets the invalidation rule.
func (s *v7SignalScaffold) invalidate(price float64, reason string) *v7SignalScaffold {
	if price > 0 {
		s.sig.Invalidation = V7InvalidationRule{Price: price, Reason: reason}
	}
	return s
}

// target appends a take-profit target when the price is valid.
func (s *v7SignalScaffold) target(price float64, reason string) *v7SignalScaffold {
	if price > 0 {
		s.sig.Targets = append(s.sig.Targets, V7Target{Price: price, Reason: reason})
	}
	return s
}

// finish runs the shared 5-step tail: clamp the score, gate on the module's
// floor, snapshot price/derivatives context, compute the timing score, and
// check the output contract. Returns nil when the score floor is not met.
func (s *v7SignalScaffold) finish(minScore float64) *V7SignalOutput {
	s.sig.SetupScore = clampFloat(s.score, 0, 100)
	if s.sig.SetupScore < minScore {
		return nil
	}
	s.sig.PriceCtx = buildPriceCtx(s.ctx)
	s.sig.DerivativesCtx = buildDerivCtx(s.ctx)
	s.sig.TimingScore = calcTimingScore(s.sig, s.ctx)
	v7CheckSignalContract(s.sig)
	return s.sig
}

// finishWithTiming is finish for modules that computed their own timing score
// instead of the shared formula.
func (s *v7SignalScaffold) finishWithTiming(minScore, timingScore float64) *V7SignalOutput {
	s.sig.SetupScore = clampFloat(s.score, 0, 100)
	if s.sig.SetupScore < minScore {
		return nil
	}
	s.sig.PriceCtx = buildPriceCtx(s.ctx)
	s.sig.DerivativesCtx = buildDerivCtx(s.ctx)
	s.sig.TimingScore = clampFloat(timingScore, 0, 100)
	v7CheckSignalContract(s.sig)
	return s.sig
}

// v7SignalContractHook is swapped to a panicking hook in tests so a module
// that ships a signal without zone/invalidation/targets/price context fails
// loudly there while production only logs.
var v7SignalContractHook = func(string, string) {}

// v7CheckSignalContract flags modules that emit signals missing the fields
// every downstream consumer assumes. Legacy modules that legitimately omit a
// field (e.g. watch-state signals without targets) surface here explicitly
// instead of relying on downstream zero-value defenses.
func v7CheckSignalContract(sig *V7SignalOutput) {
	if sig == nil {
		return
	}
	missing := ""
	switch {
	case sig.EntryZone.Lower <= 0 && sig.EntryZone.Upper <= 0:
		missing = "entry_zone"
	case sig.Invalidation.Price <= 0:
		missing = "invalidation"
	case len(sig.Targets) == 0:
		missing = "targets"
	case sig.PriceCtx == nil:
		missing = "price_context"
	}
	if missing != "" {
		v7SignalContractHook(string(sig.SetupType), missing)
	}
}
