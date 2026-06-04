package local

// ============================================================================
// Hunter v7 — Multi-Regime Alpha Signal Router: Core Types
// ============================================================================
// This file defines all types used by the v7 signal routing engine.
// The v7 engine replaces the single-score v6 with modular pattern detection.

// V7Direction represents the trading direction.
type V7Direction string

const (
	V7DirLong  V7Direction = "LONG"
	V7DirShort V7Direction = "SHORT"
)

// V7MarketRegime identifies the current global market state.
type V7MarketRegime string

const (
	V7RegimeTrendUp     V7MarketRegime = "trend_up"
	V7RegimeTrendDown   V7MarketRegime = "trend_down"
	V7RegimeRange       V7MarketRegime = "range"
	V7RegimePanicDump   V7MarketRegime = "panic_dump"
	V7RegimePullback    V7MarketRegime = "market_pullback"
	V7RegimeManiaPump   V7MarketRegime = "mania_pump"
	V7RegimeCompression V7MarketRegime = "compression"
	V7RegimeRotation    V7MarketRegime = "rotation"
	V7RegimeMixed       V7MarketRegime = "mixed"
)

// V7SetupType identifies the trading setup pattern.
type V7SetupType string

const (
	V7SetupPullbackLong       V7SetupType = "pullback_reversal_long"
	V7SetupShortSqueezeLong   V7SetupType = "short_squeeze_long"
	V7SetupTrendBreakoutLong  V7SetupType = "trend_breakout_long"
	V7SetupLeaderMomentumLong V7SetupType = "leader_momentum_long"
	V7SetupPanicReversalLong  V7SetupType = "panic_reversal_long"
	V7SetupAccumulationLong   V7SetupType = "accumulation_breakout_long"
	V7SetupDistributionShort  V7SetupType = "distribution_short"
	V7SetupLongSqueezeShort   V7SetupType = "long_squeeze_short"
	V7SetupRangeReversion     V7SetupType = "range_reversion"
	V7SetupFundingReversal    V7SetupType = "funding_reversal"
)

// V7SignalStatus represents the signal's lifecycle state.
type V7SignalStatus string

const (
	V7StatusCandidate     V7SignalStatus = "candidate"
	V7StatusWaitConfirm   V7SignalStatus = "wait_confirm"
	V7StatusConflictWatch V7SignalStatus = "conflict_watch"
	V7StatusFiltered      V7SignalStatus = "filtered"
)

// V7RiskLevel represents the overall risk assessment.
type V7RiskLevel string

const (
	V7RiskLow     V7RiskLevel = "LOW"
	V7RiskMedium  V7RiskLevel = "MEDIUM"
	V7RiskHigh    V7RiskLevel = "HIGH"
	V7RiskExtreme V7RiskLevel = "EXTREME"
)

// V7EntryMode describes how the AI should enter the trade.
type V7EntryMode string

const (
	V7EntryImmediate         V7EntryMode = "immediate"
	V7EntryWaitConfirm       V7EntryMode = "wait_confirm"
	V7EntryBreakout          V7EntryMode = "breakout_or_pullback"
	V7EntryFastConfirm       V7EntryMode = "fast_confirm"
	V7EntryWaitReclaim       V7EntryMode = "wait_reclaim"
	V7EntryWaitBreakout      V7EntryMode = "wait_breakout"
	V7EntryWaitReject        V7EntryMode = "wait_reject"
	V7EntryRangeEdge         V7EntryMode = "range_edge_only"
	V7EntryWaitPriceReversal V7EntryMode = "wait_price_reversal"
	V7EntryMomentumTrailing  V7EntryMode = "momentum_with_trailing_stop"
)

// V7PoolType classifies which candidate pool a symbol belongs to.
type V7PoolType string

const (
	V7PoolCoreLiquidity V7PoolType = "core_liquidity"
	V7PoolHotAlt        V7PoolType = "hot_alt"
	V7PoolPanic         V7PoolType = "panic"
	V7PoolSqueeze       V7PoolType = "squeeze"
	V7PoolFunding       V7PoolType = "funding"
	V7PoolNewListing    V7PoolType = "new_listing"
)

// V7SymbolContext holds all derived data for a single symbol, pre-computed once
// by UniverseBuilder and reused across all signal modules.
type V7SymbolContext struct {
	Symbol   string
	Snapshot *SymbolSnapshotData // Lightweight snapshot data

	// Derived price data
	CurrentPrice float64
	Change1h     float64
	Change4h     float64
	Change24h    float64

	// ATR by timeframe
	ATR1h  float64
	ATR4h  float64
	ATR1d  float64
	ATR15m float64
	ATR5m  float64

	// Bollinger Band data (15m primary)
	BBWidth15m  float64
	BBUpper15m  float64
	BBMiddle15m float64
	BBLower15m  float64
	BBWidth5m   float64

	// Trend indicators
	ADX1h float64
	RSI1h float64

	// EMA values for trend detection
	EMA20_4h float64
	EMA60_4h float64
	EMA20_1h float64
	EMA60_1h float64

	// Taker buy/sell ratio by timeframe
	TakerBuy15m float64 // Latest 15m Taker Buy Ratio (0-1)
	TakerBuy1h  float64 // Latest 1h Taker Buy Ratio (0-1)

	// Pool classification
	PoolType V7PoolType

	// 1h high/low for range detection
	High1h float64
	Low1h  float64

	// 4h high/low for S/R
	High4h float64
	Low4h  float64

	// 1d high/low
	High1d float64
	Low1d  float64

	// BB width percentile (current / rolling min)
	BBWidthPercentile float64

	// VWAP approximation (15m)
	VWAP15m float64
}

// SymbolSnapshotData is a lightweight copy of the snapshot data needed by modules.
// We don't hold a pointer to datafetch.SymbolSnapshot to avoid import cycles;
// instead we extract what we need into flat fields.
type SymbolSnapshotData struct {
	// Price & Volume
	Price          float64
	PriceChange24h float64
	Volume24h      float64
	QuoteVolume24h float64
	HighPrice24h   float64
	LowPrice24h    float64
	TradeCount24h  int64

	// Derivatives
	FundingRate float64
	OI          float64
	OIDelta1h   float64
	OIDelta4h   float64
	LSR         float64 // Latest LSR
	LSRPrev     float64 // Previous LSR
	LSROldest   float64 // Oldest LSR (for reversal detection)
	TakerBuy    float64 // Latest 1h Taker Buy Ratio
}

// V7PriceZone defines an entry price range.
type V7PriceZone struct {
	Lower float64 `json:"lower"`
	Upper float64 `json:"upper"`
}

// V7InvalidationRule defines the stop-loss / invalidation condition.
type V7InvalidationRule struct {
	Price  float64 `json:"price"`
	Reason string  `json:"reason"`
}

// V7Target defines a take-profit target.
type V7Target struct {
	Price  float64 `json:"price"`
	Reason string  `json:"reason"`
}

// V7PriceContext is a snapshot of price data for the AI decision layer.
type V7PriceContext struct {
	Last      float64 `json:"last"`
	Change1h  float64 `json:"change_1h"`
	Change4h  float64 `json:"change_4h"`
	Change24h float64 `json:"change_24h"`
	ATR1h     float64 `json:"atr_1h"`
	ATR4h     float64 `json:"atr_4h"`
}

// V7DerivativesContext is a snapshot of derivatives data for the AI decision layer.
type V7DerivativesContext struct {
	OIValue     float64 `json:"oi_value"`
	OIChange1h  float64 `json:"oi_change_1h"`
	OIChange4h  float64 `json:"oi_change_4h"`
	FundingRate float64 `json:"funding_rate"`
	LSROldest   float64 `json:"lsr_oldest"`
	LSRNewest   float64 `json:"lsr_newest"`
	TakerBuy15m float64 `json:"taker_buy_ratio_15m"`
}

// V7SignalOutput is the structured output from a single signal module.
// This is the primary output of the Hunter v7 engine — it contains everything
// the AI trading engine needs to make a decision.
type V7SignalOutput struct {
	Symbol    string         `json:"symbol"`
	Direction V7Direction    `json:"direction"`
	SetupType V7SetupType    `json:"setup_type"`
	Status    V7SignalStatus `json:"status"`

	// Multi-dimensional scores (each 0-100)
	SetupScore     float64 `json:"setup_score"`
	RiskScore      float64 `json:"risk_score"`
	LiquidityScore float64 `json:"liquidity_score"`
	TimingScore    float64 `json:"timing_score"`
	RegimeFitScore float64 `json:"regime_fit_score"`
	AIPriority     float64 `json:"ai_priority"`

	// Structured trade context
	ReasonCodes      []string           `json:"reason_codes"`
	RiskTags         []string           `json:"risk_tags"`
	EntryMode        V7EntryMode        `json:"entry_mode"`
	EntryZone        V7PriceZone        `json:"entry_zone"`
	Invalidation     V7InvalidationRule `json:"invalidation"`
	Targets          []V7Target         `json:"targets"`
	RequiredConfirms []string           `json:"required_confirmations"`
	Confidence       string             `json:"confidence"`
	RiskLevel        V7RiskLevel        `json:"risk_level"`
	MarketRegime     V7MarketRegime     `json:"market_regime"`

	// Raw data snapshot for AI prompt enrichment
	PriceCtx       *V7PriceContext       `json:"price_context,omitempty"`
	DerivativesCtx *V7DerivativesContext `json:"derivatives_context,omitempty"`
}

// V7SignalModule is the interface that every trading setup module must implement.
type V7SignalModule interface {
	// Name returns the human-readable module name.
	Name() string
	// SetupType returns the setup type this module detects.
	SetupType() V7SetupType
	// Direction returns the fixed trading direction this module targets.
	Direction() V7Direction
	// Match performs a fast pre-filter (<1ms) to check symbol eligibility.
	Match(ctx *V7SymbolContext, regime V7MarketRegime) bool
	// Score performs full scoring; returns nil if the symbol doesn't qualify.
	Score(ctx *V7SymbolContext, regime V7MarketRegime) *V7SignalOutput
}

// V7Config holds configuration for the Hunter v7 engine.
type V7Config struct {
	MaxOutput     int     `json:"max_output"`
	MinAIPriority float64 `json:"min_ai_priority"`
	Aggressive    bool    `json:"aggressive"`
}

// DefaultV7Config returns sensible defaults.
func DefaultV7Config() V7Config {
	return V7Config{
		MaxOutput:     30,
		MinAIPriority: 55,
	}
}
