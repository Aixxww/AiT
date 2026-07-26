package local

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

// V7ZonePositionPct returns price's raw position inside the zone as a
// percentage of the zone span: 0 = lower bound, 100 = upper bound, negative =
// below the zone, >100 = above it. Inverted bounds are repaired by swapping.
// ok is false when the zone or price cannot support the computation.
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

// V7ZonePositionPctClamped is the gating variant: positions outside the zone
// saturate at 0/100, which is what threshold comparisons like "pos < 65" mean
// by "near the lower part of the zone".
func V7ZonePositionPctClamped(zone V7PriceZone, price float64) (float64, bool) {
	pos, ok := V7ZonePositionPct(zone, price)
	if !ok {
		return 0, false
	}
	return clampFloat(pos, 0, 100), true
}
