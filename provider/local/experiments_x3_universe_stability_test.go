package local

// Experiment X3 (issue #80, audit docs/audit/adversarial-review-20261124.md §6):
// universe stability. Measurement only: this file asserts nothing about the
// desired behaviour and changes no production code. It is skipped unless
// AIT_EXPERIMENTS=1 so the normal suite is unaffected.
//
// The audit asks for "one stored snapshot". No stored full-market snapshot
// exists in the repository, so three inputs are measured and labelled:
//   S1  SYNTHETIC: the AUD-A1 fixture (420 symbols, only volume differs).
//   S2  SYNTHETIC: 450 symbols with seeded, mostly distinct ranking values
//       and the ties a real snapshot has (funding pinned at 0.0001 for most
//       symbols, OI delta 0 for symbols outside the detail fetch).
//   S3  FIXTURE-DERIVED: the 201 symbols of testdata/golden/universe-20260726
//       turned back into a snapshot. This is real market data, but it is the
//       output of a previous universe build (not the full market) and has no
//       klines, so velocity / activity / range pools are inert.
//
// Run: AIT_EXPERIMENTS=1 go test ./provider/local -run TestExperimentX3 -v -count=1

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Aixxww/AiT/datafetch"
)

func requireExperiments(t *testing.T) {
	t.Helper()
	if os.Getenv("AIT_EXPERIMENTS") != "1" {
		t.Skip("measurement-only experiment; set AIT_EXPERIMENTS=1 to run")
	}
}

type x3Result struct {
	runs, valid, distinct, minSize, maxSize int
	meanJaccard, minJaccard                 float64
	alwaysIn, everIn                        int
}

func x3Measure(snap *datafetch.Snapshot, runs int) x3Result {
	sets := make([]map[string]bool, 0, runs)
	keys := map[string]bool{}
	res := x3Result{runs: runs, minSize: 1 << 30}
	count := map[string]int{}
	for i := 0; i < runs; i++ {
		u := BuildV7Universe(snap)
		s := make(map[string]bool, len(u))
		syms := make([]string, 0, len(u))
		for _, c := range u {
			s[c.Symbol] = true
			syms = append(syms, c.Symbol)
			count[c.Symbol]++
		}
		sort.Strings(syms)
		keys[fmt.Sprint(syms)] = true
		sets = append(sets, s)
		if len(u) < res.minSize {
			res.minSize = len(u)
		}
		if len(u) > res.maxSize {
			res.maxSize = len(u)
		}
	}
	res.distinct = len(keys)
	res.everIn = len(count)
	for _, n := range count {
		if n == runs {
			res.alwaysIn++
		}
	}
	pairs, sum := 0, 0.0
	res.minJaccard = 1
	for i := 0; i < len(sets); i++ {
		for j := i + 1; j < len(sets); j++ {
			inter := 0
			for k := range sets[i] {
				if sets[j][k] {
					inter++
				}
			}
			union := len(sets[i]) + len(sets[j]) - inter
			jac := 1.0
			if union > 0 {
				jac = float64(inter) / float64(union)
			}
			sum += jac
			pairs++
			if jac < res.minJaccard {
				res.minJaccard = jac
			}
		}
	}
	if pairs > 0 {
		res.meanJaccard = sum / float64(pairs)
	}
	for sym, ss := range snap.Symbols {
		if isUSDTPerp(sym) && !excludedTokenizedAssets[sym] && ss.QuoteVolume24h > 0 && ss.OI > 0 {
			res.valid++
		}
	}
	return res
}

func (r x3Result) log(t *testing.T, label string) {
	t.Logf("X3 %s: runs=%d valid=%d distinct_sets=%d size=%d..%d coverage=%.1f%%..%.1f%% jaccard_mean=%.4f jaccard_min=%.4f always_in=%d ever_in=%d",
		label, r.runs, r.valid, r.distinct, r.minSize, r.maxSize,
		100*float64(r.minSize)/float64(max(r.valid, 1)), 100*float64(r.maxSize)/float64(max(r.valid, 1)),
		r.meanJaccard, r.minJaccard, r.alwaysIn, r.everIn)
}

func TestExperimentX3_UniverseStability(t *testing.T) {
	requireExperiments(t)
	const runs = 50

	// S1: AUD-A1 fixture.
	s1 := &datafetch.Snapshot{Symbols: map[string]*datafetch.SymbolSnapshot{}}
	for i := 0; i < 420; i++ {
		sym := fmt.Sprintf("A%03dUSDT", i)
		s1.Symbols[sym] = &datafetch.SymbolSnapshot{Symbol: sym, Price: 1, HighPrice24h: 1, LowPrice24h: 1,
			QuoteVolume24h: float64(1_000_000 + i), OI: 1_000}
	}
	x3Measure(s1, runs).log(t, "S1[SYNTHETIC audit-A1 420 syms]")

	// S2: seeded synthetic with realistic ties.
	rng := rand.New(rand.NewSource(20261124))
	s2 := &datafetch.Snapshot{Symbols: map[string]*datafetch.SymbolSnapshot{}}
	for i := 0; i < 450; i++ {
		sym := fmt.Sprintf("B%03dUSDT", i)
		price := 1 + rng.Float64()*10
		chg := rng.NormFloat64() * 4
		ss := &datafetch.SymbolSnapshot{Symbol: sym, Price: price,
			PriceChange24h: chg,
			HighPrice24h:   price * (1 + rng.Float64()*0.08),
			LowPrice24h:    price * (1 - rng.Float64()*0.06),
			QuoteVolume24h: 1e6 * (1 + rng.ExpFloat64()*50),
			OI:             1e5 * (1 + rng.Float64()),
			FundingRate:    0.0001,
		}
		if rng.Float64() < 0.25 {
			ss.FundingRate = rng.NormFloat64() * 0.0005
		}
		if i < 200 { // detail-fetched symbols only
			ss.OIDelta1h = rng.NormFloat64() * 2
		}
		s2.Symbols[sym] = ss
	}
	x3Measure(s2, runs).log(t, "S2[SYNTHETIC seeded 450 syms, funding/OI ties]")

	// S3: golden fixture turned back into a snapshot.
	fx, err := LoadV7GoldenFixture(filepath.Join("testdata", "golden", "universe-20260726.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	s3 := &datafetch.Snapshot{Symbols: map[string]*datafetch.SymbolSnapshot{}}
	for _, c := range fx.Universe {
		if c.Snapshot == nil {
			continue
		}
		d := c.Snapshot
		s3.Symbols[c.Symbol] = &datafetch.SymbolSnapshot{Symbol: c.Symbol, Price: d.Price,
			PriceChange24h: d.PriceChange24h, Volume24h: d.Volume24h, QuoteVolume24h: d.QuoteVolume24h,
			HighPrice24h: d.HighPrice24h, LowPrice24h: d.LowPrice24h, TradeCount24h: d.TradeCount24h,
			FundingRate: d.FundingRate, OI: d.OI, OIDelta1h: d.OIDelta1h, OIDelta4h: d.OIDelta4h,
			LongShortRatio: d.LSR, LSRPrev: d.LSRPrev, LSROldest: d.LSROldest, TakerBuyRatio: d.TakerBuy}
	}
	x3Measure(s3, runs).log(t, fmt.Sprintf("S3[FIXTURE-DERIVED universe-20260726 %d syms, no klines]", len(s3.Symbols)))
}
