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
	V7SetupPreBreakoutWatch   V7SetupType = "pre_breakout_watch"
	V7SetupPreSqueezeWatch    V7SetupType = "pre_squeeze_watch"
	V7SetupPreDistribution    V7SetupType = "pre_distribution_watch"
	V7SetupAccumulationWatch  V7SetupType = "accumulation_watch"
	V7SetupDisplacementLong   V7SetupType = "displacement_momentum_long"
	V7SetupRangeExpansion     V7SetupType = "range_expansion_event"

	// v8 new modules (Phase 2 P1-D)
	V7SetupIntradayScalp     V7SetupType = "intraday_scalp_long"
	V7SetupVolatilitySqueeze V7SetupType = "volatility_squeeze_breakout"
	V7SetupWhaleFlow         V7SetupType = "whale_flow_reversal"

	V7SetupModuleNoMatch V7SetupType = "module_no_match"
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

// V7ExecutionQuality summarizes whether a signal is executable now or should
// stay in the AI watchlist for confirmation.
type V7ExecutionQuality string

const (
	V7ExecReady       V7ExecutionQuality = "ready"
	V7ExecNearConfirm V7ExecutionQuality = "near_confirm"
	V7ExecWatchOnly   V7ExecutionQuality = "watch_only"
	V7ExecChaseRisk   V7ExecutionQuality = "chase_risk"
	V7ExecInvalidRR   V7ExecutionQuality = "invalid_rr"
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
	V7PoolVelocity      V7PoolType = "velocity"
	V7PoolNewActivity   V7PoolType = "new_activity"
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

	// Amplitude & range expansion (added for mover recall improvement)
	Amplitude24h     float64 // (High24h - Low24h) / Low24h * 100
	RangeExpansion1h float64 // 1h trueRange / median 20h trueRange
	Velocity5m       float64 // latest 5m close-to-close change %
	Velocity15m      float64 // latest 15m close-to-close change %
	VolumeBurst5m    float64 // latest 5m volume / recent average
	VolumeBurst15m   float64 // latest 15m volume / recent average
	ExecutionContext *V7ExecutionContext

	// 5m micro-structure data (v8 timing booster)
	RSI5m      float64 // 5m RSI for timing booster
	OI5m       float64 // 5m OI change
	TakerBuy5m float64 // 5m taker buy ratio
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
	OI          float64 // USDT notional: Binance openInterest quantity * current price
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
	VWAP15m   float64 `json:"vwap_15m,omitempty"`
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

type V7ExecutionTimeframeSummary struct {
	Timeframe        string  `json:"timeframe"`
	CandleCount      int     `json:"candle_count"`
	LastClose        float64 `json:"last_close"`
	RecentHigh3      float64 `json:"recent_high3,omitempty"`
	RecentLow3       float64 `json:"recent_low3,omitempty"`
	HasEMA20         bool    `json:"has_ema20"`
	CloseVsEMA20Pct  float64 `json:"close_vs_ema20_pct,omitempty"`
	HasATR           bool    `json:"has_atr"`
	ATRPct           float64 `json:"atr_pct,omitempty"`
	MinStop08ATRPct  float64 `json:"min_stop_0_8atr_pct,omitempty"`
	HasVWAP20        bool    `json:"has_vwap20"`
	VWAP20           float64 `json:"vwap20,omitempty"`
	CloseVsVWAP20Pct float64 `json:"close_vs_vwap20_pct,omitempty"`
	NoNewHigh        bool    `json:"no_new_high,omitempty"`
	NoNewLow         bool    `json:"no_new_low,omitempty"`
	VolumeVsAvg5     float64 `json:"volume_vs_avg5,omitempty"`
}

type V7ExecutionContext struct {
	DataQuality string                                 `json:"data_quality"`
	Timeframes  map[string]V7ExecutionTimeframeSummary `json:"timeframes,omitempty"`
}

type V7TakeProfitPlan struct {
	TP0Price               float64  `json:"tp0_price,omitempty"`
	TP0DistancePct         float64  `json:"tp0_distance_pct,omitempty"`
	TP0ReducePctMin        float64  `json:"tp0_reduce_pct_min,omitempty"`
	TP0ReducePctMax        float64  `json:"tp0_reduce_pct_max,omitempty"`
	MoveStopToBreakeven    bool     `json:"move_stop_to_breakeven"`
	TrailingStopMode       string   `json:"trailing_stop_mode,omitempty"`
	TrailingBasis          []string `json:"trailing_basis,omitempty"`
	TrailingDistancePctMin float64  `json:"trailing_distance_pct_min,omitempty"`
	TrailingDistancePctMax float64  `json:"trailing_distance_pct_max,omitempty"`
	StatsBucket            string   `json:"stats_bucket,omitempty"`
}

type V7ReadinessTier string

const (
	V7ReadinessExecutable V7ReadinessTier = "EXECUTABLE"
	V7ReadinessReviewable V7ReadinessTier = "REVIEWABLE"
	V7ReadinessWatch      V7ReadinessTier = "WATCH"
	V7ReadinessRejected   V7ReadinessTier = "REJECTED"
)

type V7ExecutionReadiness struct {
	Tier             V7ReadinessTier `json:"tier"`
	Reason           string          `json:"reason"`
	ReadyScore       float64         `json:"ready_score"`
	WindowHealth     float64         `json:"window_health"`
	EntryZonePos     float64         `json:"entry_zone_position"`
	PriceDeviation   float64         `json:"price_deviation_pct"`
	DataQuality      string          `json:"data_quality"`
	MissingHard      []string        `json:"missing_hard,omitempty"`
	MissingExecution []string        `json:"missing_execution,omitempty"`
	MissingContext   []string        `json:"missing_context,omitempty"`
	BlockedGate      string          `json:"blocked_gate,omitempty"`
	NextConfirm      []string        `json:"next_confirmations,omitempty"`
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
	ReasonCodes      []string               `json:"reason_codes"`
	RiskTags         []string               `json:"risk_tags"`
	EntryMode        V7EntryMode            `json:"entry_mode"`
	ExecutionQuality V7ExecutionQuality     `json:"execution_quality,omitempty"`
	EntryZone        V7PriceZone            `json:"entry_zone"`
	Invalidation     V7InvalidationRule     `json:"invalidation"`
	Targets          []V7Target             `json:"targets"`
	RequiredConfirms []string               `json:"required_confirmations"`
	ConfirmSummary   *V7ConfirmationSummary `json:"confirmation_summary,omitempty"`
	Confidence       string                 `json:"confidence"`
	RiskLevel        V7RiskLevel            `json:"risk_level"`
	MarketRegime     V7MarketRegime         `json:"market_regime"`

	// Raw data snapshot for AI prompt enrichment
	PriceCtx       *V7PriceContext       `json:"price_context,omitempty"`
	DerivativesCtx *V7DerivativesContext `json:"derivatives_context,omitempty"`

	ExecutionReadiness *V7ExecutionReadiness `json:"execution_readiness,omitempty"`
	ExecutionContext   *V7ExecutionContext   `json:"execution_context,omitempty"`

	// Liquidity context for adaptive OI threshold in prompt-data filter
	QuoteVolume24h float64 `json:"quote_volume_24h,omitempty"` // 24h quote volume (USD)

	// Multi-timeframe TP targets (new in v8)
	TP0Price      float64           `json:"tp0_price,omitempty"`
	TP0RR         float64           `json:"tp0_rr,omitempty"`
	TP0TimeWindow string            `json:"tp0_time_window,omitempty"`
	TP0Method     string            `json:"tp0_method,omitempty"`
	TP1Price      float64           `json:"tp1_price,omitempty"`
	TP1RR         float64           `json:"tp1_rr,omitempty"`
	TP1TimeWindow string            `json:"tp1_time_window,omitempty"`
	TP1Method     string            `json:"tp1_method,omitempty"`
	TP2Price      float64           `json:"tp2_price,omitempty"`
	TP2RR         float64           `json:"tp2_rr,omitempty"`
	TP2TimeWindow string            `json:"tp2_time_window,omitempty"`
	TP2Method     string            `json:"tp2_method,omitempty"`
	TPPlan        *V7TakeProfitPlan `json:"take_profit_plan,omitempty"`

	// Resonance scoring (Phase 2 prep)
	ResonanceBonus float64 `json:"resonance_bonus,omitempty"`
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

// V7SignalRecord is the raw signal data passed to the recorder callback.
type V7SignalRecord struct {
	Signal      V7SignalOutput
	Tier        string // EXECUTABLE / REVIEWABLE / WATCH / REJECTED
	TierReason  string
	BlockedGate string // Where the signal was blocked in the funnel
}

type V7PotentialComponents struct {
	Amplitude        float64 `json:"amplitude"`
	Velocity         float64 `json:"velocity"`
	VolumeBurst      float64 `json:"volume_burst"`
	OIDelta          float64 `json:"oi_delta"`
	FundingCrowding  float64 `json:"funding_crowding"`
	RelativeStrength float64 `json:"relative_strength"`
}

type V7PotentialCandidate struct {
	Symbol                    string                `json:"symbol"`
	Direction                 V7Direction           `json:"direction"`
	OpportunityPotentialScore float64               `json:"opportunity_potential_score"`
	Components                V7PotentialComponents `json:"components"`
	Amplitude24h              float64               `json:"amplitude_24h"`
	Velocity5m                float64               `json:"velocity_5m"`
	Velocity15m               float64               `json:"velocity_15m"`
	VolumeBurst5m             float64               `json:"volume_burst_5m"`
	VolumeBurst15m            float64               `json:"volume_burst_15m"`
	OIDelta1h                 float64               `json:"oi_delta_1h"`
	OIDelta4h                 float64               `json:"oi_delta_4h"`
	FundingRate               float64               `json:"funding_rate"`
	RelativeStrength4h        float64               `json:"relative_strength_4h"`
	MatchedModule             bool                  `json:"matched_module"`
	MatchedSetups             []V7SetupType         `json:"matched_setups,omitempty"`
	TrackingWindows           []string              `json:"tracking_windows"`
	AuditUse                  string                `json:"audit_use"`
}

// V7SignalRecorder is a callback invoked after ScoreHunterV7 completes,
// enabling the caller to persist all raw signals for funnel attribution.
type V7SignalRecorder func(cycleNumber int, records []V7SignalRecord, regime V7MarketRegime)

// V7RouteResult carries both the LLM-facing output and the raw router output.
type V7RouteResult struct {
	RawSignals       []V7SignalOutput
	ConfirmedSignals []V7SignalOutput
	WatchSignals     []V7SignalOutput
	OutputSignals    []V7SignalOutput
	PotentialPool    []V7PotentialCandidate
}

// V7ScoreResult is the detailed Hunter v7 scoring result used for attribution.
type V7ScoreResult struct {
	Regime        V7MarketRegime
	Universe      []V7SymbolContext
	RawSignals    []V7SignalOutput
	Signals       []V7SignalOutput
	PotentialPool []V7PotentialCandidate
	Attribution   V7AttributionSummary
}

// V7AttributionSummary captures per-cycle funnel counts for diagnostics.
type V7AttributionSummary struct {
	UniverseTotal int
	PoolCounts    map[string]int
	SetupCounts   map[string]int
	StatusCounts  map[string]int
	QualityCounts map[string]int
	OutputCounts  map[string]int
}

// V7Config holds configuration for the Hunter v7 engine.
type V7Config struct {
	MaxOutput              int                          `json:"max_output"`
	MinOutput              int                          `json:"min_output"`
	WatchOutput            int                          `json:"watch_output"`
	MinAIPriority          float64                      `json:"min_ai_priority"`
	FallbackMinAIPriority  float64                      `json:"fallback_min_ai_priority"`
	CorrelationMaxPerTheme int                          `json:"correlation_max_per_theme"`
	Aggressive             bool                         `json:"aggressive"`
	SetupThresholds        map[string]V7SetupThresholds `json:"setup_thresholds,omitempty"`
	SignalRecorder         V7SignalRecorder             `json:"-"` // Optional callback for signal persistence
	CycleNumber            int                          `json:"-"` // Current cycle number for recorder
	WatchStateManager      *V7SignalStateManager        `json:"-"` // Optional cross-cycle watch state tracker
}

// DefaultV7Config returns sensible defaults.
func DefaultV7Config() V7Config {
	return V7Config{
		MaxOutput:              30,
		MinOutput:              3,
		WatchOutput:            5,
		MinAIPriority:          55,
		FallbackMinAIPriority:  45,
		CorrelationMaxPerTheme: 3,
		SetupThresholds:        DefaultSetupThresholds(),
	}
}

// V7SetupThresholds defines per-setup filtering and execution guard parameters.
// Setup-specific values are execution guidance/guards. The router uses
// V7Config.MinAIPriority for candidate visibility so guard defaults do not hide
// otherwise useful LLM context.
type V7SetupThresholds struct {
	MinAIPriority   float64 `json:"min_ai_priority"`
	MinZonePosShort int     `json:"min_zone_pos_short"` // SHORT requires zone_pos >= this (0 = disabled)
	MaxZonePosLong  int     `json:"max_zone_pos_long"`  // LONG requires zone_pos <= this (100 = disabled)
	RequireOIFlush  bool    `json:"require_oi_flush"`   // C-grade must have OI flush to proceed
	MinConfidence   string  `json:"min_confidence"`     // Minimum confidence grade (e.g. "C", "B")
}

// DefaultSetupThresholds returns per-setup default thresholds for all 10 setup types.
func DefaultSetupThresholds() map[string]V7SetupThresholds {
	return map[string]V7SetupThresholds{
		string(V7SetupFundingReversal): {
			MinAIPriority:   55,
			MinZonePosShort: 65,
			MaxZonePosLong:  35,
			RequireOIFlush:  true,
			MinConfidence:   "C",
		},
		string(V7SetupPanicReversalLong): {
			MinAIPriority:   50,
			MinZonePosShort: 0,
			MaxZonePosLong:  100,
			RequireOIFlush:  false,
			MinConfidence:   "C",
		},
		string(V7SetupTrendBreakoutLong): {
			MinAIPriority:   60,
			MinZonePosShort: 0,
			MaxZonePosLong:  100,
			RequireOIFlush:  false,
			MinConfidence:   "C",
		},
		string(V7SetupLeaderMomentumLong): {
			MinAIPriority:   58,
			MinZonePosShort: 0,
			MaxZonePosLong:  100,
			RequireOIFlush:  false,
			MinConfidence:   "C",
		},
		string(V7SetupPullbackLong): {
			MinAIPriority:   55,
			MinZonePosShort: 0,
			MaxZonePosLong:  50,
			RequireOIFlush:  false,
			MinConfidence:   "C",
		},
		string(V7SetupShortSqueezeLong): {
			MinAIPriority:   55,
			MinZonePosShort: 0,
			MaxZonePosLong:  100,
			RequireOIFlush:  false,
			MinConfidence:   "C",
		},
		string(V7SetupAccumulationLong): {
			MinAIPriority:   55,
			MinZonePosShort: 0,
			MaxZonePosLong:  100,
			RequireOIFlush:  false,
			MinConfidence:   "C",
		},
		string(V7SetupDistributionShort): {
			MinAIPriority:   55,
			MinZonePosShort: 60,
			MaxZonePosLong:  0,
			RequireOIFlush:  false,
			MinConfidence:   "C",
		},
		string(V7SetupLongSqueezeShort): {
			MinAIPriority:   55,
			MinZonePosShort: 60,
			MaxZonePosLong:  0,
			RequireOIFlush:  false,
			MinConfidence:   "C",
		},
		string(V7SetupRangeReversion): {
			MinAIPriority:   55,
			MinZonePosShort: 55,
			MaxZonePosLong:  45,
			RequireOIFlush:  false,
			MinConfidence:   "C",
		},
		string(V7SetupPreBreakoutWatch): {
			MinAIPriority:   40,
			MinZonePosShort: 0,
			MaxZonePosLong:  100,
			RequireOIFlush:  false,
			MinConfidence:   "C",
		},
		string(V7SetupPreSqueezeWatch): {
			MinAIPriority:   40,
			MinZonePosShort: 0,
			MaxZonePosLong:  100,
			RequireOIFlush:  false,
			MinConfidence:   "C",
		},
		string(V7SetupPreDistribution): {
			MinAIPriority:   40,
			MinZonePosShort: 55,
			MaxZonePosLong:  0,
			RequireOIFlush:  false,
			MinConfidence:   "C",
		},
		string(V7SetupAccumulationWatch): {
			MinAIPriority:   40,
			MinZonePosShort: 0,
			MaxZonePosLong:  100,
			RequireOIFlush:  false,
			MinConfidence:   "C",
		},
		string(V7SetupDisplacementLong): {
			MinAIPriority:   50,
			MinZonePosShort: 0,
			MaxZonePosLong:  100,
			RequireOIFlush:  false,
			MinConfidence:   "C",
		},
		string(V7SetupRangeExpansion): {
			MinAIPriority:   50,
			MinZonePosShort: 0,
			MaxZonePosLong:  100,
			RequireOIFlush:  false,
			MinConfidence:   "C",
		},
		// v8 new modules
		string(V7SetupIntradayScalp): {
			MinAIPriority:   50,
			MinZonePosShort: 0,
			MaxZonePosLong:  100,
			RequireOIFlush:  false,
			MinConfidence:   "C",
		},
		string(V7SetupVolatilitySqueeze): {
			MinAIPriority:   50,
			MinZonePosShort: 0,
			MaxZonePosLong:  100,
			RequireOIFlush:  false,
			MinConfidence:   "C",
		},
		string(V7SetupWhaleFlow): {
			MinAIPriority:   50,
			MinZonePosShort: 0,
			MaxZonePosLong:  100,
			RequireOIFlush:  false,
			MinConfidence:   "C",
		},
	}
}

// GetSetupThresholds returns the thresholds for a given setup type.
// Falls back to a default using the global MinAIPriority if no per-setup config exists.
func (c *V7Config) GetSetupThresholds(setupType V7SetupType) V7SetupThresholds {
	if c.SetupThresholds != nil {
		if th, ok := c.SetupThresholds[string(setupType)]; ok {
			return th
		}
	}
	// Fallback: use global MinAIPriority with relaxed guard values
	return V7SetupThresholds{
		MinAIPriority:   c.MinAIPriority,
		MinZonePosShort: 0,
		MaxZonePosLong:  100,
		RequireOIFlush:  false,
		MinConfidence:   "C",
	}
}
