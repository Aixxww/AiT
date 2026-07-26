// Package datafetch provides the unified data collection layer for AiT.
// It fetches market data from Binance Futures (REST + WebSocket) and
// LunarCrush social metrics, then atomically publishes snapshots.
package datafetch

import "time"

// SymbolSnapshot holds ALL fetched data for a single symbol.
type SymbolSnapshot struct {
	Symbol    string
	Price     float64
	Timestamp time.Time

	// From ticker/24hr
	PriceChange24h float64
	Volume24h      float64
	QuoteVolume24h float64
	HighPrice24h   float64
	LowPrice24h    float64
	TradeCount24h  int64

	// From premiumIndex (bulk)
	MarkPrice       float64
	IndexPrice      float64
	FundingRate     float64
	NextFundingTime int64
	Spread          float64 // (markPrice - indexPrice) / indexPrice * 100

	// Per-symbol data (fetched individually for top N)
	OI             float64   // Current open interest quantity from Binance openInterest
	OIDelta1h      float64   // OI change rate over 1h (%)
	OIDelta4h      float64   // OI change rate over 4h (%)
	OISpikeData    []float64 // 13 hourly OI period-over-period % changes
	LongShortRatio float64   // Latest LSR
	LSRPrev        float64   // Previous period LSR
	LSROldest      float64   // Oldest LSR (from 12-entry fetch, for Hunter reversal detection)
	TakerBuyRatio  float64   // Taker buy ratio 0-1

	// Klines by timeframe: "1m","5m","15m","1h","4h","1d"
	Klines map[string][]Kline

	// LunarCrush social data
	Social SocialData
}

// Kline represents a single candlestick bar.
type Kline struct {
	OpenTime  int64
	CloseTime int64
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	TakerBuy  float64
}

// SocialData holds LunarCrush social metrics for a symbol.
type SocialData struct {
	HeatScore      float64   // 0-100 composite score
	Sentiment      float64   // 0-100 (>50 = positive)
	SentimentDelta float64   // sentiment change
	SocialVolume   int       // 24h social mentions
	VolumeChange   float64   // social volume change %
	KOLCount       int       // key opinion leader count
	GalaxyScore    float64   // LunarCrush galaxy score
	AltRank        int       // alt rank
	UpdatedAt      time.Time // when data was fetched
}

// Snapshot is a point-in-time capture of all symbol data.
type Snapshot struct {
	Symbols   map[string]*SymbolSnapshot
	Meta      SnapshotMeta
	CreatedAt time.Time
}

// SnapshotMeta holds metadata about how the snapshot was built.
type SnapshotMeta struct {
	SymbolCount   int
	FetchDuration time.Duration
	RestErrors    int
	WSConnected   bool
	SocialFresh   bool
}

// ---------------------------------------------------------------------------
// Binance API raw response structs
// ---------------------------------------------------------------------------

// ticker24hrRaw is the raw response from GET /fapi/v1/ticker/24hr.
type ticker24hrRaw struct {
	Symbol             string `json:"symbol"`
	PriceChange        string `json:"priceChange"`
	PriceChangePercent string `json:"priceChangePercent"`
	LastPrice          string `json:"lastPrice"`
	HighPrice          string `json:"highPrice"`
	LowPrice           string `json:"lowPrice"`
	Volume             string `json:"volume"`
	QuoteVolume        string `json:"quoteVolume"`
	OpenTime           int64  `json:"openTime"`
	CloseTime          int64  `json:"closeTime"`
	Count              int64  `json:"count"`
}

// premiumIndexRaw is the raw response from GET /fapi/v1/premiumIndex.
type premiumIndexRaw struct {
	Symbol               string `json:"symbol"`
	MarkPrice            string `json:"markPrice"`
	IndexPrice           string `json:"indexPrice"`
	EstimatedSettlePrice string `json:"estimatedSettlePrice"`
	LastFundingRate      string `json:"lastFundingRate"`
	NextFundingTime      int64  `json:"nextFundingTime"`
	InterestRate         string `json:"interestRate"`
	Time                 int64  `json:"time"`
}

// exchangeInfoRaw is the raw response from GET /fapi/v1/exchangeInfo.
type exchangeInfoRaw struct {
	Symbols []exchangeSymbolRaw `json:"symbols"`
}

// exchangeSymbolRaw is a single symbol entry in exchangeInfo.
type exchangeSymbolRaw struct {
	Symbol            string   `json:"symbol"`
	Status            string   `json:"status"`
	ContractType      string   `json:"contractType"`
	BaseAsset         string   `json:"baseAsset"`
	UnderlyingType    string   `json:"underlyingType"`
	UnderlyingSubType []string `json:"underlyingSubType"`
}

// oiRaw is the raw response from GET /fapi/v1/openInterest.
type oiRaw struct {
	Symbol       string `json:"symbol"`
	OpenInterest string `json:"openInterest"`
	Time         int64  `json:"time"`
}

// oiHistEntry is a single entry from GET /futures/data/openInterestHist.
type oiHistEntry struct {
	Symbol               string `json:"symbol"`
	SumOpenInterest      string `json:"sumOpenInterest"`
	SumOpenInterestValue string `json:"sumOpenInterestValue"`
	Timestamp            int64  `json:"timestamp"`
}

// lsrEntry is a single entry from GET /futures/data/topLongShortPositionRatio.
type lsrEntry struct {
	Symbol         string `json:"symbol"`
	LongShortRatio string `json:"longShortRatio"`
	Timestamp      int64  `json:"timestamp"`
}

// KlineInterval is a helper type for kline interval configuration.
type KlineInterval struct {
	Interval string
	Limit    int
}

// DefaultKlineIntervals defines the kline timeframes to fetch per symbol.
var DefaultKlineIntervals = []KlineInterval{
	{Interval: "1m", Limit: 60},
	{Interval: "5m", Limit: 50},
	{Interval: "15m", Limit: 120},
	{Interval: "1h", Limit: 100},
	{Interval: "4h", Limit: 100},
	{Interval: "1d", Limit: 50},
}

// FastKlineIntervals are refreshed on every REST cycle. Higher-timeframe
// structure is carried between fast cycles because it changes slowly and is not
// an execution-timing input.
var FastKlineIntervals = []KlineInterval{
	{Interval: "1m", Limit: 60},
	{Interval: "5m", Limit: 50},
	{Interval: "15m", Limit: 120},
	{Interval: "1h", Limit: 100},
}

// StructuralKlineIntervals are refreshed less frequently to keep 4h structure
// current without adding a full extra kline request per detailed symbol on
// every 30-second REST cycle.
var StructuralKlineIntervals = []KlineInterval{
	{Interval: "4h", Limit: 100},
}
