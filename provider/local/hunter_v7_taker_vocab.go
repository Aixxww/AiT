package local

// Hunter v7 unified taker-flow vocabulary (U6.3, alias period).
//
// Modules historically grew two long-side taker ladders with different
// spellings for the same ladder positions (taker_aggressive_buy vs
// taker_buy_aggressive, ...) plus scattered sell-side synonyms. Per the
// redesign plan §3.9, the canonical vocabulary uses the flow_ domain prefix,
// and migration runs through an alias period:
//
//	emission (modules)     — still the historical names (provider golden
//	                         replay stays byte-identical)
//	backend matching       — still the historical names (tier rules, guards)
//	prompt payload         — canonical names via V7CanonicalizeTags; the
//	                         tag_semantics entries explain both spellings
//
// After two live verification cycles the emission flips to canonical names,
// backend matchers follow, and this alias map plus the historical catalog
// entries are deleted.

// v7TakerTagAliases maps every historical taker tag spelling to its canonical
// flow_ name. Tags absent from this map pass through unchanged.
var v7TakerTagAliases = map[string]string{
	// Long-side ladder.
	"taker_aggressive_buy": "flow_taker_buy_aggressive",
	"taker_buy_aggressive": "flow_taker_buy_aggressive",
	"taker_strong_buy":     "flow_taker_buy_strong",
	"taker_buy_strong":     "flow_taker_buy_strong",
	"taker_moderate_buy":   "flow_taker_buy_moderate",
	"taker_buy_recovering": "flow_taker_buy_recovering",
	"taker_buy_neutral":    "flow_taker_buy_neutral",
	"taker_neutral_buy":    "flow_taker_buy_neutral",
	"taker_buy_aligned":    "flow_taker_buy_aligned",
	"taker_weak_buy":       "flow_taker_buy_weak",

	// Short-side ladder.
	"taker_sell_dominant":    "flow_taker_sell_dominant",
	"taker_sell_strong":      "flow_taker_sell_strong",
	"taker_sell_emerging":    "flow_taker_sell_emerging",
	"taker_selling_emerging": "flow_taker_sell_emerging",
	"taker_sell_aligned":     "flow_taker_sell_aligned",
}

// V7CanonicalTag returns the canonical spelling for a tag, or the tag itself
// when no alias is registered.
func V7CanonicalTag(tag string) string {
	if canonical, ok := v7TakerTagAliases[tag]; ok {
		return canonical
	}
	return tag
}

// V7CanonicalizeTags maps a tag list to canonical spellings, deduplicating
// collisions (two historical spellings of the same position collapse to one
// canonical tag, keeping first-occurrence order). Returns the input slice
// unchanged when nothing needs rewriting.
func V7CanonicalizeTags(tags []string) []string {
	changed := false
	for _, tag := range tags {
		if _, ok := v7TakerTagAliases[tag]; ok {
			changed = true
			break
		}
	}
	if !changed {
		return tags
	}
	out := make([]string, 0, len(tags))
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		canonical := V7CanonicalTag(tag)
		if seen[canonical] {
			continue
		}
		seen[canonical] = true
		out = append(out, canonical)
	}
	return out
}

// Canonical tag_semantics entries. The historical entries stay registered for
// the alias period so both spellings resolve; definitions name the former
// spellings explicitly to keep the LLM unconfused across the transition.
var v7TakerCanonicalDefs = []HunterV7TagDefinition{
	tagDef("flow_taker_buy_aggressive", "reason_code", "flow", "bullish", V7TagActionOpenSupport, "Unified taker ladder: aggressive buy-side flow at the module's top band (formerly taker_aggressive_buy / taker_buy_aggressive)."),
	tagDef("flow_taker_buy_strong", "reason_code", "flow", "bullish", V7TagActionOpenSupport, "Unified taker ladder: strong buy-side flow (formerly taker_strong_buy / taker_buy_strong)."),
	tagDef("flow_taker_buy_moderate", "reason_code", "flow", "bullish", V7TagActionEvidence, "Unified taker ladder: moderate buy-side flow (formerly taker_moderate_buy)."),
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
