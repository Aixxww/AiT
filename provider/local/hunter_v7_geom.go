package local

import "github.com/Aixxww/AiT/store"

// ============================================================================
// Hunter v7 — Geometry primitives (U2.1+, lean-core redesign)
// ============================================================================
// "Where is price inside the entry zone" was computed at five call sites with
// three divergent semantics: the provider clamped to 0-100, two kernel gates
// and two kernel display sites used raw values (negative = below the zone),
// and the trader additionally repaired inverted bounds. The divergence was
// partly intentional — displays need raw positions to say "below_zone" — so
// the resolution is one implementation with the variant made explicit at the
// call site, not a forced single semantic.

// Risk/reward floor (#77): the strategy's RiskControl.MinRiskRewardRatio is
// authoritative. The kernel passes it as V7Config.MinRiskRewardRatio; the
// former code floors (1.2 absolute / 1.5 executable) are gone. Geometry below
// the floor is invalid_rr and can never execute.

// EffectiveMinRR is the router's RR floor: the configured value, or the
// store default when the strategy leaves it unset.
func (c V7Config) EffectiveMinRR() float64 {
	if c.MinRiskRewardRatio > 0 {
		return c.MinRiskRewardRatio
	}
	return store.DefaultMinRiskRewardRatio
}

// V7ZonePositionPct returns price's RAW position inside the zone as a
// percentage of the zone span: 0 = lower bound, 100 = upper bound, negative =
// below the zone, >100 = above it. Inverted bounds are repaired by swapping.
// ok is false when the zone or price cannot support the computation.
//
// zone_position single source of truth (#134): this function and
// V7ZonePositionPctClamped are the ONLY implementations of the concept. The
// measure is direction-independent (from the lower edge for LONG and SHORT
// alike). The raw value is internal only — it feeds below_zone/above_zone
// classification and window-health falloff — and is never emitted; every
// emitted field (entry_zone_position, entry_zone_position_pct,
// entry_zone_pos, zone_position_pct) and every guard uses the clamped form.
func V7ZonePositionPct(zone V7PriceZone, price float64) (pos float64, ok bool) {
	lower, upper := zone.Lower, zone.Upper
	if price <= 0 || lower <= 0 || upper <= 0 {
		return 0, false
	}
	if lower > upper {
		lower, upper = upper, lower
	}
	if upper <= lower {
		return 0, false
	}
	return (price - lower) / (upper - lower) * 100, true
}

// V7ZonePositionPctClamped is the canonical zone_position (#134): the
// from-lower-edge percentage saturated to [0,100]. It is what every emitted
// field and every threshold comparison ("pos < 65", "pos > 80") means. For
// LONG a low value is the value/retest end of the zone; for SHORT a high value
// is the retest/premium end, and a low value is the chase end.
func V7ZonePositionPctClamped(zone V7PriceZone, price float64) (float64, bool) {
	pos, ok := V7ZonePositionPct(zone, price)
	if !ok {
		return 0, false
	}
	return clampFloat(pos, 0, 100), true
}
