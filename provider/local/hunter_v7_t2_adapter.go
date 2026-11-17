package local

import (
	"time"

	"github.com/Aixxww/AiT/datafetch"
)

// ============================================================================
// Hunter v7 — Task-2 module adapter (M3)
// ============================================================================
// Builds V7T2Snapshot inputs from the datafetch snapshot the v7 pipeline
// already assembles, and runs the transplanted detector over a full
// snapshot. The detector itself is input-complete; this adapter is where
// the host snapshot's gaps live. They are handled by marking the series
// unavailable (gates that need it then fail) — never by substituting a
// different signal:
//
//   - Order-book depth: datafetch carries no depth book. Depth.Available
//     stays false, so every depth-reading gate fails and the module emits
//     nothing on live snapshots until datafetch grows a depth feed.
//     (datafetch's Spread field is a mark/index basis, NOT the book
//     spread; using it would be a silent signal substitution.)
//   - 5m OI series: datafetch carries hourly OI deltas only. oi_5m_pct /
//     oi_15m_pct stay unavailable (OI5mAvailable=false); oi_1h_pct is the
//     hourly figure and oi_4h_pct is compounded from the 13 hourly spike
//     changes over the last 7 intervals, matching the scanner's 8-point
//     1h window (spec section 2.5).
//   - Global long/short account ratio: datafetch's LongShortRatio is the
//     TOP-position ratio (the scanner's top_lsr, which upstream fetched
//     but never gated on). It is carried as TopLSR; GlobalLSR takes the
//     scanner's own fetch-failure default of 1.0 (spec section 9.1).
//   - Market cap: not present in datafetch; payload-only field, no gate
//     consumes it, carried as 0.
//   - Taker series: rebuilt from per-bar taker-buy volume in the 15m
//     klines (share = takerBuy/volume). This is the same quantity the
//     scanner derives as ratio/(1+ratio) from the takerlongshortRatio
//     endpoint, modulo the endpoint's own sampling.

func v7t2BarsFromKlines(ks []datafetch.Kline) []V7T2Bar {
	bars := make([]V7T2Bar, 0, len(ks))
	for _, k := range ks {
		b := V7T2Bar{
			Open:      k.Open,
			High:      k.High,
			Low:       k.Low,
			Close:     k.Close,
			Volume:    k.Volume,
			CloseTime: k.CloseTime,
		}
		if k.Volume > 0 {
			b.TakerBuyShare = k.TakerBuy / k.Volume
		}
		bars = append(bars, b)
	}
	return bars
}

// BuildV7T2SnapshotFromDatafetch adapts one datafetch symbol snapshot.
func BuildV7T2SnapshotFromDatafetch(ss *datafetch.SymbolSnapshot) *V7T2Snapshot {
	if ss == nil {
		return nil
	}
	s := &V7T2Snapshot{
		Symbol:      ss.Symbol,
		QuoteVolume: ss.QuoteVolume24h,
		Change24h:   ss.PriceChange24h,
		LastPrice:   ss.Price,
		MarkPrice:   ss.MarkPrice,
		FundingPct:  ss.FundingRate * 100,
		GlobalLSR:   1.0, // scanner fetch-failure default; see header
		TopLSR:      ss.LongShortRatio,
	}
	if len(ss.Symbol) > 4 && ss.Symbol[len(ss.Symbol)-4:] == "USDT" {
		s.Base = ss.Symbol[:len(ss.Symbol)-4]
	}
	rows5 := v7t2BarsFromKlines(ss.Klines["5m"])
	rows15 := v7t2BarsFromKlines(ss.Klines["15m"])
	rows1h := v7t2BarsFromKlines(ss.Klines["1h"])
	rows4h := v7t2BarsFromKlines(ss.Klines["4h"])
	s.K5 = BuildV7T2KlineFeatures(rows5)
	s.K15 = BuildV7T2KlineFeatures(rows15)
	s.K1h = BuildV7T2KlineFeatures(rows1h)
	s.K4h = BuildV7T2KlineFeatures(rows4h)
	s.Rows1h = rows1h

	// Taker series from the last 15m bars' taker-buy shares.
	var shares []float64
	for _, b := range rows15 {
		shares = append(shares, b.TakerBuyShare)
	}
	if len(shares) > 5 {
		shares = shares[len(shares)-5:]
	}
	s.Taker = BuildV7T2TakerFeaturesFromShares(shares)
	s.TakerRatio = s.Taker.RawRatio
	if len(shares) >= 3 {
		ratios := make([]float64, len(shares))
		for i, sh := range shares {
			if sh >= 1 {
				ratios[i] = 999
			} else if sh > 0 {
				ratios[i] = sh / (1 - sh)
			}
		}
		prevMean := v7T2Mean(ratios[:len(ratios)-1])
		if prevMean > 0 {
			s.TakerTrendPct = (ratios[len(ratios)-1]/prevMean - 1) * 100
		}
	}

	// OI: hourly-only in datafetch (see header for the gap semantics).
	price := ss.MarkPrice
	if price <= 0 {
		price = ss.Price
	}
	s.OI.NotionalUSD = ss.OI * price
	s.OI.Pct1h = ss.OIDelta1h
	s.OI5mAvailable = false
	s.OIPct1hFrom5mSeries = false
	// oi_acc: price window from 1h bars; Pct4h compounded from spike data.
	s.OIAcc = BuildV7T2OIAccFeatures(nil, rows1h)
	if len(ss.OISpikeData) > 0 {
		spikes := ss.OISpikeData
		if len(spikes) > 7 {
			spikes = spikes[len(spikes)-7:]
		}
		prod := 1.0
		for _, p := range spikes {
			prod *= 1 + p/100
		}
		s.OIAcc.Pct4h = (prod - 1) * 100
	}
	// Depth stays unavailable (see header).
	s.Depth.Available = false
	return s
}

// RunV7T2Module evaluates the transplanted detector over every symbol of
// a datafetch snapshot. When store is non-nil, emitted signals pass
// through the lifecycle upsert (cooldown / terminal / watch_hits rules)
// and only non-dropped signals are returned. now is the scan time.
func RunV7T2Module(snap *datafetch.Snapshot, cfg *V7T2Config, store *V7T2EventStore, regime *V7T2RegimeInput, v7Regime V7MarketRegime, now time.Time) []V7SignalOutput {
	if snap == nil || cfg == nil || !cfg.Enabled {
		return nil
	}
	var out []V7SignalOutput
	for _, ss := range snap.Symbols {
		ts := BuildV7T2SnapshotFromDatafetch(ss)
		if ts == nil {
			continue
		}
		sig := EvaluateV7T2Snapshot(ts, cfg, regime)
		if sig == nil {
			continue
		}
		if store != nil {
			res := store.Upsert(sig, ts.Price(), now)
			if res.Dropped {
				continue
			}
		}
		out = append(out, sig.ToV7SignalOutput(v7Regime))
	}
	return out
}
