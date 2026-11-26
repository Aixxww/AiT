package local

import "github.com/Aixxww/AiT/store"

// V7HTFAgainstDirectionTag marks a gated setup (store.V7HTFFamily) whose
// direction opposes the candidate's own 1h/4h direction (#122, #111 Q2).
// The kernel tier cap, the decision validator and the trader guard act on
// it per family; the provider only states the fact.
const V7HTFAgainstDirectionTag = "htf_against_direction"

// V7HTFBias is one timeframe's direction from its EMAs: +1 when price >
// EMA20 > EMA60, -1 when price < EMA20 < EMA60, 0 otherwise (mixed or no
// data).
func V7HTFBias(price, ema20, ema60 float64) int {
	if price <= 0 || ema20 <= 0 || ema60 <= 0 {
		return 0
	}
	switch {
	case price > ema20 && ema20 > ema60:
		return 1
	case price < ema20 && ema20 < ema60:
		return -1
	}
	return 0
}

// V7HTFDirection is the candidate's higher-timeframe direction: +1 / -1
// only when the 1h and the 4h agree (both up / both down), else 0. A mixed
// or missing timeframe means there is no clear 1h/4h direction, which never
// gates (#122: "against 1h/4h" = against both).
func V7HTFDirection(ctx *V7SymbolContext) int {
	if ctx == nil {
		return 0
	}
	b1 := V7HTFBias(ctx.CurrentPrice, ctx.EMA20_1h, ctx.EMA60_1h)
	b4 := V7HTFBias(ctx.CurrentPrice, ctx.EMA20_4h, ctx.EMA60_4h)
	if b1 != 0 && b1 == b4 {
		return b1
	}
	return 0
}

// v7HTFAgainstDirection reports whether a gated signal trades against its
// 1h/4h direction.
func v7HTFAgainstDirection(sig *V7SignalOutput, ctx *V7SymbolContext) bool {
	if sig == nil || store.V7HTFFamily(string(sig.SetupType)) == "" {
		return false
	}
	htf := V7HTFDirection(ctx)
	switch sig.Direction {
	case V7DirLong:
		return htf < 0
	case V7DirShort:
		return htf > 0
	}
	return false
}

// applyV7HTFDirectionTag is the htf_direction router pass.
func applyV7HTFDirectionTag(sig *V7SignalOutput, ctx *V7SymbolContext) {
	if v7HTFAgainstDirection(sig, ctx) {
		sig.RiskTags = appendIfMissing(sig.RiskTags, V7HTFAgainstDirectionTag)
	}
}
