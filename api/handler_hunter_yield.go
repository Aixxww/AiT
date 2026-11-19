package api

import (
	"fmt"
	"sync"
	"time"

	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/store"
)

// U13: read-only per-regime yield table on the cell dashboard. The window's
// 5-minute rounds are labelled with the BTC RV24 tier (scanner spec §6.1,
// recomputed from BTCUSDT futures klines exactly as cmd/t2_u13_yield does);
// live T2 triggered events are counted per tier and compared with the
// hunter.v7_yield_target rate. It gates nothing.

// hunterV7YieldFineTF is the forming-bar price timeframe of the RV24
// series (the U13 method).
const (
	hunterV7YieldFineTF = "5m"
	hunterV7YieldFineMs = 5 * 60_000
)

// hunterV7BTCKlineCacheTTL bounds how long fetched BTC klines are reused.
const hunterV7BTCKlineCacheTTL = 5 * time.Minute

type btcKlineCacheEntry struct {
	start, end time.Time
	at         time.Time
	bars       []store.V7PriceBar
}

var (
	btcKlineCacheMu sync.Mutex
	btcKlineCache   = map[string]btcKlineCacheEntry{}
)

// hunterV7BTCKlines fetches BTCUSDT futures klines of [start, end]
// (market.GetKlinesRange: fapi, then the data.binance.vision archive).
// A var so tests can replace the network.
var hunterV7BTCKlines = func(tf string, start, end time.Time) ([]store.V7PriceBar, error) {
	ks, err := market.GetKlinesRange("BTCUSDT", tf, start, end)
	if err != nil {
		return nil, err
	}
	out := make([]store.V7PriceBar, 0, len(ks))
	for _, k := range ks {
		out = append(out, store.V7PriceBar{OpenMs: k.OpenTime, Close: k.Close})
	}
	return out, nil
}

// cachedBTCKlines reuses a fetch of the same timeframe that covers
// [start, end] (with end within the TTL of the cached fetch).
func cachedBTCKlines(tf string, start, end time.Time) ([]store.V7PriceBar, error) {
	btcKlineCacheMu.Lock()
	e, ok := btcKlineCache[tf]
	btcKlineCacheMu.Unlock()
	if ok && !e.start.After(start) && end.Sub(e.end) < hunterV7BTCKlineCacheTTL && time.Since(e.at) < hunterV7BTCKlineCacheTTL {
		return e.bars, nil
	}
	bars, err := hunterV7BTCKlines(tf, start, end)
	if err != nil {
		return nil, err
	}
	btcKlineCacheMu.Lock()
	btcKlineCache[tf] = btcKlineCacheEntry{start: start, end: end, at: time.Now(), bars: bars}
	btcKlineCacheMu.Unlock()
	return bars, nil
}

// hunterV7YieldConfig returns the default strategy's v7_yield_target.
func (s *Server) hunterV7YieldConfig() store.V7YieldTargetConfig {
	if h := s.hunterV7DefaultHunterConfig(); h != nil {
		return store.EffectiveV7YieldTarget(h.V7YieldTarget)
	}
	return store.EffectiveV7YieldTarget(nil)
}

type hunterV7YieldTable struct {
	Available     bool               `json:"available"`
	Reason        string             `json:"reason,omitempty"`
	Stage         string             `json:"stage"`
	FormingBarTF  string             `json:"forming_bar_tf"`
	RoundMinutes  int                `json:"round_minutes"`
	CILevel       float64            `json:"ci_level"`
	T2Records     int                `json:"t2_records"`
	Events        int                `json:"events"`
	RoundsPerTier map[string]int     `json:"rounds_per_tier"`
	Rows          []store.V7YieldRow `json:"rows"`
}

func (s *Server) hunterV7YieldTable(from, to time.Time) hunterV7YieldTable {
	cfg := s.hunterV7YieldConfig()
	out := hunterV7YieldTable{Stage: store.V7YieldStageT2Triggered, FormingBarTF: hunterV7YieldFineTF,
		RoundMinutes: cfg.RoundMinutes, CILevel: cfg.CILevel, RoundsPerTier: map[string]int{}, Rows: []store.V7YieldRow{}}
	recs, err := s.store.HunterV7Signal().T2YieldSource(from, to)
	if err != nil {
		out.Reason = "t2 record query failed"
		return out
	}
	out.T2Records = len(recs)
	if len(recs) == 0 {
		out.Reason = "no T2 records in the window (T2 module off?)"
		return out
	}
	// RV24 needs RV24Bars-1 closed 1h bars before the first round.
	h1, err := cachedBTCKlines("1h", from.Add(-time.Duration(cfg.RV24Bars+1)*time.Hour), to)
	if err != nil {
		out.Reason = fmt.Sprintf("BTC 1h klines unavailable: %v", err)
		return out
	}
	fine, err := cachedBTCKlines(hunterV7YieldFineTF, from.Add(-time.Hour), to)
	if err != nil {
		out.Reason = fmt.Sprintf("BTC %s klines unavailable: %v", hunterV7YieldFineTF, err)
		return out
	}
	series := store.NewV7RV24Series(cfg, h1, fine, hunterV7YieldFineMs)
	out.RoundsPerTier = series.RoundsPerTier(from.UnixMilli(), to.UnixMilli())
	counts := map[string]int{}
	for _, ev := range store.V7T2TriggeredEvents(recs) {
		counts[series.Tier(ev.Time.UnixMilli())]++
		out.Events++
	}
	for _, tier := range store.V7RV24Tiers {
		out.Rows = append(out.Rows, store.BuildV7YieldRow(out.Stage, tier, counts[tier], out.RoundsPerTier[tier], cfg))
	}
	if n := counts[store.V7RV24TierNone]; n > 0 {
		out.Rows = append(out.Rows, store.BuildV7YieldRow(out.Stage, store.V7RV24TierNone, n, out.RoundsPerTier[store.V7RV24TierNone], cfg))
	}
	out.Available = true
	return out
}
