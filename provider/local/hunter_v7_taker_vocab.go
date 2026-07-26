package local

// Hunter v7 unified taker-flow vocabulary (U6.3, complete).
//
// Modules historically grew two long-side taker ladders with different
// spellings for the same ladder positions plus scattered sell-side synonyms.
// The migration finished on 2026-07-27: modules emit the canonical flow_
// names, tier rules and guards match them, the historical catalog entries
// and the transition alias map are deleted. This file is the canonical
// taker vocabulary's semantic registry.

// Canonical tag_semantics entries; definitions keep naming the former
// spellings so older stored records and analyses stay interpretable.
var v7TakerCanonicalDefs = []HunterV7TagDefinition{
	tagDef("flow_taker_buy_aggressive", "reason_code", "flow", "bullish", V7TagActionOpenSupport, "Unified taker ladder: aggressive buy-side flow at the module's top band (formerly taker_aggressive_buy / taker_buy_aggressive)."),
	tagDef("flow_taker_buy_strong", "reason_code", "flow", "bullish", V7TagActionOpenSupport, "Unified taker ladder: strong buy-side flow (formerly taker_strong_buy / taker_buy_strong)."),
	tagDef("flow_taker_buy_moderate", "reason_code", "flow", "bullish", V7TagActionEvidence, "Unified taker ladder: moderate buy-side flow (formerly taker_moderate_buy)."),
	tagDef("flow_taker_buy_sustained", "reason_code", "flow", "bullish", V7TagActionOpenSupport, "Unified taker ladder: buy-side aggression sustained through the move (formerly taker_sustained_buy)."),
	tagDef("flow_taker_buy_recovering", "reason_code", "flow", "bullish", V7TagActionEvidence, "Unified taker ladder: buy-side flow recovering from a flush (formerly taker_buy_recovering)."),
	tagDef("flow_taker_buy_neutral", "reason_code", "flow", "neutral", V7TagActionEvidence, "Unified taker ladder: near-balanced flow with a slight buy tilt (formerly taker_buy_neutral / taker_neutral_buy); requires other confirmation before opening."),
	tagDef("flow_taker_buy_aligned", "reason_code", "flow", "bullish", V7TagActionEvidence, "Unified taker ladder: flow aligned with the long direction (formerly taker_buy_aligned)."),
	tagDef("flow_taker_buy_weak", "reason_code", "flow", "bearish", V7TagActionReduceOrWait, "Unified taker ladder: weak buy-side flow against a long setup (formerly taker_weak_buy); reduce size or wait for flow confirmation."),
	tagDef("flow_taker_sell_dominant", "reason_code", "flow", "bearish", V7TagActionOpenSupport, "Unified taker ladder: dominant sell-side flow at the module's top short band (formerly taker_sell_dominant)."),
	tagDef("flow_taker_sell_strong", "reason_code", "flow", "bearish", V7TagActionOpenSupport, "Unified taker ladder: strong sell-side flow (formerly taker_sell_strong)."),
	tagDef("flow_taker_sell_emerging", "reason_code", "flow", "bearish", V7TagActionEvidence, "Unified taker ladder: sell pressure emerging (formerly taker_sell_emerging / taker_selling_emerging)."),
	tagDef("flow_taker_sell_aligned", "reason_code", "flow", "bearish", V7TagActionEvidence, "Unified taker ladder: flow aligned with the short direction (formerly taker_sell_aligned)."),
}

func init() {
	for _, def := range v7TakerCanonicalDefs {
		hunterV7TagCatalog[def.Tag] = def
	}
}
