package kernel

import (
	"fmt"
	"strings"

	"github.com/Aixxww/AiT/market"
)

// Single authoritative Hunter v7 tier (#86, audit AUD-B7, design
// docs/audit/optimization-design-20261124.md cluster 1).
//
// Before #86 every round produced two tier verdicts: the assembly tier
// (CandidateCoin.V7ExecutionTier from hunterV7SignalsToCandidateCoins) and
// the prompt-final tier (hunterV7PromptItems: live reprice, live
// confirmations, readiness re-check, PromptWait). The LLM saw the
// prompt-final tier, while the AI-skip check, the decision validator and the
// execution gate read the assembly tier.
//
// finalizeHunterV7Tiers computes the prompt-final tier once per round and
// writes it back to ctx.CandidateCoins, so every reader sees the verdict the
// LLM saw. The prompt renders the stored items; it never re-tiers.

// hunterV7FinalTiers is the per-round finalization result kept on Context.
type hunterV7FinalTiers struct {
	items []hunterV7PromptCandidate
}

// FinalizeHunterV7Tiers finalizes the Hunter v7 tiers of ctx for this round
// (idempotent). Callers that act on tiers before the prompt is built (the
// trader's AI-skip check) call it first; BuildUserPrompt calls it lazily.
// It needs ctx.MarketDataMap; use PrepareHunterV7Tiers to fetch it first.
func (e *StrategyEngine) FinalizeHunterV7Tiers(ctx *Context) {
	if e == nil || ctx == nil || !strings.EqualFold(e.config.CoinSource.SourceType, "hunter_v7") {
		return
	}
	e.finalizeHunterV7Tiers(ctx, hunterV7PositionSymbols(ctx))
}

// PrepareHunterV7Tiers fetches the round's market data (when ctx has none
// yet, exactly as GetFullDecisionWithStrategy would) and finalizes the
// tiers. GetFullDecisionWithStrategy then reuses both: it fetches market
// data only when MarketDataMap is empty, and BuildUserPrompt renders the
// finalized items.
func PrepareHunterV7Tiers(ctx *Context, engine *StrategyEngine) error {
	if ctx == nil || engine == nil {
		return nil
	}
	// Same pre-fetch steps as GetFullDecisionWithStrategy, so the market
	// data is identical to what that function would fetch (both idempotent).
	engine.GetConfig().ClampLimits()
	if engine.marketEnv != nil {
		ctx.MarketEnv = engine.marketEnv
	}
	if len(ctx.MarketDataMap) == 0 {
		if err := fetchMarketDataWithStrategy(ctx, engine); err != nil {
			return fmt.Errorf("failed to fetch market data: %w", err)
		}
	}
	engine.FinalizeHunterV7Tiers(ctx)
	return nil
}

// HunterV7TiersFinalized reports whether ctx carries finalized tiers.
func (ctx *Context) HunterV7TiersFinalized() bool {
	return ctx != nil && ctx.hunterV7Final != nil
}

func hunterV7PositionSymbols(ctx *Context) map[string]bool {
	out := make(map[string]bool, len(ctx.Positions))
	for _, pos := range ctx.Positions {
		out[market.Normalize(pos.Symbol)] = true
	}
	return out
}

func (e *StrategyEngine) finalizeHunterV7Tiers(ctx *Context, positionSymbols map[string]bool) []hunterV7PromptCandidate {
	if ctx.hunterV7Final != nil {
		return ctx.hunterV7Final.items
	}
	items := e.hunterV7PromptItems(ctx, positionSymbols)
	// Record before the write-back so AssembledTier is the assembly verdict.
	ctx.HunterV7PromptFinal = hunterV7PromptFinalTiers(ctx.CandidateCoins, items)

	final := make(map[string]hunterV7PromptCandidate, len(items))
	for _, it := range items {
		final[HunterV7PromptFinalKey(it.Coin.Symbol, it.Coin.V7SetupType)] = it
	}
	// Copy, so a caller's slice (e.g. the assembled candidates a harness
	// keeps for comparison) is not modified behind its back.
	coins := make([]CandidateCoin, len(ctx.CandidateCoins))
	copy(coins, ctx.CandidateCoins)
	for i := range coins {
		c := &coins[i]
		if c.V7SetupType == "" {
			continue
		}
		it, ok := final[HunterV7PromptFinalKey(c.Symbol, c.V7SetupType)]
		if !ok {
			// Not shown to the LLM (an existing position, or no market data
			// this round): it keeps the assembly verdict, which is also what
			// any later reader sees. The LLM cannot select a signal it was
			// not shown.
			continue
		}
		c.V7AssembledTier, c.V7AssembledReason = c.V7ExecutionTier, c.V7TierReason
		c.V7ExecutionTier, c.V7TierReason = it.Tier, it.Reason
		c.V7RRVerdict = it.Coin.V7RRVerdict
	}
	ctx.CandidateCoins = coins
	ctx.hunterV7Final = &hunterV7FinalTiers{items: items}
	return items
}
