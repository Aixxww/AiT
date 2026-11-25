package kernel

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/logger"
	"github.com/Aixxww/AiT/market"
)

// #87 L4 finalist refresh (docs/audit/optimization-design-20261124.md
// cluster 3). Immediately before the prompt is built, the top Hunter v7
// candidates get a fresh REST fetch of the latest price and the fast
// indicators (ticker, premium index, OI, 1m/5m/15m/1h klines) through the
// collector's RefreshSymbol path, under one hard deadline. The prompt-time
// re-tier then runs on that data instead of the up-to-30s-old round
// snapshot. A symbol whose refresh fails or misses the deadline keeps its
// snapshot values, and the age of those values is stamped.

const (
	// hunterV7FinalistCount is how many candidates are refreshed (3-5).
	hunterV7FinalistCount = 5
)

// hunterV7FinalistDeadline bounds the whole refresh (all finalists in
// parallel); requests carry the deadline, so it also cancels in flight.
// A variable only so tests can shorten it.
var hunterV7FinalistDeadline = 5 * time.Second

// HunterV7FinalistRefresh is one finalist's refresh outcome.
type HunterV7FinalistRefresh struct {
	Symbol    string        `json:"symbol"`
	Refreshed bool          `json:"refreshed"`
	DataAge   time.Duration `json:"data_age"` // age of the price the prompt uses, at refresh end
	Err       string        `json:"error,omitempty"`
}

// hunterV7FinalistTierRank orders finalists by their assembly tier: the
// final tier needs the very data being refreshed, so the selection uses the
// tier the candidate arrived with.
func hunterV7FinalistTierRank(tier string) int {
	switch strings.ToUpper(tier) {
	case "EXECUTABLE":
		return 0
	case "REVIEWABLE":
		return 1
	case "WATCH":
		return 2
	default:
		return 3
	}
}

// selectHunterV7Finalists returns up to n candidate symbols that have market
// data and no open position, best first (tier, then AI priority).
func selectHunterV7Finalists(ctx *Context, n int) []string {
	held := make(map[string]bool, len(ctx.Positions))
	for _, p := range ctx.Positions {
		held[market.Normalize(p.Symbol)] = true
	}
	type cand struct {
		sym      string
		rank     int
		priority float64
	}
	seen := map[string]bool{}
	var cands []cand
	for _, c := range ctx.CandidateCoins {
		if c.V7SetupType == "" || held[market.Normalize(c.Symbol)] || seen[c.Symbol] {
			continue
		}
		if _, ok := ctx.MarketDataMap[c.Symbol]; !ok {
			continue
		}
		seen[c.Symbol] = true
		cands = append(cands, cand{c.Symbol, hunterV7FinalistTierRank(c.V7ExecutionTier), c.V7AIPriority})
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].rank != cands[j].rank {
			return cands[i].rank < cands[j].rank
		}
		return cands[i].priority > cands[j].priority
	})
	if len(cands) > n {
		cands = cands[:n]
	}
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.sym
	}
	return out
}

// refreshHunterV7Finalists runs the L4 refresh and rebuilds the finalists'
// market data from the refreshed snapshots. No-op without a live collector
// (backtests, one-shot engines, tests) or for non-hunter_v7 strategies.
func refreshHunterV7Finalists(ctx *Context, engine *StrategyEngine, timeframes []string, primaryTimeframe string, klineCount int) {
	if ctx == nil || engine == nil || !strings.EqualFold(engine.GetConfig().CoinSource.SourceType, "hunter_v7") {
		return
	}
	se := engine.GetSnapshotEngine()
	if se == nil || se.source == nil || se.source.collector == nil {
		return
	}
	finalists := selectHunterV7Finalists(ctx, hunterV7FinalistCount)
	if len(finalists) == 0 {
		return
	}
	start := time.Now()
	rctx, cancel := context.WithTimeout(context.Background(), hunterV7FinalistDeadline)
	defer cancel()
	results, err := se.RefreshSymbolSnapshots(rctx, finalists, datafetch.FastKlineIntervals)
	if err != nil {
		logger.Infof("⚠ hunter_v7 finalist refresh unavailable: %v", err)
		return
	}
	end := time.Now()
	refreshed := 0
	ctx.HunterV7Finalists = make([]HunterV7FinalistRefresh, 0, len(results))
	for _, r := range results {
		out := HunterV7FinalistRefresh{Symbol: r.Symbol}
		if r.Snap != nil && r.Err == nil {
			data, berr := market.BuildDataFromSymbolSnapshot(market.Normalize(r.Symbol), r.Snap, timeframes, primaryTimeframe, klineCount)
			if berr == nil && data != nil {
				ctx.MarketDataMap[r.Symbol] = data
				out.Refreshed = true
				out.DataAge = end.Sub(r.At)
				refreshed++
			} else if berr != nil {
				out.Err = berr.Error()
			}
		} else if r.Err != nil {
			out.Err = r.Err.Error()
		}
		if !out.Refreshed {
			// Snapshot fallback: stamp the age of the price the prompt uses.
			out.DataAge = hunterV7SnapshotPriceAge(se.GetSnapshot(), r.Symbol, end)
		}
		ctx.HunterV7Finalists = append(ctx.HunterV7Finalists, out)
	}
	logger.Infof("📡 hunter_v7 finalist refresh: %d/%d refreshed in %v (deadline %v)",
		refreshed, len(results), end.Sub(start).Round(time.Millisecond), hunterV7FinalistDeadline)
	for _, f := range ctx.HunterV7Finalists {
		if !f.Refreshed {
			logger.Infof("⚠ hunter_v7 finalist %s uses snapshot data (age %v): %s", f.Symbol, f.DataAge.Round(time.Millisecond), f.Err)
		}
	}
}

func hunterV7SnapshotPriceAge(snap *datafetch.Snapshot, symbol string, at time.Time) time.Duration {
	if snap == nil {
		return 0
	}
	ss := snap.Symbols[market.Normalize(symbol)]
	if ss == nil {
		ss = snap.Symbols[symbol]
	}
	if ss == nil {
		return 0
	}
	t := ss.PriceAt
	if t.IsZero() {
		t = ss.Timestamp
	}
	if t.IsZero() {
		return 0
	}
	return at.Sub(t)
}
