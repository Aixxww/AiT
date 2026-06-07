package datafetch

import (
	"math"
	"sort"
)

func selectDetailSymbols(
	symbols []string,
	tickers map[string]*ticker24hrRaw,
	premiums map[string]*premiumIndexRaw,
	limit int,
) []string {
	if limit <= 0 || limit >= len(symbols) {
		out := make([]string, len(symbols))
		copy(out, symbols)
		return out
	}

	selected := make([]string, 0, limit)
	seen := make(map[string]bool, limit)
	add := func(sym string) {
		if len(selected) >= limit || seen[sym] {
			return
		}
		seen[sym] = true
		selected = append(selected, sym)
	}
	addRanked := func(quota int, metric func(string) float64) {
		if quota <= 0 || len(selected) >= limit {
			return
		}
		ranked := make([]string, len(symbols))
		copy(ranked, symbols)
		sort.SliceStable(ranked, func(i, j int) bool {
			return metric(ranked[i]) > metric(ranked[j])
		})
		added := 0
		for _, sym := range ranked {
			if added >= quota || len(selected) >= limit {
				break
			}
			if seen[sym] {
				continue
			}
			add(sym)
			added++
		}
	}

	volumeQuota := maxInt(1, int(float64(limit)*0.45))
	gainerQuota := maxInt(1, int(float64(limit)*0.22))
	loserQuota := maxInt(1, int(float64(limit)*0.18))
	fundingQuota := maxInt(1, int(float64(limit)*0.10))

	addRanked(volumeQuota, func(sym string) float64 {
		return quoteVolume(tickers[sym])
	})
	addRanked(gainerQuota, func(sym string) float64 {
		return priceChange24h(tickers[sym])
	})
	addRanked(loserQuota, func(sym string) float64 {
		return -priceChange24h(tickers[sym])
	})
	addRanked(fundingQuota, func(sym string) float64 {
		return math.Abs(fundingRate(premiums[sym]))
	})
	addRanked(limit-len(selected), func(sym string) float64 {
		return quoteVolume(tickers[sym])
	})

	return selected
}

func priceChange24h(t *ticker24hrRaw) float64 {
	if t == nil {
		return 0
	}
	return parseFloat(t.PriceChangePercent)
}

func fundingRate(p *premiumIndexRaw) float64 {
	if p == nil {
		return 0
	}
	return parseFloat(p.LastFundingRate)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
