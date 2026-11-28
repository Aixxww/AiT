package local

import (
	"sort"
	"time"

	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/store"
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
//   - Order-book depth: when the datafetch snapshot carries a book
//     (fetched only with datafetch's IncludeT2Inputs), the raw levels go
//     through the detector's own BuildV7T2DepthFeatures referenced to
//     the mark price — the scanner's exact construction (spec sections
//     2.1/2.7). Without a book, Depth.Available stays false and every
//     depth-reading gate fails. (datafetch's Spread field is a
//     mark/index basis, NOT the book spread; it is never substituted.)
//   - 5m OI series: when the snapshot carries the 5m openInterestHist
//     series, oi_5m_pct / oi_15m_pct / oi_1h_pct are computed from it by
//     BuildV7T2OIFeatures after the scanner's series selection (spec
//     section 2.4). The series counts as available only with >= 13
//     selected points: oi_1h_pct looks back 12 bars, and upstream a
//     shorter series died inside oi_features (index error -> snapshot
//     skipped), so a short series fails closed here instead of reading
//     fabricated zeros. Without the series, oi_1h_pct falls back to the
//     hourly figure and the 5m/15m gates stay failed. oi_4h_pct is
//     compounded from the 13 hourly spike changes over the last 7
//     intervals, matching the scanner's 8-point 1h window (spec
//     section 2.5) — compounding consecutive hourly changes of the same
//     value series is the endpoint ratio, not an approximation of a
//     different quantity.
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
	// U10: a closed-bar view keeps the live price for every frame's price
	// (the forming bar is used for price only).
	if ss.ClosedBarsOnly && ss.Price > 0 {
		s.K5.Price, s.K15.Price, s.K1h.Price, s.K4h.Price = ss.Price, ss.Price, ss.Price, ss.Price
	}

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

	// OI (see header for the availability semantics).
	price := ss.MarkPrice
	if price <= 0 {
		price = ss.Price
	}
	if series := v7t2OI5mSeriesFromDatafetch(ss.OI5mHistory); len(series) >= 13 {
		s.OI = BuildV7T2OIFeatures(series)
		s.OI5mAvailable = true
		s.OIPct1hFrom5mSeries = true
	} else {
		s.OI.NotionalUSD = ss.OI * price
		s.OI.Pct1h = ss.OIDelta1h
		s.OI5mAvailable = false
		s.OIPct1hFrom5mSeries = false
	}
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
	// Depth (see header): raw book through the detector's own builder,
	// referenced to the mark price (fallback: last price), as upstream.
	if len(ss.DepthBids) > 0 && len(ss.DepthAsks) > 0 {
		bids := make([]V7T2DepthLevel, len(ss.DepthBids))
		for i, l := range ss.DepthBids {
			bids[i] = V7T2DepthLevel{Price: l.Price, Qty: l.Qty}
		}
		asks := make([]V7T2DepthLevel, len(ss.DepthAsks))
		for i, l := range ss.DepthAsks {
			asks[i] = V7T2DepthLevel{Price: l.Price, Qty: l.Qty}
		}
		s.Depth = BuildV7T2DepthFeatures(bids, asks, price)
	}
	return s
}

// v7t2OI5mSeriesFromDatafetch splits the fetched 5m OI history into the
// notional-value and contract-count series and applies the scanner's
// selection (spec section 2.4, via V7T2OIValueSeries).
func v7t2OI5mSeriesFromDatafetch(hist []datafetch.OIHistoryPoint) []float64 {
	if len(hist) == 0 {
		return nil
	}
	vals := make([]float64, len(hist))
	cts := make([]float64, len(hist))
	for i, p := range hist {
		vals[i] = p.SumOpenInterestValue
		cts[i] = p.SumOpenInterest
	}
	return V7T2OIValueSeries(vals, cts)
}

// RunV7T2Module evaluates the transplanted detector over every symbol of
// a datafetch snapshot. When store is non-nil, emitted signals pass
// through the lifecycle upsert (cooldown / terminal / watch_hits rules)
// and only non-dropped signals are returned. now is the scan time.
func RunV7T2Module(snap *datafetch.Snapshot, cfg *V7T2Config, store *V7T2EventStore, regime *V7T2RegimeInput, v7Regime V7MarketRegime, now time.Time) []V7SignalOutput {
	if snap == nil || cfg == nil || !cfg.Enabled {
		return nil
	}
	if store != nil {
		store.Configure(cfg.Lifecycle)
		// Spec section 1.5: live events of activity-listed symbols are
		// invalidated.
		for _, sym := range cfg.ActivitySymbols {
			store.InvalidateActivity(sym, now)
		}
		// Spec section 5.3: outcomes run at the START of each scan with
		// the latest price (U1/P1).
		prices := make(map[string]float64, len(snap.Symbols))
		for sym, ss := range snap.Symbols {
			if ss != nil && ss.Price > 0 {
				prices[sym] = ss.Price
			}
		}
		store.UpdateOutcomes(prices, now)
	}
	// Deterministic order (map iteration would vary run to run).
	syms := make([]string, 0, len(snap.Symbols))
	for sym := range snap.Symbols {
		syms = append(syms, sym)
	}
	sort.Strings(syms)
	var out []V7SignalOutput
	emitted := map[string]int{} // event id -> index in out
	for _, sym := range syms {
		ts := BuildV7T2SnapshotFromDatafetch(snap.Symbols[sym])
		if ts == nil {
			continue
		}
		sig := EvaluateV7T2Snapshot(ts, cfg, regime)
		if sig == nil {
			continue
		}
		state := sig.EffectiveStatus()
		eventID := ""
		if store != nil {
			res := store.Upsert(sig, ts.Price(), now)
			if res.Dropped {
				continue
			}
			state = res.Event.Status
			emitted[res.Event.EventID] = len(out)
			eventID = res.Event.EventID
		}
		o := sig.ToV7SignalOutputWithLifecycle(v7Regime, state)
		o.T2EventID = eventID // fix 11
		out = append(out, o)
	}
	// U1/P1: freshly TRIGGERED events reach the pipeline whether or not
	// the detector fired again on this scan.
	if store != nil {
		for _, ev := range store.FreshlyTriggered(now) {
			o := ev.Signal.ToV7SignalOutputWithLifecycle(v7Regime, "TRIGGERED")
			o.T2EventID = ev.EventID // fix 11
			if i, ok := emitted[ev.EventID]; ok {
				out[i] = o
				continue
			}
			out = append(out, o)
		}
	}
	// TA-1: every emitted output, including the re-emitted TRIGGERED
	// events above, carries the signal-contract context fields.
	for i := range out {
		ApplyV7T2ContractFields(&out[i], snap)
	}
	if cfg.SetupPrefix {
		v7T2PrefixSetupTypes(out)
	}
	return out
}

// V7T2DataGapContextUnavailable marks a T2 output whose symbol is missing
// from the snapshot, so its price/derivatives context is empty (TA-1).
const V7T2DataGapContextUnavailable = "t2_context_unavailable"

// v7T2LivePriceConfirmation is the confirmation a non-immediate T2 output
// waits for: the event triggers when price reaches the entry (U1, spec
// section 5.3), which the native confirmation evaluator knows as
// live_price_in_entry_zone.
const v7T2LivePriceConfirmation = "live_price_in_entry_zone"

// ApplyV7T2ContractFields fills the signal-contract fields the Task-2
// converter cannot (it has no snapshot): price_context, derivatives_context,
// liquidity_score and quote_volume_24h, built from the symbol's datafetch
// snapshot exactly as native modules build them (buildSymbolContext ->
// buildPriceCtx / buildDerivCtx / AssessLiquidityScore), and
// required_confirmations for outputs whose entry is not immediate. A symbol
// absent from the snapshot gets empty contexts, zero liquidity/volume, plus
// the t2_context_unavailable data gap: values are never fabricated. Score,
// status, entry mode, targets and lifecycle state are not touched.
//
// price_context.last is the scan price (ss.Price, as native modules carry
// it; fix 06). It used to be left 0 because the kernel measured backend RR
// and stop distance from PriceCtx.Last when set, which moved 16 TRIGGERED
// tiers in the TA-1 golden replay. The kernel now measures T2 RR from
// CandidateCoin.V7RRReferencePrice (the entry-zone edge, the old fallback)
// before Last, so the RR geometry is unchanged.
// Exported for the offline TA-1 golden replay.
func ApplyV7T2ContractFields(out *V7SignalOutput, snap *datafetch.Snapshot) {
	if out == nil {
		return
	}
	var ss *datafetch.SymbolSnapshot
	if snap != nil {
		ss = snap.Symbols[out.Symbol]
	}
	if ss == nil {
		out.PriceCtx = &V7PriceContext{}
		out.DerivativesCtx = &V7DerivativesContext{}
		out.DataGaps = appendIfMissing(out.DataGaps, V7T2DataGapContextUnavailable)
	} else {
		ctx := buildSymbolContext(out.Symbol, ss, snap)
		out.PriceCtx = buildPriceCtx(ctx)
		out.DerivativesCtx = buildDerivCtx(ctx)
		out.LiquidityScore = AssessLiquidityScore(ctx)
		out.QuoteVolume24h = ss.QuoteVolume24h
	}
	if out.EntryMode != V7EntryImmediate {
		out.RequiredConfirms = appendIfMissing(out.RequiredConfirms, v7T2LivePriceConfirmation)
	}
}

// v7T2PrefixSetupTypes renames emitted setup types to t2_<setup> (U11,
// D2 (a)). Signal IDs are assigned after this, from the emitted name.
func v7T2PrefixSetupTypes(out []V7SignalOutput) {
	for i := range out {
		out[i].SetupType = V7SetupType(store.V7T2SetupPrefix + store.V7CanonicalSetupType(string(out[i].SetupType)))
	}
}
