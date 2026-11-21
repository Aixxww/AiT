package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/provider/local"
)

// Fix 04 (Tier A screening evaluation P0-4): pre-prompt repricing.
//
// The snapshot fetch takes ~3 minutes, so by prompt build the per-symbol
// prices were ~3.2 minutes old. Right before the prompt is built the
// harness makes ONE bulk ticker call and gives every prompt symbol its
// current last price; the prompt's market data, price_at_prompt_build and
// data_freshness then reflect a price seconds old, capped at
// --prompt-price-max-age (default 60 s).
//
// Scoring, the assembled (DB) tiers, RR floors and the shadow-only gate are
// untouched: repricing happens after scoring and candidate assembly, on a
// copy of the snapshot used only for the prompt. The kernel's existing
// prompt-time pass (hunterV7PromptItems: live price, live confirmations,
// re-tier) runs unchanged and now sees the fresh price; the prompt-final
// tier can therefore differ from the scoring-time one, which is that pass's
// documented purpose. Bars (klines) are not refreshed and keep their own
// ages in data_freshness.

const defaultPromptPriceMaxAge = 60 * time.Second

// repriceSummary records the repricing of one round (report field
// prompt_reprice).
type repriceSummary struct {
	Enabled     bool     `json:"enabled"`
	Symbols     int      `json:"symbols"`  // prompt symbols considered
	Repriced    int      `json:"repriced"` // given the bulk-ticker price
	Missing     []string `json:"missing,omitempty"`
	TickerAgeMs int64    `json:"ticker_age_ms"` // age of the ticker prices at prompt build
	MaxAgeMs    int64    `json:"max_age_ms"`    // the cap
	Stale       []string `json:"stale,omitempty"`
}

// promptSymbols is every symbol the prompt will show: the output signals
// (promptMarketData covers all of them) and the assembled candidates.
func promptSymbols(signals []local.V7SignalOutput, candidates []kernel.CandidateCoin) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range signals {
		add(s.Symbol)
	}
	for _, c := range candidates {
		add(c.Symbol)
	}
	sort.Strings(out)
	return out
}

// repriceForPrompt returns a copy of snap in which every prompt symbol found
// in prices carries that price, stamped fetchedAt. snap itself (used for
// scoring, persistence and tracking) is not modified.
func repriceForPrompt(snap *datafetch.Snapshot, symbols []string, prices map[string]float64, fetchedAt, now time.Time, maxAge time.Duration) (*datafetch.Snapshot, repriceSummary) {
	sum := repriceSummary{Enabled: true, Symbols: len(symbols), MaxAgeMs: maxAge.Milliseconds(), TickerAgeMs: now.Sub(fetchedAt).Milliseconds()}
	if snap == nil {
		return nil, sum
	}
	out := *snap
	out.Symbols = make(map[string]*datafetch.SymbolSnapshot, len(snap.Symbols))
	for k, v := range snap.Symbols {
		out.Symbols[k] = v
	}
	for _, sym := range symbols {
		ss, ok := snap.Symbols[sym]
		p, has := prices[sym]
		if !ok || ss == nil || !has || p <= 0 {
			sum.Missing = append(sum.Missing, sym)
			continue
		}
		cp := *ss // shallow: klines/depth are shared read-only
		cp.Price = p
		cp.Timestamp = fetchedAt
		out.Symbols[sym] = &cp
		sum.Repriced++
	}
	if now.Sub(fetchedAt) > maxAge {
		sum.Stale = append(sum.Stale, symbols...)
	} else {
		sum.Stale = append(sum.Stale, sum.Missing...)
	}
	return &out, sum
}

// applyPromptFreshness updates the candidates' data_freshness for the
// prompt: repriced symbols get the ticker age; every candidate gets the
// fixed prompt staleness cap instead of the fetch-length threshold.
func applyPromptFreshness(candidates []kernel.CandidateCoin, repriced *datafetch.Snapshot, base *datafetch.Snapshot, now time.Time, maxAge time.Duration) {
	for i := range candidates {
		c := &candidates[i]
		c.V7DataFreshness.StaleThresholdMs = maxAge.Milliseconds()
		if repriced == nil || base == nil {
			continue
		}
		nss, ok := repriced.Symbols[c.Symbol]
		if !ok || nss == nil || nss == base.Symbols[c.Symbol] { // not repriced
			continue
		}
		c.V7DataFreshness.PriceAgeMs = now.Sub(nss.Timestamp).Milliseconds()
	}
}

// repriceIssues reports prompt symbols whose price could not be refreshed
// within the cap (medium: the round stays valid, the stale prices are named).
func repriceIssues(sum repriceSummary) []issue {
	if !sum.Enabled || len(sum.Stale) == 0 {
		return nil
	}
	return []issue{{
		Severity: "medium",
		Code:     "prompt_price_stale",
		Detail: fmt.Sprintf("%d of %d prompt symbols have no price within %d ms at prompt build (ticker age %d ms): %s",
			len(sum.Stale), sum.Symbols, sum.MaxAgeMs, sum.TickerAgeMs, strings.Join(sum.Stale, ",")),
	}}
}
