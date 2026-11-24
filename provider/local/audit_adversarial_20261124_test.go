package local

// Adversarial audit of Hunter v7 (issue #69, docs/audit/adversarial-review-20261124.md).
//
// Each test characterises CURRENT behaviour at 461074e that the audit reports
// as a finding. The tests pass today on purpose (the audit changes no
// screening logic, and the suite must stay green); the assertion that encodes
// the flaw is marked "FLAW" and must be inverted by the fix.

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/Aixxww/AiT/datafetch"
)

// AUD-A2: isCompression has no Bollinger-width threshold (any bbWidth > 0
// with ADX < 25 on both majors), and it is checked before isRange. A quiet,
// non-trending market with WIDE bands is therefore labelled compression, and
// range fires only when one major has ADX >= 25 (or no data).
func TestAudit_A2_CompressionShadowsRangeAndIgnoresBandWidth(t *testing.T) {
	quietWide := regimeAnalysis{hasData: true, adx: 15, bbWidth: 25, change24h: 1, ema20_4h: 100, ema60_4h: 100}
	if !isRange(quietWide, quietWide) {
		t.Fatal("fixture must satisfy the range predicate")
	}
	if !isCompression(quietWide, quietWide) {
		t.Fatal("FLAW no longer present: compression now needs a band-width threshold; update AUD-A2")
	}
	// Range is reachable only when compression fails, e.g. one major with ADX >= 25.
	trendingETH := quietWide
	trendingETH.adx = 30
	if isCompression(quietWide, trendingETH) || !isRange(quietWide, trendingETH) {
		t.Fatal("range should only be reachable when one major's ADX >= 25")
	}
}

// AUD-A1: BuildV7Universe is the union of fixed top-N lists (volume 150,
// gainers/losers/OI/funding 50 each, velocity/activity/drawdown 80 each) plus
// side pools. Every list is filled to N even when the ranked value is 0, and
// ties are broken by map iteration order plus an unstable sort. With 420
// valid symbols that differ only in volume, the universe is ~200 symbols
// (not "all" and not the 350 cap), and its membership changes on every call.
func TestAudit_A1_UniverseSelectionIsNondeterministic(t *testing.T) {
	snap := &datafetch.Snapshot{Symbols: map[string]*datafetch.SymbolSnapshot{}}
	for i := 0; i < 420; i++ {
		sym := fmt.Sprintf("A%03dUSDT", i)
		snap.Symbols[sym] = &datafetch.SymbolSnapshot{
			Symbol:         sym,
			Price:          1,
			HighPrice24h:   1,
			LowPrice24h:    1,
			QuoteVolume24h: float64(1_000_000 + i), // distinct volumes; every other ranked field is 0
			OI:             1_000,
		}
	}
	key := func(u []V7SymbolContext) string {
		syms := make([]string, 0, len(u))
		for _, c := range u {
			syms = append(syms, c.Symbol)
		}
		sort.Strings(syms)
		return fmt.Sprintf("%d:%s", len(syms), strings.Join(syms, ","))
	}
	seen := map[string]bool{}
	sizes := map[int]bool{}
	for i := 0; i < 30; i++ {
		u := BuildV7Universe(snap)
		seen[key(u)] = true
		sizes[len(u)] = true
	}
	t.Logf("30 calls on one snapshot -> %d distinct universes (sizes %v)", len(seen), sizes)
	if len(seen) < 2 {
		t.Fatal("FLAW no longer present: universe selection is deterministic; update AUD-A1")
	}
	for n := range sizes {
		if n >= 350 {
			t.Fatalf("universe reached %d symbols; the coverage part of AUD-A1 no longer holds", n)
		}
	}
}
