// Package t2payload holds the stored Task-2 scanner payload shapes
// (spec section 9.1) shared by cmd/t2_m3_recall and the U0 harnesses.
// Moved verbatim from cmd/t2_m3_recall (names exported only).
package t2payload

import (
	"strconv"

	local "github.com/Aixxww/AiT/provider/local"
)

// ---------------------------------------------------------------------------
// Stored Payload shapes (spec section 9.1)
// ---------------------------------------------------------------------------

type KFeat struct {
	Price         float64  `json:"price"`
	ATR           float64  `json:"atr"`
	ATRPct        float64  `json:"atr_pct"`
	ATR14         float64  `json:"atr14"`
	RSI           float64  `json:"rsi"`
	BBWidth       float64  `json:"bb_width"`
	BBWidthMin50  float64  `json:"bb_width_min_50"`
	BBWidthRatio  float64  `json:"bb_width_ratio"`
	BBPercentile  float64  `json:"bb_percentile"`
	Range6Pct     float64  `json:"range_6_pct"`
	VolumeRatio   float64  `json:"volume_ratio"`
	Resistance    float64  `json:"resistance"`
	Support       float64  `json:"support"`
	PriorHigh     float64  `json:"prior_high"`
	PriorLow      float64  `json:"prior_low"`
	EMA20         *float64 `json:"ema20"`
	EMA50         *float64 `json:"ema50"`
	EMATrendPct   float64  `json:"ema_trend_pct"`
	Change3Pct    float64  `json:"change_3_pct"`
	Change12Pct   float64  `json:"change_12_pct"`
	Change24Pct   float64  `json:"change_24_pct"`
	LastCloseTime float64  `json:"last_close_time"`
}

func (k KFeat) ToFeatures() local.V7T2KlineFeatures {
	f := local.V7T2KlineFeatures{
		Price: k.Price, ATR: k.ATR, ATRPct: k.ATRPct, ATR14: k.ATR14, RSI: k.RSI,
		BBWidth: k.BBWidth, BBWidthMin50: k.BBWidthMin50, BBWidthRatio: k.BBWidthRatio,
		BBPercentile: k.BBPercentile, Range6Pct: k.Range6Pct, VolumeRatio: k.VolumeRatio,
		Resistance: k.Resistance, Support: k.Support, PriorHigh: k.PriorHigh, PriorLow: k.PriorLow,
		EMATrendPct: k.EMATrendPct, Change3Pct: k.Change3Pct, Change12Pct: k.Change12Pct,
		Change24Pct: k.Change24Pct, LastCloseTime: int64(k.LastCloseTime),
	}
	if k.EMA20 != nil && k.EMA50 != nil {
		f.EMA20 = *k.EMA20
		f.EMA50 = *k.EMA50
		f.HasEMA = true
	}
	return f
}

type RawBar struct {
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    float64 `json:"volume"`
	CloseTime float64 `json:"close_time"`
}

func (b RawBar) ToBar() local.V7T2Bar {
	return local.V7T2Bar{Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume, CloseTime: int64(b.CloseTime)}
}

type OIRow struct {
	SumOIValue string `json:"sumOpenInterestValue"`
	SumOI      string `json:"sumOpenInterest"`
}

func (r OIRow) Value() float64 {
	v, _ := strconv.ParseFloat(r.SumOIValue, 64)
	return v
}

func (r OIRow) Contracts() float64 {
	v, _ := strconv.ParseFloat(r.SumOI, 64)
	return v
}

// OISeries applies the scanner's series selection (filter the value
// series to positive entries; contract counts only if it is all-empty).
func OISeries(rows []OIRow) []float64 {
	vals := make([]float64, len(rows))
	cts := make([]float64, len(rows))
	for i, r := range rows {
		vals[i] = r.Value()
		cts[i] = r.Contracts()
	}
	return local.V7T2OIValueSeries(vals, cts)
}

type Payload struct {
	Ts          string   `json:"ts"` // scanner snapshot time (U0 native harness)
	Symbol      string   `json:"symbol"`
	Base        string   `json:"base"`
	MarketCap   float64  `json:"market_cap"`
	QuoteVolume float64  `json:"quote_volume"`
	Change24h   float64  `json:"change_24h"`
	LastPrice   float64  `json:"last_price"`
	MarkPrice   float64  `json:"mark_price"`
	FundingPct  float64  `json:"funding_pct"`
	GlobalLSR   float64  `json:"global_lsr"`
	TopLSR      float64  `json:"top_lsr"`
	K5          KFeat    `json:"k5"`
	K15         KFeat    `json:"k15"`
	K1h         KFeat    `json:"k1h"`
	K4h         KFeat    `json:"k4h"`
	Rows5       []RawBar `json:"rows5"`
	Rows1h      []RawBar `json:"rows1h"`
	Rows4h      []RawBar `json:"rows4h"`
	OIRows      []OIRow  `json:"oi_rows"`
	OI4hRows    []OIRow  `json:"oi_4h_rows"`
	OI          struct {
		Pct5m    float64 `json:"oi_5m_pct"`
		Pct15m   float64 `json:"oi_15m_pct"`
		Pct1h    float64 `json:"oi_1h_pct"`
		Z        float64 `json:"oi_z"`
		Notional float64 `json:"oi_notional_usd"`
	} `json:"oi"`
	OIAcc struct {
		Pct4h    float64 `json:"oi_4h_pct"`
		PriceChg float64 `json:"price_change_4h_pct"`
		Ampl     float64 `json:"price_amplitude_4h_pct"`
	} `json:"oi_acc"`
	TakerSignal struct {
		BuyShare       float64   `json:"buy_share"`
		BuyShare3      []float64 `json:"buy_share_3"`
		Buy3Confirmed  bool      `json:"buy_3_confirmed"`
		Sell3Confirmed bool      `json:"sell_3_confirmed"`
		RawRatio       float64   `json:"raw_ratio"`
	} `json:"taker_signal"`
	TakerRatio    float64 `json:"taker_ratio"`
	TakerTrendPct float64 `json:"taker_trend_pct"`
	Depth         struct {
		BidTotal  float64 `json:"bid_total"`
		AskTotal  float64 `json:"ask_total"`
		Ratio     float64 `json:"ratio"`
		SpreadBps float64 `json:"spread_bps"`
	} `json:"depth"`
	// Recorded outputs (expected values; never fed to the detector).
	SetupType   string   `json:"setup_type"`
	Direction   string   `json:"direction"`
	Score       float64  `json:"score"`
	Status      string   `json:"status"`
	RegimeRV24  *float64 `json:"regime_rv24"`
	ObserveOnly *bool    `json:"observe_only"`
	Version     string   `json:"strategy_version"`
}

type ExtractRow struct {
	EventID     string  `json:"event_id"`
	Symbol      string  `json:"symbol"`
	Direction   string  `json:"direction"`
	SetupType   string  `json:"setup_type"`
	Score       float64 `json:"score"`
	CreatedAt   string  `json:"created_at"`
	TriggeredAt string  `json:"triggered_at"`
	Payload     Payload `json:"Payload"`
}

func (p *Payload) ToSnapshot() *local.V7T2Snapshot {
	s := &local.V7T2Snapshot{
		Symbol:              p.Symbol,
		Base:                p.Base,
		MarketCap:           p.MarketCap,
		QuoteVolume:         p.QuoteVolume,
		Change24h:           p.Change24h,
		LastPrice:           p.LastPrice,
		MarkPrice:           p.MarkPrice,
		FundingPct:          p.FundingPct,
		GlobalLSR:           p.GlobalLSR,
		TopLSR:              p.TopLSR,
		K5:                  p.K5.ToFeatures(),
		K15:                 p.K15.ToFeatures(),
		K1h:                 p.K1h.ToFeatures(),
		K4h:                 p.K4h.ToFeatures(),
		TakerRatio:          p.TakerRatio,
		TakerTrendPct:       p.TakerTrendPct,
		OI5mAvailable:       true,
		OIPct1hFrom5mSeries: true,
	}
	for _, b := range p.Rows1h {
		s.Rows1h = append(s.Rows1h, b.ToBar())
	}
	s.OI = local.V7T2OIFeatures{
		Pct5m: p.OI.Pct5m, Pct15m: p.OI.Pct15m, Pct1h: p.OI.Pct1h, Z: p.OI.Z, NotionalUSD: p.OI.Notional,
	}
	s.OIAcc = local.V7T2OIAccFeatures{
		Pct4h: p.OIAcc.Pct4h, PriceChange4hPct: p.OIAcc.PriceChg, PriceAmplitude4hPct: p.OIAcc.Ampl,
	}
	s.Taker = local.V7T2TakerFeatures{
		BuyShare: p.TakerSignal.BuyShare, BuyShare3: p.TakerSignal.BuyShare3,
		Buy3Confirmed: p.TakerSignal.Buy3Confirmed, Sell3Confirmed: p.TakerSignal.Sell3Confirmed,
		RawRatio: p.TakerSignal.RawRatio,
	}
	s.Depth = local.V7T2DepthFeatures{
		BidTotal: p.Depth.BidTotal, AskTotal: p.Depth.AskTotal,
		Ratio: p.Depth.Ratio, SpreadBps: p.Depth.SpreadBps, Available: true,
	}
	return s
}
