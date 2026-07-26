package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/logger"
	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/aitos"
	"github.com/Aixxww/AiT/provider/hyperliquid"
	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/provider/square"
	"github.com/Aixxww/AiT/security"
	"github.com/Aixxww/AiT/store"
)

var snapshotReadyTimeout = 3 * time.Minute

// ============================================================================
// Type Definitions
// ============================================================================

// PositionInfo position information
type PositionInfo struct {
	Symbol           string  `json:"symbol"`
	Side             string  `json:"side"` // "long" or "short"
	EntryPrice       float64 `json:"entry_price"`
	MarkPrice        float64 `json:"mark_price"`
	Quantity         float64 `json:"quantity"`
	Leverage         int     `json:"leverage"`
	UnrealizedPnL    float64 `json:"unrealized_pnl"`
	UnrealizedPnLPct float64 `json:"unrealized_pnl_pct"`
	PeakPnLPct       float64 `json:"peak_pnl_pct"` // Historical peak profit percentage
	LiquidationPrice float64 `json:"liquidation_price"`
	MarginUsed       float64 `json:"margin_used"`
	StopLoss         float64 `json:"stop_loss,omitempty"`
	TakeProfit       float64 `json:"take_profit,omitempty"`
	UpdateTime       int64   `json:"update_time"` // Position update timestamp (milliseconds)
}

// AccountInfo account information
type AccountInfo struct {
	TotalEquity      float64 `json:"total_equity"`      // Account equity
	AvailableBalance float64 `json:"available_balance"` // Available balance
	UnrealizedPnL    float64 `json:"unrealized_pnl"`    // Unrealized profit/loss
	TotalPnL         float64 `json:"total_pnl"`         // Total profit/loss
	TotalPnLPct      float64 `json:"total_pnl_pct"`     // Total profit/loss percentage
	MarginUsed       float64 `json:"margin_used"`       // Used margin
	MarginUsedPct    float64 `json:"margin_used_pct"`   // Margin usage rate
	PositionCount    int     `json:"position_count"`    // Number of positions
}

// CandidateCoin candidate coin (from coin pool)
type CandidateCoin struct {
	Symbol         string                 `json:"symbol"`
	Sources        []string               `json:"sources"` // Sources: "ai500" and/or "oi_top"
	PreFetchedData *market.PreFetchedData `json:"-"`       // Hunter pre-fetched klines, not serialized
	Direction      string                 `json:"-"`       // Hunter direction: "LONG" or "SHORT"
	SignalTags     []string               `json:"-"`       // Hunter signal tags for AI context
	LongScore      float64                `json:"-"`       // Hunter LONG composite score
	ShortScore     float64                `json:"-"`       // Hunter SHORT composite score
	LongTags       []string               `json:"-"`       // Hunter LONG signal tags
	ShortTags      []string               `json:"-"`       // Hunter SHORT signal tags

	// Sniff mode fields (only set when source_type == "hunter_sniff")
	AmbushType    string   `json:"-"` // "LONG_AMBUSH" or "SHORT_DISTRIBUTION"
	AmbushReasons []string `json:"-"` // Sniff filter pass reasons

	// Capital confirmation tier (three-level gate)
	CapitalLevel int    `json:"-"` // 0=none, 1=weak, 2=moderate, 3=strong
	CapitalTier  string `json:"-"` // "Tier-S PRIME SIGNAL" / "Tier-A" / "Tier-B LOW CONFIDENCE" / "Untiered"

	// Hunter v7 structured signal context (set when source_type == "hunter_v7")
	V7SignalID         string                       `json:"-"`
	V7SetupType        string                       `json:"-"`
	V7Status           string                       `json:"-"`
	V7AIPriority       float64                      `json:"-"`
	V7SetupScore       float64                      `json:"-"`
	V7RiskScore        float64                      `json:"-"`
	V7LiquidityScore   float64                      `json:"-"`
	V7TimingScore      float64                      `json:"-"`
	V7RegimeFitScore   float64                      `json:"-"`
	V7RiskLevel        string                       `json:"-"`
	V7EntryMode        string                       `json:"-"`
	V7MarketShape      string                       `json:"-"`
	V7EntrySignal      string                       `json:"-"`
	V7ExecutionQuality string                       `json:"-"`
	V7MarketRegime     string                       `json:"-"`
	V7Confidence       string                       `json:"-"`
	V7ReasonCodes      []string                     `json:"-"`
	V7RiskTags         []string                     `json:"-"`
	V7RequiredConfirms []string                     `json:"-"`
	V7EntryZone        local.V7PriceZone            `json:"-"`
	V7Invalidation     local.V7InvalidationRule     `json:"-"`
	V7Targets          []local.V7Target             `json:"-"`
	V7ConfirmSummary   *local.V7ConfirmationSummary `json:"-"`
	V7PriceContext     *local.V7PriceContext        `json:"-"`
	V7DerivativesCtx   *local.V7DerivativesContext  `json:"-"`
	V7DataFreshness    local.V7DataFreshness        `json:"-"`
	V7Readiness        *local.V7ExecutionReadiness  `json:"-"`
	V7ExecutionContext *local.V7ExecutionContext    `json:"-"`
	V7TP0Price         float64                      `json:"-"`
	V7TP0RR            float64                      `json:"-"`
	V7TP0TimeWindow    string                       `json:"-"`
	V7TP0Method        string                       `json:"-"`
	V7TP1Price         float64                      `json:"-"`
	V7TP1RR            float64                      `json:"-"`
	V7TP1TimeWindow    string                       `json:"-"`
	V7TP1Method        string                       `json:"-"`
	V7TP2Price         float64                      `json:"-"`
	V7TP2RR            float64                      `json:"-"`
	V7TP2TimeWindow    string                       `json:"-"`
	V7TP2Method        string                       `json:"-"`
	V7TPPlan           *local.V7TakeProfitPlan      `json:"-"`
	V7VWAP15m          float64                      `json:"-"`
	V7ExecutionTier    string                       `json:"-"`
	V7TierReason       string                       `json:"-"`
	V7QuoteVolume24h   float64                      `json:"-"` // 24h quote volume for adaptive OI threshold

	// IndicatorHub unified engine signal (set when using SnapshotEngine)
	TradeSignal interface{} `json:"-"` // *engine.TradeSignal when using SnapshotEngine
}

// OITopData open interest growth top data (for AI decision reference)
type OITopData struct {
	Rank              int     // OI Top ranking
	OIDeltaPercent    float64 // Open interest change percentage (1 hour)
	OIDeltaValue      float64 // Open interest change value
	PriceDeltaPercent float64 // Price change percentage
}

// TradingStats trading statistics (for AI input)
type TradingStats struct {
	TotalTrades    int     `json:"total_trades"`     // Total number of trades (closed)
	WinRate        float64 `json:"win_rate"`         // Win rate (%)
	ProfitFactor   float64 `json:"profit_factor"`    // Profit factor
	SharpeRatio    float64 `json:"sharpe_ratio"`     // Sharpe ratio
	TotalPnL       float64 `json:"total_pnl"`        // Total profit/loss
	AvgWin         float64 `json:"avg_win"`          // Average win
	AvgLoss        float64 `json:"avg_loss"`         // Average loss
	MaxDrawdownPct float64 `json:"max_drawdown_pct"` // Maximum drawdown (%)
}

// RecentOrder recently completed order (for AI input)
type RecentOrder struct {
	Symbol       string  `json:"symbol"`        // Trading pair
	Side         string  `json:"side"`          // long/short
	EntryPrice   float64 `json:"entry_price"`   // Entry price
	ExitPrice    float64 `json:"exit_price"`    // Exit price
	RealizedPnL  float64 `json:"realized_pnl"`  // Realized profit/loss
	PnLPct       float64 `json:"pnl_pct"`       // Profit/loss percentage
	EntryTime    string  `json:"entry_time"`    // Entry time
	ExitTime     string  `json:"exit_time"`     // Exit time
	HoldDuration string  `json:"hold_duration"` // Hold duration, e.g. "2h30m"
}

// Context trading context (complete information passed to AI)
type Context struct {
	CurrentTime        string                             `json:"current_time"`
	RuntimeMinutes     int                                `json:"runtime_minutes"`
	CallCount          int                                `json:"call_count"`
	IsDegraded         bool                               `json:"is_degraded,omitempty"`
	DegradationReasons []string                           `json:"degradation_reasons,omitempty"`
	AccountDataStale   bool                               `json:"account_data_stale,omitempty"`
	PositionDataStale  bool                               `json:"position_data_stale,omitempty"`
	DisableOpenOrders  bool                               `json:"disable_open_orders,omitempty"`
	Account            AccountInfo                        `json:"account"`
	Positions          []PositionInfo                     `json:"positions"`
	CandidateCoins     []CandidateCoin                    `json:"candidate_coins"`
	PromptVariant      string                             `json:"prompt_variant,omitempty"`
	TradingStats       *TradingStats                      `json:"trading_stats,omitempty"`
	RecentOrders       []RecentOrder                      `json:"recent_orders,omitempty"`
	MarketDataMap      map[string]*market.Data            `json:"-"`
	MultiTFMarket      map[string]map[string]*market.Data `json:"-"`
	OITopDataMap       map[string]*OITopData              `json:"-"`
	QuantDataMap       map[string]*QuantData              `json:"-"`
	OIRankingData      *aitos.OIRankingData               `json:"-"` // Market-wide OI ranking data
	NetFlowRankingData *aitos.NetFlowRankingData          `json:"-"` // Market-wide fund flow ranking data
	PriceRankingData   *aitos.PriceRankingData            `json:"-"` // Market-wide price gainers/losers
	BTCETHLeverage     int                                `json:"-"`
	AltcoinLeverage    int                                `json:"-"`
	Timeframes         []string                           `json:"-"`
	MarketEnv          *market.MarketEnvironment          `json:"-"` // Market regime (ADX-based)
}

// Decision AI trading decision
type Decision struct {
	Symbol string `json:"symbol"`
	Action string `json:"action"` // Standard: "open_long", "open_short", "close_long", "close_short", "hold", "wait"
	// Grid actions: "place_buy_limit", "place_sell_limit", "cancel_order", "cancel_all_orders", "pause_grid", "resume_grid", "adjust_grid"

	// Opening position parameters
	Leverage        int     `json:"leverage,omitempty"`
	PositionSizeUSD float64 `json:"position_size_usd,omitempty"`
	StopLoss        float64 `json:"stop_loss,omitempty"`
	TakeProfit      float64 `json:"take_profit,omitempty"`

	// Grid trading parameters
	Price      float64 `json:"price,omitempty"`       // Limit order price (for grid)
	Quantity   float64 `json:"quantity,omitempty"`    // Order quantity (for grid)
	LevelIndex int     `json:"level_index,omitempty"` // Grid level index
	OrderID    string  `json:"order_id,omitempty"`    // Order ID (for cancel)

	// Common parameters
	Confidence               int              `json:"confidence,omitempty"` // Confidence level (0-100)
	RiskUSD                  float64          `json:"risk_usd,omitempty"`   // Maximum USD risk
	Reasoning                string           `json:"reasoning"`
	BlockedReasonCode        string           `json:"blocked_reason_code,omitempty"`          // Structured wait reason enum (hunter_v7)
	SelectedHunterV7SignalID string           `json:"selected_hunter_v7_signal_id,omitempty"` // Exact Hunter v7 signal selected for open.
	SelectedHunterV7Tier     string           `json:"selected_hunter_v7_tier,omitempty"`
	SelectedHunterV7Setup    string           `json:"selected_hunter_v7_setup,omitempty"`
	BlockedSignalSymbol      string           `json:"blocked_signal_symbol,omitempty"`
	EffectiveRRAfterCap      float64          `json:"effective_rr_after_cap,omitempty"`
	SignalAgeMs              int64            `json:"signal_age_ms,omitempty"`
	Trigger                  *DecisionTrigger `json:"trigger,omitempty"` // Structured wait trigger (hunter_v7)
}

type DecisionTrigger struct {
	TriggerPrice      float64 `json:"trigger_price,omitempty"`
	RequiredClose     string  `json:"required_close,omitempty"`
	ExpiresInBars     int     `json:"expires_in_bars,omitempty"`
	ActionIfTriggered string  `json:"action_if_triggered,omitempty"`
}

// FullDecision AI's complete decision (including chain of thought)
type FullDecision struct {
	SystemPrompt        string     `json:"system_prompt"`
	UserPrompt          string     `json:"user_prompt"`
	CoTTrace            string     `json:"cot_trace"`
	Decisions           []Decision `json:"decisions"`
	RawResponse         string     `json:"raw_response"`
	Timestamp           time.Time  `json:"timestamp"`
	AIRequestDurationMs int64      `json:"ai_request_duration_ms,omitempty"`
	PromptTokens        int        `json:"prompt_tokens,omitempty"`
	CompletionTokens    int        `json:"completion_tokens,omitempty"`
	TotalTokens         int        `json:"total_tokens,omitempty"`
}

// QuantData quantitative data structure (fund flow, position changes, price changes)
type QuantData struct {
	Symbol      string             `json:"symbol"`
	Price       float64            `json:"price"`
	Netflow     *NetflowData       `json:"netflow,omitempty"`
	OI          map[string]*OIData `json:"oi,omitempty"`
	PriceChange map[string]float64 `json:"price_change,omitempty"`
}

type NetflowData struct {
	Institution *FlowTypeData `json:"institution,omitempty"`
	Personal    *FlowTypeData `json:"personal,omitempty"`
}

type FlowTypeData struct {
	Future map[string]float64 `json:"future,omitempty"`
	Spot   map[string]float64 `json:"spot,omitempty"`
}

type OIData struct {
	CurrentOI float64                 `json:"current_oi"`
	Delta     map[string]*OIDeltaData `json:"delta,omitempty"`
}

type OIDeltaData struct {
	OIDelta        float64 `json:"oi_delta"`
	OIDeltaValue   float64 `json:"oi_delta_value"`
	OIDeltaPercent float64 `json:"oi_delta_percent"`
}

// ============================================================================
// StrategyEngine - Core Strategy Execution Engine
// ============================================================================

// StrategyEngine strategy execution engine
type StrategyEngine struct {
	config           *store.StrategyConfig
	aitosClient      aitos.DataProvider
	squareClient     *square.Client              // nil when square_heat not configured
	marketEnv        *market.MarketEnvironment   // cached market regime classification (ADX-based)
	snapshotEngine   *SnapshotEngine             // nil = use legacy coin sources
	v7SignalRecorder local.V7SignalRecorder      // optional callback for funnel attribution
	v7CycleCounter   int                         // incremented each scoring call for recorder
	v7WatchState     *local.V7SignalStateManager // cross-cycle watch upgrade tracker
}

// NewStrategyEngine creates strategy execution engine.
// Uses local Binance-backed data provider by default.
func NewStrategyEngine(config *store.StrategyConfig) *StrategyEngine {
	// Use local data provider (Binance public APIs, no external dependencies)
	client := local.NewClient("")
	logger.Info("📊 Using local Binance-backed data provider (AI500/OI/NetFlow/Price)")

	e := &StrategyEngine{
		config:      config,
		aitosClient: client,
	}

	// Initialize Binance Square heat client when square_heat source is configured
	if config.CoinSource.SourceType == "square_heat" || config.CoinSource.UseSquareHeat {
		cfgURL := config.CoinSource.SquareHeatURL
		if cfgURL == "" {
			cfgURL = square.DefaultBaseURL
		}
		// SSRF validation: block internal/metadata IPs
		if err := security.ValidateURL(cfgURL); err != nil {
			logger.Warnf("⚠ SquareHeatURL rejected by SSRF check (%s): %v — using default %s", cfgURL, err, square.DefaultBaseURL)
			cfgURL = square.DefaultBaseURL
		}
		sq := square.NewClient(cfgURL)
		if config.CoinSource.SquareMinScore > 0 {
			sq.MinScore = config.CoinSource.SquareMinScore
		}
		e.squareClient = sq
		logger.Infof("🔥 Using Binance Square heat source (url=%s, minScore=%.1f)", cfgURL, sq.MinScore)
	}

	return e
}

// GetRiskControlConfig gets risk control configuration
func (e *StrategyEngine) GetRiskControlConfig() store.RiskControlConfig {
	return e.config.RiskControl
}

// GetLanguage returns the language from config or falls back to auto-detection
func (e *StrategyEngine) GetLanguage() Language {
	switch e.config.Language {
	case "zh":
		return LangChinese
	case "en":
		return LangEnglish
	default:
		// Fall back to auto-detection from prompt content for backward compatibility
		return detectLanguage(e.config.PromptSections.RoleDefinition)
	}
}

// GetConfig gets complete strategy configuration
func (e *StrategyEngine) GetConfig() *store.StrategyConfig {
	return e.config
}

// SetSnapshotEngine sets the unified IndicatorHub engine.
func (e *StrategyEngine) SetSnapshotEngine(se *SnapshotEngine) {
	e.snapshotEngine = se
}

// GetSnapshotEngine returns the current SnapshotEngine (may be nil).
func (e *StrategyEngine) GetSnapshotEngine() *SnapshotEngine {
	return e.snapshotEngine
}

// SetV7SignalRecorder sets the callback for persisting raw V7 signals.
func (e *StrategyEngine) SetV7SignalRecorder(recorder local.V7SignalRecorder) {
	e.v7SignalRecorder = recorder
}

// buildV7SignalRecords merges raw V7 signals with kernel tier classification.
func (e *StrategyEngine) buildV7SignalRecords(signals []local.V7SignalOutput, candidates []CandidateCoin) []local.V7SignalRecord {
	// Build a lookup from symbol+setup to candidate tier info
	tierMap := make(map[string]CandidateCoin, len(candidates))
	for _, cc := range candidates {
		key := cc.Symbol + "|" + cc.V7SetupType
		tierMap[key] = cc
	}
	records := make([]local.V7SignalRecord, 0, len(signals))
	for _, sig := range signals {
		key := sig.Symbol + "|" + string(sig.SetupType)
		rec := local.V7SignalRecord{Signal: sig}
		if sig.SetupType == local.V7SetupModuleNoMatch {
			rec.Tier = "REJECTED"
			rec.TierReason = "module_no_match"
			rec.BlockedGate = "module_no_match"
		} else if cc, ok := tierMap[key]; ok {
			rec.Tier = cc.V7ExecutionTier
			rec.TierReason = cc.V7TierReason
			rec.BlockedGate = v7BlockedGate(cc)
			if cc.V7Readiness != nil && cc.V7Readiness.BlockedGate != "" {
				rec.BlockedGate = cc.V7Readiness.BlockedGate
			}
		} else {
			rec.Tier = ""
			if sig.Status == local.V7StatusFiltered {
				rec.Tier = "REJECTED"
				rec.TierReason = "router_filtered"
				rec.BlockedGate = "router_filtered"
			} else {
				rec.BlockedGate = "router_priority_filtered"
			}
		}
		records = append(records, rec)
	}
	return records
}

// v7BlockedGate determines where a signal was blocked in the funnel.
func v7BlockedGate(cc CandidateCoin) string {
	if cc.V7ExecutionQuality == "invalid_rr" {
		return "execution_invalid_rr"
	}
	if cc.V7ExecutionQuality == "chase_risk" {
		return "execution_chase_risk"
	}
	if cc.V7Status == "filtered" {
		return "router_filtered"
	}
	if cc.V7ExecutionTier == "REJECTED" {
		return "kernel_tier_rejected:" + cc.V7TierReason
	}
	if cc.V7ExecutionTier == "WATCH" {
		return "kernel_tier_watch:" + cc.V7TierReason
	}
	return ""
}

// GetCandidateCoinsWithSnapshot is the unified data source entry point.
// All scoring engines (Hunter/AI500/IndicatorHub) share a single datafetch.Snapshot
// fetched by DataCollector — zero duplicate API calls, <1s pure-CPU scoring.
func (e *StrategyEngine) GetCandidateCoinsWithSnapshot() ([]CandidateCoin, error) {
	if e.snapshotEngine == nil {
		return e.GetCandidateCoins() // no SnapshotEngine → legacy path
	}

	snap := e.snapshotEngine.GetSnapshot()
	if snap == nil || len(snap.Symbols) == 0 {
		logger.Info("📋 Snapshot 为空，等待 DataCollector 首轮快照")
		snap = e.snapshotEngine.WaitForSnapshot(snapshotReadyTimeout)
	}
	if snap == nil || len(snap.Symbols) == 0 {
		return nil, fmt.Errorf("snapshot not ready after waiting for initial DataCollector fetch")
	}
	maxAge := e.snapshotEngine.MaxSnapshotAge()
	if !snapshotIsFresh(snap, maxAge) {
		logger.Infof("📋 Snapshot 已过期(age=%s, max=%s)，等待 DataCollector 刷新", time.Since(snap.CreatedAt).Round(time.Second), maxAge)
		snap = e.snapshotEngine.WaitForFreshSnapshot(snapshotReadyTimeout, maxAge)
	}
	if !snapshotIsFresh(snap, maxAge) {
		return nil, fmt.Errorf("snapshot stale after waiting for DataCollector refresh")
	}

	return e.scoreFromSnapshot(snap)
}

// scoreFromSnapshot routes to the appropriate scorer based on source_type.
// Hunter/AI500 use snapshot-based pure-CPU scorers; IndicatorHub uses its own engine.
func (e *StrategyEngine) scoreFromSnapshot(snap *datafetch.Snapshot) ([]CandidateCoin, error) {
	coinSource := e.config.CoinSource
	var candidates []CandidateCoin

	switch coinSource.SourceType {
	case "hunter", "hunter_sniff":
		cfg := local.HunterSnapshotConfig{
			MinOIValue: 500_000,
			MaxSymbols: 50,
		}
		if coinSource.Hunter != nil && coinSource.Hunter.MinOIValue > 0 {
			cfg.MinOIValue = coinSource.Hunter.MinOIValue
		}
		scores := local.ScoreHunterFromSnapshot(snap, cfg)
		candidates = e.hunterScoresToCandidateCoins(scores, coinSource.HunterDirection)
		if coinSource.SourceType == "hunter_sniff" {
			candidates = e.filterSniffCandidates(candidates, coinSource.HunterSniffer)
		}

	case "hunter_v7":
		v7cfg := local.DefaultV7Config()
		if coinSource.Hunter != nil && coinSource.Hunter.V7MaxOutput > 0 {
			v7cfg.MaxOutput = coinSource.Hunter.V7MaxOutput
		}
		if coinSource.Hunter != nil && coinSource.Hunter.V7WatchOutput > 0 {
			v7cfg.WatchOutput = coinSource.Hunter.V7WatchOutput
		}
		if coinSource.Hunter != nil && coinSource.Hunter.V7MinAIPriority > 0 {
			v7cfg.MinAIPriority = coinSource.Hunter.V7MinAIPriority
		}
		if coinSource.Hunter != nil && coinSource.Hunter.V7Aggressive {
			v7cfg.Aggressive = true
		}
		if coinSource.HunterLimit > 0 && (v7cfg.MaxOutput <= 0 || coinSource.HunterLimit < v7cfg.MaxOutput) {
			v7cfg.MaxOutput = coinSource.HunterLimit
		}
		e.v7CycleCounter++
		v7cfg.CycleNumber = e.v7CycleCounter
		// Initialize watch state manager on first call
		if e.v7WatchState == nil {
			e.v7WatchState = local.NewV7SignalStateManager()
		}
		v7cfg.WatchStateManager = e.v7WatchState
		v7Result := local.ScoreHunterV7Detailed(snap, v7cfg)
		signals := v7Result.Signals
		candidates = e.hunterV7SignalsToCandidateCoins(signals, coinSource.HunterDirection)
		// Record raw v7 signals for funnel attribution if recorder is set
		if e.v7SignalRecorder != nil {
			rawSignals := v7Result.RawSignals
			if len(rawSignals) == 0 {
				rawSignals = signals
			}
			e.v7SignalRecorder(v7cfg.CycleNumber, e.buildV7SignalRecords(rawSignals, candidates), v7Result.Regime)
		}

	case "ai500":
		limit := coinSource.AI500Limit
		if limit <= 0 {
			limit = 20
		}
		coins, err := local.ScoreAI500FromSnapshot(snap, limit)
		if err != nil {
			return nil, err
		}
		candidates = e.ai500CoinsToCandidateCoins(coins)

	default:
		// IndicatorHub: use the engine's own signals
		signals := e.snapshotEngine.GetTradeSignals()
		if len(signals) == 0 {
			logger.Info("📋 IndicatorHub 返回 0 信号")
			return nil, nil
		}
		candidates = e.snapshotEngine.ConvertSignalsToCandidateCoins(signals)
	}

	logger.Infof("📋 [Snapshot] %s → %d candidate coins", coinSource.SourceType, len(candidates))
	return e.filterExcludedCoins(candidates), nil
}

// hunterV7SignalsToCandidateCoins converts Hunter v7 structured signals into
// CandidateCoin while preserving the rich signal context for the AI prompt.
func (e *StrategyEngine) hunterV7SignalsToCandidateCoins(signals []local.V7SignalOutput, direction string) []CandidateCoin {
	return AssembleHunterV7CandidateCoins(signals, direction, e.hunterV7ExecutionGeometry())
}

// AssembleHunterV7CandidateCoins is the production signal→candidate assembly,
// exported so offline tooling (cmd/hunter_v7_validate) sees exactly what the
// live engine would build instead of maintaining a hand copy (U5.3).
func AssembleHunterV7CandidateCoins(signals []local.V7SignalOutput, direction string, geometry HunterV7ExecutionGeometry) []CandidateCoin {
	var candidates []CandidateCoin
	for _, sig := range signals {
		if sig.Direction == "" {
			continue
		}
		if direction != "" && direction != "BOTH" && string(sig.Direction) != direction {
			continue
		}

		tags := make([]string, 0, 2+len(sig.ReasonCodes)+len(sig.RiskTags))
		tags = append(tags, string(sig.SetupType), string(sig.Status))
		tags = append(tags, sig.ReasonCodes...)
		tags = append(tags, sig.RiskTags...)

		vwap15m := 0.0
		if sig.PriceCtx != nil {
			vwap15m = sig.PriceCtx.VWAP15m
		}

		cc := CandidateCoin{
			Symbol:     sig.Symbol,
			Sources:    []string{"hunter_v7"},
			Direction:  string(sig.Direction),
			SignalTags: tags,
			LongTags:   nil,
			ShortTags:  nil,

			V7SignalID:         sig.SignalID,
			V7SetupType:        string(sig.SetupType),
			V7Status:           string(sig.Status),
			V7AIPriority:       sig.AIPriority,
			V7SetupScore:       sig.SetupScore,
			V7RiskScore:        sig.RiskScore,
			V7LiquidityScore:   sig.LiquidityScore,
			V7TimingScore:      sig.TimingScore,
			V7RegimeFitScore:   sig.RegimeFitScore,
			V7RiskLevel:        string(sig.RiskLevel),
			V7EntryMode:        string(sig.EntryMode),
			V7MarketShape:      string(sig.MarketShape),
			V7EntrySignal:      string(sig.EntrySignal),
			V7ExecutionQuality: string(sig.ExecutionQuality),
			V7MarketRegime:     string(sig.MarketRegime),
			V7Confidence:       sig.Confidence,
			V7ReasonCodes:      append([]string{}, sig.ReasonCodes...),
			V7RiskTags:         append([]string{}, sig.RiskTags...),
			V7RequiredConfirms: append([]string{}, sig.RequiredConfirms...),
			V7EntryZone:        sig.EntryZone,
			V7Invalidation:     sig.Invalidation,
			V7Targets:          append([]local.V7Target{}, sig.Targets...),
			V7ConfirmSummary:   sig.ConfirmSummary,
			V7PriceContext:     sig.PriceCtx,
			V7DerivativesCtx:   sig.DerivativesCtx,
			V7DataFreshness:    sig.DataFreshness,
			V7Readiness:        sig.ExecutionReadiness,
			V7ExecutionContext: sig.ExecutionContext,
			V7TP0Price:         sig.TP0Price,
			V7TP0RR:            sig.TP0RR,
			V7TP0TimeWindow:    sig.TP0TimeWindow,
			V7TP0Method:        sig.TP0Method,
			V7TP1Price:         sig.TP1Price,
			V7TP1RR:            sig.TP1RR,
			V7TP1TimeWindow:    sig.TP1TimeWindow,
			V7TP1Method:        sig.TP1Method,
			V7TP2Price:         sig.TP2Price,
			V7TP2RR:            sig.TP2RR,
			V7TP2TimeWindow:    sig.TP2TimeWindow,
			V7TP2Method:        sig.TP2Method,
			V7TPPlan:           sig.TPPlan,
			V7VWAP15m:          vwap15m,
			V7QuoteVolume24h:   sig.QuoteVolume24h,
		}
		cc.V7ExecutionTier, cc.V7TierReason = classifyHunterV7CandidateTierWithGeometry(cc, geometry)

		if sig.Direction == local.V7DirLong {
			cc.LongScore = sig.AIPriority
			cc.LongTags = tags
		} else {
			cc.ShortScore = sig.AIPriority
			cc.ShortTags = tags
		}
		cc.CapitalLevel, cc.CapitalTier = local.V7ConfidenceToCapitalLevel(sig.Confidence)
		candidates = append(candidates, cc)
	}
	return candidates
}

type HunterV7ExecutionGeometry struct {
	MaxTPMovePct float64
	MinSLMovePct float64
	MaxDriftPct  float64
	MinRR        float64
}

func HunterV7EffectiveExecutionGeometry(maxTPMovePct, minSLMovePct, maxDriftPct, minRR float64, hunterV7 bool) HunterV7ExecutionGeometry {
	geometry := HunterV7ExecutionGeometry{
		MaxTPMovePct: maxTPMovePct,
		MinSLMovePct: minSLMovePct,
		MaxDriftPct:  maxDriftPct,
		MinRR:        minRR,
	}
	if !hunterV7 {
		return geometry
	}
	if geometry.MaxTPMovePct <= 0 {
		geometry.MaxTPMovePct = 3.0
	}
	if geometry.MinSLMovePct <= 0 {
		geometry.MinSLMovePct = 2.0
	}
	if geometry.MaxDriftPct <= 0 {
		geometry.MaxDriftPct = 0.5
	}
	if geometry.MinRR <= 0 {
		geometry.MinRR = hunterV7BackendMinRR
	}
	minFeasibleTPPct := (geometry.MinSLMovePct + geometry.MaxDriftPct) * geometry.MinRR
	minFeasibleTPPct += 0.25
	if geometry.MaxTPMovePct < minFeasibleTPPct {
		geometry.MaxTPMovePct = minFeasibleTPPct
	}
	return geometry
}

func classifyHunterV7CandidateTierWithGeometry(coin CandidateCoin, geometry HunterV7ExecutionGeometry) (string, string) {
	if coin.V7SetupType == "" {
		return "", ""
	}
	if coin.V7Status == "filtered" {
		return "REJECTED", "status_filtered"
	}
	// Alt-ladder extreme continuation stays visible as WATCH instead of being
	// rejected outright, so the review pool keeps strong-but-risky movers.
	if hunterV7AltLadderExtremeContinuationWatch(coin) {
		return "WATCH", "alt_ladder_extreme_continuation_watch"
	}
	if strings.EqualFold(coin.V7RiskLevel, "EXTREME") {
		return "REJECTED", "risk_extreme"
	}
	if coin.V7ExecutionQuality == "invalid_rr" {
		return "REJECTED", "invalid_rr"
	}
	for _, tag := range coin.V7RiskTags {
		if hunterV7StaleDisplacementRRTagMaskedByLiquidity(coin, tag) {
			continue
		}
		if action, ok := local.HunterV7TagLLMAction(tag); ok && action == local.V7TagActionRejectOnly {
			return "REJECTED", tag
		}
		switch tag {
		case "risk_filtered", "liquidity_filtered", "extreme_volatility":
			return "REJECTED", tag
		}
	}
	if coin.V7LiquidityScore > 0 && coin.V7LiquidityScore < 50 {
		return "REJECTED", "liquidity_lt_50"
	}
	if coin.V7RiskScore >= 65 {
		return "REJECTED", "risk_score_gte_65"
	}

	if reason := hunterV7RequiredConfirmationWaitReason(coin); reason != "" {
		if ok, reviewReason := hunterV7LiveConfirmableReviewableReason(coin, reason); ok {
			return "REVIEWABLE", reviewReason
		}
		return "WATCH", reason
	}
	if reason := hunterV7ReadinessMissingExecutionWaitReason(coin); reason != "" {
		return "WATCH", reason
	}
	if hunterV7NonFundingCrowdedLongWait(coin) {
		return "WATCH", "crowding_extreme"
	}
	if coin.V7SetupType == "funding_reversal" &&
		containsStringValue(coin.V7RiskTags, "oi_building_no_flush") &&
		!hunterV7FundingShortMixedOIReviewAllowed(coin) {
		return "WATCH", "funding_reversal_oi_building"
	}
	if coin.V7SetupType == "funding_reversal" {
		if stopDistancePct := hunterV7StopDistancePct(coin); stopDistancePct > 0 && stopDistancePct < 2.0 {
			return "WATCH", "funding_reversal_stop_too_tight"
		}
		if hunterV7FundingShortWeakRetestFlush(coin) {
			return "WATCH", "funding_short_weak_4h_flush_retest_wait"
		}
	}
	if hunterV7LeaderMomentumLatePullbackWait(coin) {
		if ok, reason := hunterV7LeaderMomentumLatePullbackReviewable(coin); ok {
			return "REVIEWABLE", reason
		}
		return "WATCH", "momentum_late_pullback_zone_upper_wait"
	}
	if hunterV7LeaderMomentumUpperChaseWait(coin) {
		return "WATCH", "momentum_upper_zone_chase_wait"
	}
	if reason := hunterV7HighRiskSignalWaitReason(coin); reason != "" {
		// High RSI + volume expansion: REVIEWABLE with position_reduce instead of WATCH
		if ok, reviewReason := hunterV7HighRSIVolumeReviewable(coin); ok {
			return "REVIEWABLE", reviewReason + "|position_reduce"
		}
		return "WATCH", reason
	}
	if containsStringValue(coin.V7RiskTags, "do_not_open_until_confirmed") {
		return "WATCH", "watch_only_confirm_required"
	}
	if coin.V7ExecutionQuality == "chase_risk" {
		return "WATCH", "chase_risk_wait_reentry"
	}
	if coin.V7Status == "conflict_watch" {
		return "WATCH", "conflict_watch"
	}
	if strings.EqualFold(coin.V7RiskLevel, "HIGH") && hasHunterV7DangerRiskTag(coin.V7RiskTags) {
		return "WATCH", "risk_high_with_danger_tag"
	}
	if reason := hunterV7PanicReversalLowTimingWaitReason(coin); reason != "" {
		return "WATCH", reason
	}
	if reason := hunterV7PanicReversalTrendDownStructureWaitReason(coin); reason != "" {
		return "WATCH", reason
	}
	if reason := hunterV7CounterTrendConfirmationWaitReason(coin); reason != "" {
		return "WATCH", reason
	}
	if reason := hunterV7BackendCappedRRWaitReason(coin, geometry); reason != "" {
		return "WATCH", reason
	}
	if ok, reason := hunterV7ReadinessReviewableReason(coin); ok {
		return "REVIEWABLE", reason
	}

	if ok, reason := hunterV7ExecutableCandidateReason(coin); ok {
		return "EXECUTABLE", reason
	}
	if ok, reason := hunterV7ReviewableCandidateReason(coin); ok {
		return "REVIEWABLE", reason
	}
	// Fallback: confirmed breakout + aggressive taker → force REVIEWABLE for any setup type
	if coin.V7SetupType != "" && coin.V7RiskScore < 55 &&
		containsStringValue(coin.V7ReasonCodes, "confirmed_breakout") &&
		containsStringValue(coin.V7ReasonCodes, "taker_aggressive_buy") &&
		hunterV7TakerBuyAtLeast(coin, 0.52) {
		return "REVIEWABLE", "confirmed_breakout_aggressive_taker_force_reviewable"
	}
	if containsStringValue(coin.V7RiskTags, "context_only_low_priority") {
		return "WATCH", "context_only_low_priority"
	}

	return "WATCH", "needs_confirmation"
}

func hunterV7RequiredConfirmationWaitReason(coin CandidateCoin) string {
	if len(coin.V7RequiredConfirms) == 0 {
		return ""
	}
	if coin.V7ConfirmSummary == nil {
		return "confirmation_missing_summary"
	}
	required := make(map[string]struct{}, len(coin.V7RequiredConfirms))
	for _, code := range coin.V7RequiredConfirms {
		if code != "" {
			required[code] = struct{}{}
		}
	}
	if len(required) == 0 {
		return ""
	}
	for _, check := range coin.V7ConfirmSummary.MissingHard {
		if _, ok := required[check.Code]; ok {
			return "confirmation_missing_" + check.Code
		}
	}
	for _, check := range coin.V7ConfirmSummary.MissingReview {
		if _, ok := required[check.Code]; ok {
			return "confirmation_missing_" + check.Code
		}
	}
	for _, check := range coin.V7ConfirmSummary.ContextChecks {
		if _, ok := required[check.Code]; ok && !check.Passed {
			return "confirmation_missing_" + check.Code
		}
	}
	return ""
}

func hunterV7LiveConfirmableReviewableReason(coin CandidateCoin, waitReason string) (bool, string) {
	if waitReason == "" || !strings.HasPrefix(waitReason, "confirmation_missing_") {
		return false, ""
	}
	if coin.V7RiskScore >= 55 || hunterV7DangerRiskTagBlocksOpenReview(coin) {
		return false, ""
	}
	if coin.V7LiquidityScore > 0 && coin.V7LiquidityScore < 50 {
		return false, ""
	}
	if !hunterV7EntryZoneReachable(coin) || !hunterV7TakerBuyAligned(coin) {
		return false, ""
	}
	missing := strings.TrimPrefix(waitReason, "confirmation_missing_")
	if ok, reason := hunterV7RangeExpansionLiveReviewableReason(coin, missing); ok {
		return true, reason
	}
	if coin.V7ConfirmSummary == nil || !coin.V7ConfirmSummary.PassedHard || len(coin.V7ConfirmSummary.MissingHard) > 0 {
		return false, ""
	}
	if coin.V7Readiness != nil && len(coin.V7Readiness.MissingHard) > 0 {
		return false, ""
	}
	if !hunterV7ConfirmationCanBeLiveReviewed(missing) {
		return false, ""
	}
	if !hunterV7OpenRateCandidateFloor(coin) {
		return false, ""
	}
	if coin.V7ExecutionQuality == "chase_risk" || containsStringValue(coin.V7RiskTags, "do_not_market_chase") {
		return false, ""
	}
	return true, "live_reviewable_" + missing
}

func hunterV7RangeExpansionLiveReviewableReason(coin CandidateCoin, missing string) (bool, string) {
	if coin.V7SetupType != "range_expansion_event" {
		return false, ""
	}
	switch missing {
	case "summary",
		"fresh_micro_confirmed",
		"15m_close_above_vwap_or_ema20_or_entry_zone_upper",
		"15m_close_below_vwap_or_ema20_or_entry_zone_lower",
		"taker_buy_15m_gt_0_52",
		"taker_buy_15m_lt_0_48",
		"no_new_low_after_reclaim",
		"no_new_high_after_rejection":
	default:
		return false, ""
	}
	if coin.V7AIPriority < 58 ||
		coin.V7SetupScore < 70 ||
		coin.V7TimingScore < 55 ||
		coin.V7RiskScore >= 35 {
		return false, ""
	}
	if coin.V7LiquidityScore > 0 && coin.V7LiquidityScore < 70 {
		return false, ""
	}
	if containsAnyStringValue(coin.V7RiskTags, []string{
		"funding_extreme",
		"high_volatility",
		"range_expansion_exhaustion",
		"micro_reversal_against_signal",
		"late_short_after_deep_drop",
		"short_after_fast_drop_without_flush",
	}) {
		return false, ""
	}
	if coin.V7Readiness != nil {
		if len(coin.V7Readiness.MissingHard) > 0 || len(coin.V7Readiness.MissingExecution) > 0 {
			return false, ""
		}
		if coin.V7Readiness.ReadyScore > 0 && coin.V7Readiness.ReadyScore < 75 {
			return false, ""
		}
		if coin.V7Readiness.WindowHealth > 0 && coin.V7Readiness.WindowHealth < 80 {
			return false, ""
		}
		if coin.V7Readiness.EntryZonePos > 0 && coin.V7Readiness.EntryZonePos > 70 {
			return false, ""
		}
	}
	if coin.V7ConfirmSummary != nil && (!coin.V7ConfirmSummary.PassedHard || len(coin.V7ConfirmSummary.MissingHard) > 0) {
		return false, ""
	}
	if !hunterV7EntryZoneReachable(coin) {
		return false, ""
	}
	if strings.EqualFold(coin.Direction, "SHORT") {
		if !containsAnyStringValue(coin.V7ReasonCodes, []string{"event_breakdown_short", "event_directional_followthrough", "range_expansion_continuation"}) ||
			!containsStringValue(coin.V7ReasonCodes, "taker_sell_aligned") ||
			!hunterV7TakerBuyAtMost(coin, 0.48) {
			return false, ""
		}
		return true, "range_expansion_live_reviewable_short_" + missing
	}
	if !containsAnyStringValue(coin.V7ReasonCodes, []string{"event_continuation_long", "event_directional_followthrough", "range_expansion_continuation"}) ||
		!containsAnyStringValue(coin.V7ReasonCodes, []string{"taker_buy_aligned", "taker_buy_strong", "taker_aggressive_buy"}) ||
		!hunterV7TakerBuyAtLeast(coin, 0.52) {
		return false, ""
	}
	return true, "range_expansion_live_reviewable_long_" + missing
}

func hunterV7ConfirmationCanBeLiveReviewed(code string) bool {
	return local.V7ConfirmLiveReviewable(code)
}

func hunterV7OpenRateCandidateFloor(coin CandidateCoin) bool {
	if spec, ok := hunterV7SetupTierSpecs[coin.V7SetupType]; ok && spec.OpenRateFloor != nil {
		matched, _ := hunterV7EvalTierRules(coin, spec.OpenRateFloor)
		return matched
	}
	// Shared default floor for setups without a dedicated OpenRateFloor.
	return coin.V7AIPriority >= 60 &&
		coin.V7SetupScore >= 55 &&
		coin.V7TimingScore >= 55 &&
		coin.V7RiskScore < 40
}

// ClassifyHunterV7CandidateTierForRuntime exposes the same prompt tiering rules
// to runtime filters so stale WAIT cooling and LLM prompt expansion stay aligned.
// Geometry is required so every caller classifies against the same configured
// execution constraints instead of silently falling back to defaults.
func ClassifyHunterV7CandidateTierForRuntime(coin CandidateCoin, geometry HunterV7ExecutionGeometry) (string, string) {
	return classifyHunterV7CandidateTierWithGeometry(coin, geometry)
}

func hunterV7StaleDisplacementRRTagMaskedByLiquidity(coin CandidateCoin, tag string) bool {
	if tag != "displacement_rr_insufficient" || coin.V7SetupType != "displacement_momentum_long" {
		return false
	}
	if coin.V7LiquidityScore <= 0 || coin.V7LiquidityScore >= 50 {
		return false
	}
	rr, ok := hunterV7FinalConfirmationRR(coin)
	return ok && rr >= hunterV7BackendMinRR
}

func hunterV7NonFundingCrowdedLongWait(coin CandidateCoin) bool {
	return strings.EqualFold(coin.Direction, "LONG") &&
		coin.V7SetupType != "funding_reversal" &&
		containsStringValue(coin.V7RiskTags, "crowding_extreme")
}

func hunterV7AltLadderExtremeContinuationWatch(coin CandidateCoin) bool {
	return coin.V7SetupType == "alt_ladder_momentum_long" &&
		containsStringValue(coin.V7RiskTags, "alt_ladder_extreme_continuation_watch") &&
		containsStringValue(coin.V7ReasonCodes, "alt_ladder_stage_extreme")
}

func hunterV7StopTightenedNeedsReview(coin CandidateCoin) bool {
	if !containsStringValue(coin.V7RiskTags, "execution_stop_tightened") {
		return false
	}
	switch coin.V7SetupType {
	case "alt_ladder_momentum_long", "alt_ladder_breakdown_short", "mms_trend_ride_long":
	default:
		return false
	}
	if coin.V7AIPriority < 80 || coin.V7TimingScore < 70 || coin.V7RiskScore >= 20 {
		return true
	}
	if strings.EqualFold(coin.Direction, "LONG") {
		return !hunterV7TakerBuyAtLeast(coin, 0.56)
	}
	if strings.EqualFold(coin.Direction, "SHORT") {
		return !hunterV7TakerBuyAtMost(coin, 0.44)
	}
	return true
}

func hunterV7FinalConfirmationRR(coin CandidateCoin) (float64, bool) {
	if coin.V7ConfirmSummary != nil && coin.V7ConfirmSummary.RR > 0 {
		return coin.V7ConfirmSummary.RR, true
	}
	price := hunterV7ReferencePrice(coin)
	if price <= 0 || coin.V7Invalidation.Price <= 0 || len(coin.V7Targets) == 0 {
		return 0, false
	}
	risk := 0.0
	if strings.EqualFold(coin.Direction, "SHORT") {
		risk = coin.V7Invalidation.Price - price
	} else if strings.EqualFold(coin.Direction, "LONG") {
		risk = price - coin.V7Invalidation.Price
	}
	if risk <= 0 {
		return 0, false
	}
	bestReward := 0.0
	for _, target := range coin.V7Targets {
		reward := hunterV7DirectionalRewardPct(price, target.Price, coin.Direction)
		if reward > bestReward {
			bestReward = reward
		}
	}
	if bestReward <= 0 {
		return 0, false
	}
	riskPct := risk / price * 100
	return bestReward / riskPct, true
}

func hunterV7ExecutableCandidateReason(coin CandidateCoin) (bool, string) {
	if coin.V7RiskScore >= 65 || hunterV7DangerRiskTagBlocksOpenReview(coin) {
		return false, ""
	}
	if coin.V7Readiness != nil && len(coin.V7Readiness.MissingExecution) > 0 {
		return false, ""
	}
	if hunterV7DirectOpenWaitOnlyReason(coin) != "" {
		return false, ""
	}
	if hunterV7StopTightenedNeedsReview(coin) || hunterV7DisplacementReviewOnlyRisk(coin) {
		return false, ""
	}
	if coin.V7SetupType == "funding_reversal" && containsStringValue(coin.V7RiskTags, "oi_building_no_flush") {
		return false, ""
	}
	if coin.V7ExecutionQuality == "ready" {
		return hunterV7ReadyExecutableReason(coin)
	}
	if coin.V7EntrySignal == "entry_open_now" {
		return hunterV7ReadyExecutableReason(coin)
	}
	if coin.V7ExecutionQuality == "near_confirm" || coin.V7Status == "candidate" {
		return hunterV7NearConfirmExecutableReason(coin)
	}
	return false, ""
}

func hunterV7ReadyExecutableReason(coin CandidateCoin) (bool, string) {
	if spec, ok := hunterV7SetupTierSpecs[coin.V7SetupType]; ok {
		return hunterV7EvalTierRules(coin, spec.Ready)
	}
	// Unregistered (future) setups fall back to the generic ready floor until
	// their spec lands.
	if coin.V7AIPriority >= 60 && coin.V7TimingScore >= 60 && coin.V7RiskScore < 55 {
		return true, "execution_quality_ready"
	}
	return false, ""
}

func hunterV7NearConfirmExecutableReason(coin CandidateCoin) (bool, string) {
	if spec, ok := hunterV7SetupTierSpecs[coin.V7SetupType]; ok {
		return hunterV7EvalTierRules(coin, spec.NearConfirm)
	}
	return false, ""
}

func hunterV7ReviewableCandidateReason(coin CandidateCoin) (bool, string) {
	if coin.V7RiskScore >= 65 || hunterV7DangerRiskTagBlocksOpenReview(coin) {
		return false, ""
	}
	if coin.V7LiquidityScore > 0 && coin.V7LiquidityScore < 50 {
		return false, ""
	}
	if coin.V7ExecutionQuality == "near_confirm" &&
		coin.V7AIPriority >= 45 &&
		coin.V7SetupScore >= 45 &&
		coin.V7TimingScore >= 45 &&
		coin.V7RiskScore < 45 &&
		hunterV7TakerBuyAligned(coin) &&
		hunterV7ConfirmationSummaryReviewPassed(coin) {
		return true, "near_confirm_reviewable_micro_confirmed"
	}
	if containsStringValue(coin.V7ReasonCodes, "trigger_memory_confirmed") &&
		(coin.V7SetupType == "trend_breakout_long" || coin.V7SetupType == "accumulation_breakout_long") &&
		coin.V7AIPriority >= 45 &&
		coin.V7SetupScore >= 50 &&
		coin.V7RiskScore < 55 &&
		(coin.V7LiquidityScore == 0 || coin.V7LiquidityScore >= 70) &&
		hunterV7TakerBuyAligned(coin) {
		return true, "breakout_trigger_memory_confirmed_reviewable"
	}
	if coin.V7EntrySignal == "entry_trigger_near" &&
		(coin.V7SetupType == "trend_breakout_long" || coin.V7SetupType == "accumulation_breakout_long") &&
		coin.V7SetupScore >= 78 &&
		coin.V7AIPriority >= 45 &&
		coin.V7RiskScore < 55 &&
		(coin.V7LiquidityScore == 0 || coin.V7LiquidityScore >= 70) &&
		hunterV7TakerBuyAligned(coin) {
		return true, "breakout_trigger_near_reviewable"
	}
	if hunterV7BreakoutTriggerNearFlowReviewable(coin) {
		return true, "breakout_trigger_near_flow_reviewable"
	}
	if spec, ok := hunterV7SetupTierSpecs[coin.V7SetupType]; ok {
		return hunterV7EvalTierRules(coin, spec.Reviewable)
	}
	return false, ""
}

func hunterV7PanicReversalCoreFlowOK(coin CandidateCoin) bool {
	if containsStringValue(coin.V7ReasonCodes, "low_timing_watch_only") ||
		coin.V7TimingScore <= 40 ||
		coin.V7ExecutionQuality == "watch_only" {
		return hunterV7TakerBuyConfirmedAtLeast(coin, 0.52)
	}
	return hunterV7TakerBuyAtLeast(coin, 0.50)
}

func hunterV7ReadinessReviewableReason(coin CandidateCoin) (bool, string) {
	if coin.V7Readiness == nil || coin.V7Readiness.Tier != local.V7ReadinessReviewable {
		return false, ""
	}
	if coin.V7RiskScore >= 55 || hunterV7DangerRiskTagBlocksOpenReview(coin) {
		return false, ""
	}
	if coin.V7LiquidityScore > 0 && coin.V7LiquidityScore < 50 {
		return false, ""
	}
	if !hunterV7EntryZoneReachable(coin) || !hunterV7TakerBuyAligned(coin) {
		return false, ""
	}
	if len(coin.V7Readiness.MissingHard) > 0 {
		return false, ""
	}
	minReady := 62.0
	switch coin.V7SetupType {
	case "trend_breakout_long", "accumulation_breakout_long", "displacement_momentum_long", "leader_momentum_long":
		minReady = 60
	case "panic_reversal_long", "funding_reversal", "range_reversion":
		minReady = 68
	}
	if coin.V7Readiness.ReadyScore > 0 && coin.V7Readiness.ReadyScore < minReady {
		return false, ""
	}
	if coin.V7Readiness.BlockedGate == "execution_geometry" || coin.V7Readiness.BlockedGate == "prompt_data_quality" {
		return false, ""
	}
	return true, "readiness_reviewable_" + coin.V7Readiness.Reason
}

func hunterV7ReadinessMissingExecutionWaitReason(coin CandidateCoin) string {
	if coin.V7Readiness == nil || len(coin.V7Readiness.MissingExecution) == 0 {
		return ""
	}
	return "missing_execution_" + coin.V7Readiness.MissingExecution[0]
}

func hunterV7DirectOpenWaitOnlyReason(coin CandidateCoin) string {
	for _, tag := range coin.V7ReasonCodes {
		switch tag {
		case "chase_high_protection":
			if hunterV7ConfirmedRangeExpansionContinuation(coin, true) {
				continue
			}
			if action, ok := local.HunterV7TagLLMAction(tag); ok && action == local.V7TagActionWaitOnly {
				return "wait_only_reason_" + tag
			}
		case "no_pullback_still_running", "momentum_rsi_overheated_wait":
			if action, ok := local.HunterV7TagLLMAction(tag); ok && action == local.V7TagActionWaitOnly {
				return "wait_only_reason_" + tag
			}
		}
	}
	for _, tag := range coin.V7RiskTags {
		switch tag {
		case "momentum_confirmation_missing", "momentum_overheated", "momentum_chase_risk", "do_not_market_chase":
			if action, ok := local.HunterV7TagLLMAction(tag); ok && action == local.V7TagActionWaitOnly {
				return "wait_only_risk_" + tag
			}
		}
	}
	return ""
}

func hunterV7ConfirmedRangeExpansionContinuation(coin CandidateCoin, executable bool) bool {
	if coin.V7SetupType != "range_expansion_event" {
		return false
	}
	if coin.V7Readiness == nil || !hunterV7ConfirmationSummaryReviewPassed(coin) {
		return false
	}
	if len(coin.V7Readiness.MissingHard) > 0 || len(coin.V7Readiness.MissingExecution) > 0 {
		return false
	}
	if executable {
		if coin.V7Readiness.Tier != local.V7ReadinessExecutable {
			return false
		}
		if coin.V7Readiness.ReadyScore > 0 && coin.V7Readiness.ReadyScore < 75 {
			return false
		}
		if coin.V7Readiness.EntryZonePos > 0 && coin.V7Readiness.EntryZonePos > 80 {
			return false
		}
	} else {
		if coin.V7Readiness.Tier != local.V7ReadinessExecutable && coin.V7Readiness.Tier != local.V7ReadinessReviewable {
			return false
		}
		if coin.V7Readiness.ReadyScore > 0 && coin.V7Readiness.ReadyScore < 68 {
			return false
		}
		if coin.V7Readiness.EntryZonePos > 0 && coin.V7Readiness.EntryZonePos > 90 {
			return false
		}
	}
	if hunterV7DangerRiskTagBlocksOpenReview(coin) {
		return false
	}
	if coin.V7LiquidityScore > 0 && coin.V7LiquidityScore < 50 {
		return false
	}
	if !hunterV7EntryZoneReachable(coin) {
		return false
	}
	if strings.EqualFold(coin.Direction, "SHORT") {
		return hunterV7TakerBuyConfirmedAtMost(coin, 0.48)
	}
	return hunterV7TakerBuyConfirmedAtLeast(coin, 0.52)
}

func hunterV7LeaderMomentumFlexibleReviewableReason(coin CandidateCoin) (bool, string) {
	if coin.V7SetupType != "leader_momentum_long" || !strings.EqualFold(coin.Direction, "LONG") {
		return false, ""
	}
	if coin.V7ExecutionQuality == "chase_risk" ||
		containsAnyStringValue(coin.V7RiskTags, []string{"momentum_confirmation_missing", "momentum_overheated", "momentum_chase_risk"}) ||
		containsStringValue(coin.V7ReasonCodes, "momentum_rsi_overheated_wait") {
		return false, ""
	}
	if coin.V7RiskScore >= 55 || hunterV7DangerRiskTagBlocksOpenReview(coin) {
		return false, ""
	}
	if coin.V7LiquidityScore > 0 && coin.V7LiquidityScore < 50 {
		return false, ""
	}
	if containsAnyStringValue(coin.V7RiskTags, []string{"do_not_market_chase", "momentum_crowded_long", "funding_extreme"}) {
		return false, ""
	}
	if coin.V7Invalidation.Price <= 0 || len(coin.V7Targets) == 0 || coin.V7Targets[0].Price <= 0 {
		return false, ""
	}
	if containsStringValue(coin.V7ReasonCodes, "taker_weak_buy") || !hunterV7EntryZoneReachable(coin) {
		return false, ""
	}

	evidence := hunterV7LeaderMomentumEvidenceCount(coin)
	if evidence < 3 {
		return false, ""
	}
	score := hunterV7LeaderMomentumReviewScore(coin, evidence)
	if score < 62 {
		return false, ""
	}
	if coin.V7AIPriority < 50 && coin.V7SetupScore < 70 {
		return false, ""
	}
	if coin.V7TimingScore < 50 && !containsAnyStringValue(coin.V7ReasonCodes, []string{"accelerating_1h", "holding_1h", "shallow_pullback", "micro_pullback"}) {
		return false, ""
	}
	if !hunterV7TakerBuyAligned(coin) {
		return false, ""
	}
	return true, "momentum_reviewable_flexible_score"
}

func hunterV7LeaderMomentumEvidenceCount(coin CandidateCoin) int {
	evidence := 0
	for _, code := range coin.V7ReasonCodes {
		switch code {
		case "strong_24h_momentum", "solid_24h_momentum", "moderate_24h_momentum",
			"strong_4h_momentum", "solid_4h_momentum",
			"accelerating_1h", "holding_1h",
			"oi_healthy_growth", "oi_moderate_growth", "oi_explosive_growth",
			"taker_sustained_buy", "taker_aggressive_buy", "taker_neutral_buy",
			"volume_expansion", "confirmed_breakout",
			"micro_pullback", "shallow_pullback", "shallow_pullback_1h":
			evidence++
		}
	}
	if strings.EqualFold(coin.V7Confidence, "A") || strings.EqualFold(coin.V7Confidence, "B") {
		evidence++
	}
	return evidence
}

func hunterV7LeaderMomentumReviewScore(coin CandidateCoin, evidence int) float64 {
	flow := 50.0
	if coin.V7DerivativesCtx != nil && coin.V7DerivativesCtx.TakerBuy15m > 0 {
		flow = 50 + (coin.V7DerivativesCtx.TakerBuy15m-0.50)*160
		if flow < 35 {
			flow = 35
		}
		if flow > 100 {
			flow = 100
		}
	}
	score := coin.V7SetupScore*0.24 + coin.V7TimingScore*0.20 + coin.V7AIPriority*0.18 +
		coin.V7RegimeFitScore*0.10 + coin.V7LiquidityScore*0.06 + flow*0.10 + float64(evidence)*3.0
	score -= maxFloat64(0, coin.V7RiskScore-35) * 0.45
	if coin.V7ExecutionQuality == "ready" {
		score += 4
	}
	if coin.V7ExecutionQuality == "chase_risk" {
		score -= 4
	}
	if coin.V7ConfirmSummary != nil && coin.V7ConfirmSummary.PassedHard {
		score += 3
	}
	return score
}

func hunterV7WatchUpgradedReviewable(coin CandidateCoin) bool {
	if coin.V7ExecutionQuality != "near_confirm" &&
		!containsStringValue(coin.V7ReasonCodes, "watch_upgraded_reviewable") {
		return false
	}
	if !containsStringValue(coin.V7ReasonCodes, "multi_cycle_confirmation") {
		return false
	}
	if coin.V7AIPriority < 40 || coin.V7SetupScore < 35 || coin.V7TimingScore < 25 || coin.V7RiskScore >= 55 {
		return false
	}
	if coin.V7LiquidityScore > 0 && coin.V7LiquidityScore < 50 {
		return false
	}
	if strings.EqualFold(coin.Direction, "SHORT") {
		return hunterV7TakerBuyAtMost(coin, 0.50)
	}
	return hunterV7TakerBuyAtLeast(coin, 0.50)
}

func hunterV7ChaseRiskReviewableReason(coin CandidateCoin) (bool, string) {
	if coin.V7RiskScore >= 35 || hasHunterV7DangerRiskTag(coin.V7RiskTags) {
		return false, ""
	}
	if coin.V7LiquidityScore > 0 && coin.V7LiquidityScore < 50 {
		return false, ""
	}
	if coin.V7SetupType == "leader_momentum_long" && hunterV7LeaderMomentumOverheatedOrUnconfirmed(coin) {
		return hunterV7LeaderMomentumFlexibleReviewableReason(coin)
	}
	switch coin.V7SetupType {
	case "leader_momentum_long":
		if coin.V7AIPriority >= 45 &&
			coin.V7SetupScore >= 90 &&
			coin.V7TimingScore >= 45 &&
			hunterV7TakerBuyAtLeast(coin, 0.50) &&
			!containsStringValue(coin.V7ReasonCodes, "taker_weak_buy") {
			return true, "momentum_chase_risk_reviewable_pullback_only"
		}
	case "displacement_momentum_long":
		if coin.V7AIPriority >= 45 &&
			coin.V7SetupScore >= 55 &&
			coin.V7TimingScore >= 40 &&
			hunterV7TakerBuyAtLeast(coin, 0.50) {
			return true, "displacement_chase_risk_reviewable_entry_valid"
		}
	}
	// General chase-risk reviewable: entry zone still reachable + taker flow aligned
	if coin.V7AIPriority >= 50 &&
		coin.V7SetupScore >= 55 &&
		coin.V7TimingScore >= 40 &&
		coin.V7RiskScore < 30 &&
		hunterV7EntryZoneReachable(coin) &&
		hunterV7TakerBuyAligned(coin) {
		return true, "chase_risk_reviewable_entry_zone_wait"
	}
	return false, ""
}

func hunterV7LeaderMomentumOverheatedOrUnconfirmed(coin CandidateCoin) bool {
	if containsStringValue(coin.V7RiskTags, "momentum_overheated") ||
		containsStringValue(coin.V7ReasonCodes, "momentum_rsi_overheated_wait") {
		return true
	}
	if coin.V7ConfirmSummary != nil && !coin.V7ConfirmSummary.PassedReview {
		return true
	}
	return false
}

// hunterV7EntryZoneReachable checks if price is within or near the entry zone.
func hunterV7EntryZoneReachable(coin CandidateCoin) bool {
	if coin.V7PriceContext == nil || coin.V7EntryZone.Lower <= 0 {
		return false
	}
	price := coin.V7PriceContext.Last
	if price <= 0 {
		return false
	}
	// Price within 3% above entry zone upper or within zone
	if strings.EqualFold(coin.Direction, "LONG") {
		return price <= coin.V7EntryZone.Upper*1.03
	}
	// SHORT: price within 3% below entry zone lower or within zone
	return price >= coin.V7EntryZone.Lower*0.97
}

// hunterV7TakerBuyAligned checks if taker flow is aligned with direction.
func hunterV7TakerBuyAligned(coin CandidateCoin) bool {
	if coin.V7DerivativesCtx == nil || coin.V7DerivativesCtx.TakerBuy15m <= 0 {
		return true // no data = don't penalize
	}
	tb := coin.V7DerivativesCtx.TakerBuy15m
	if strings.EqualFold(coin.Direction, "LONG") {
		return tb >= 0.50
	}
	return tb <= 0.50
}

// hunterV7HighRSIVolumeReviewable checks if a high-RSI signal qualifies for
// REVIEWABLE with position_reduce due to volume/OI expansion continuation.
func hunterV7HighRSIVolumeReviewable(coin CandidateCoin) (bool, string) {
	if coin.V7RiskScore >= 55 || hasHunterV7DangerRiskTag(coin.V7RiskTags) {
		return false, ""
	}
	if coin.V7LiquidityScore > 0 && coin.V7LiquidityScore < 50 {
		return false, ""
	}
	if coin.V7AIPriority < 48 || coin.V7SetupScore < 50 {
		return false, ""
	}
	// Require volume/OI expansion confirmation
	hasVolumeExpansion := containsStringValue(coin.V7ReasonCodes, "volume_expansion") ||
		containsStringValue(coin.V7ReasonCodes, "oi_massive_flush") ||
		containsStringValue(coin.V7ReasonCodes, "oi_heavy_flush")
	if !hasVolumeExpansion {
		return false, ""
	}
	// Require taker flow aligned
	if !hunterV7TakerBuyAligned(coin) {
		return false, ""
	}
	return true, "high_rsi_volume_expansion_trailing_entry"
}

func hunterV7PanicReversalHasHighWinReclaim(coin CandidateCoin) bool {
	reclaim := containsAnyStringValue(coin.V7ReasonCodes, []string{
		"strong_reclaim",
		"solid_reclaim",
		"early_reclaim",
	})
	if !reclaim {
		return false
	}

	confirmations := 0
	for _, reason := range coin.V7ReasonCodes {
		switch reason {
		case "taker_buy_aggressive", "taker_buy_strong", "taker_buy_recovering",
			"selling_exhaustion", "selling_decelerating",
			"oi_massive_flush", "oi_heavy_flush", "oi_flush", "oi_declining",
			"1h_green_shoot", "rsi_recovering_from_extreme":
			confirmations++
		}
	}
	if strings.EqualFold(coin.V7Confidence, "A") || strings.EqualFold(coin.V7Confidence, "B") {
		confirmations++
	}
	return confirmations >= 3
}

func hunterV7PanicReversalLowTimingWaitReason(coin CandidateCoin) string {
	if coin.V7SetupType != "panic_reversal_long" {
		return ""
	}
	if coin.V7ExecutionQuality == "ready" && coin.V7TimingScore >= 45 &&
		!containsStringValue(coin.V7ReasonCodes, "low_timing_watch_only") {
		return ""
	}
	lowTiming := coin.V7TimingScore <= 30 ||
		coin.V7ExecutionQuality == "watch_only" ||
		containsStringValue(coin.V7ReasonCodes, "low_timing_watch_only")
	if !lowTiming {
		return ""
	}
	if containsStringValue(coin.V7ReasonCodes, "reviewable_floor_rescue") &&
		coin.V7ExecutionQuality == "near_confirm" &&
		coin.V7SetupScore >= 65 &&
		coin.V7TimingScore >= 35 &&
		coin.V7RiskScore < 45 &&
		hunterV7TakerBuyConfirmedAtLeast(coin, 0.52) {
		return ""
	}
	if hunterV7PanicReversalLowTimingImpulseOK(coin) {
		return ""
	}
	if containsAnyStringValue(coin.V7RiskTags, []string{"regime_against_direction", "high_volatility"}) {
		return "panic_reversal_low_timing_confirmation_wait"
	}
	if coin.V7Status == "wait_confirm" {
		return "panic_reversal_low_timing_confirmation_wait"
	}
	return ""
}

func hunterV7PanicReversalLowTimingImpulseOK(coin CandidateCoin) bool {
	if !hunterV7TakerBuyConfirmedAtLeast(coin, 0.62) {
		return false
	}
	if coin.V7PriceContext == nil || coin.V7DerivativesCtx == nil {
		return false
	}
	if coin.V7PriceContext.Change1h < 2.0 || coin.V7PriceContext.Change1h > 8.0 {
		return false
	}
	if coin.V7DerivativesCtx.OIChange1h > 0 || coin.V7DerivativesCtx.OIChange4h >= 0 {
		return false
	}
	return hunterV7EntryZoneReachable(coin)
}

func hunterV7PanicReversalTrendDownStructureWaitReason(coin CandidateCoin) string {
	if coin.V7SetupType != "panic_reversal_long" || !strings.EqualFold(coin.Direction, "LONG") {
		return ""
	}
	if coin.V7PriceContext == nil {
		return ""
	}
	if coin.V7TimingScore > 45 {
		return ""
	}
	if !strings.EqualFold(coin.V7MarketRegime, "trend_down") &&
		!containsStringValue(coin.V7RiskTags, "regime_against_direction") {
		return ""
	}
	if coin.V7PriceContext.Change24h > -20 || coin.V7PriceContext.Change4h >= 0 {
		return ""
	}
	if hunterV7TrendDownPanicReversalStrongEnough(coin) {
		return ""
	}
	return "panic_reversal_trend_down_structure_wait"
}

func hunterV7CounterTrendConfirmationWaitReason(coin CandidateCoin) string {
	if !containsStringValue(coin.V7RiskTags, "regime_against_direction") {
		return ""
	}
	if coin.V7ConfirmSummary == nil || coin.V7ConfirmSummary.PassedReview {
		return ""
	}
	return "countertrend_confirmation_wait"
}

func hunterV7TrendDownPanicReversalStrongEnough(coin CandidateCoin) bool {
	if !hunterV7TakerBuyConfirmedAtLeast(coin, 0.62) {
		return false
	}
	if coin.V7PriceContext == nil || coin.V7PriceContext.Change1h < 3.0 {
		return false
	}
	if coin.V7PriceContext.Change4h <= -1.0 {
		return false
	}
	if !containsStringValue(coin.V7ReasonCodes, "strong_reclaim") {
		return false
	}
	if !containsAnyStringValue(coin.V7ReasonCodes, []string{"selling_decelerating", "selling_exhaustion"}) {
		return false
	}
	return containsAnyStringValue(coin.V7ReasonCodes, []string{"oi_declining", "oi_flush", "oi_heavy_flush", "oi_massive_flush"})
}

func hunterV7FundingShortMixedOIReviewAllowed(coin CandidateCoin) bool {
	if !strings.EqualFold(coin.Direction, "SHORT") {
		return false
	}
	if hunterV7FundingReversalOIStateForTier(coin) != "mixed" {
		return false
	}
	if coin.V7AIPriority < 55 || coin.V7TimingScore < 55 || coin.V7RiskScore >= 55 {
		return false
	}
	if !hunterV7TakerBuyConfirmedAtMost(coin, 0.42) {
		return false
	}
	return containsAnyStringValue(coin.V7ReasonCodes, []string{
		"strong_taker_sell_reversal",
	}) && containsAnyStringValue(coin.V7ReasonCodes, []string{
		"extreme_long_crowding",
		"heavy_long_crowding",
		"long_crowding",
	})
}

func hunterV7FundingShortWeakRetestFlush(coin CandidateCoin) bool {
	if coin.V7SetupType != "funding_reversal" || !strings.EqualFold(coin.Direction, "SHORT") {
		return false
	}
	if !strings.EqualFold(coin.V7Confidence, "C") && coin.V7AIPriority >= 60 {
		return false
	}
	return containsStringValue(coin.V7RiskTags, "not_near_short_retest_zone") &&
		(containsStringValue(coin.V7RiskTags, "weak_4h_oi_flush") ||
			containsStringValue(coin.V7ReasonCodes, "funding_short_weak_4h_flush_wait"))
}

func hunterV7FundingReversalOIStateForTier(coin CandidateCoin) string {
	if coin.V7DerivativesCtx == nil {
		return ""
	}
	oi1h := coin.V7DerivativesCtx.OIChange1h
	oi4h := coin.V7DerivativesCtx.OIChange4h
	if oi1h < -0.2 && oi4h <= 0 {
		return "flush"
	}
	if oi1h <= 0 && oi4h < -0.5 {
		return "failed_rebuild_or_declining"
	}
	if oi1h > 0 && oi4h < 0 {
		return "mixed"
	}
	if oi1h > 0 && oi4h >= 0 {
		return "building"
	}
	return "neutral"
}

func hunterV7LeaderMomentumHasCleanPullback(coin CandidateCoin) bool {
	if containsStringValue(coin.V7ReasonCodes, "taker_weak_buy") {
		return false
	}
	for _, tag := range coin.V7RiskTags {
		switch tag {
		case "funding_extreme", "momentum_crowded_long", "momentum_overheated", "momentum_chase_risk", "squeeze_chase_risk", "do_not_market_chase":
			return false
		}
	}
	for _, reason := range coin.V7ReasonCodes {
		switch reason {
		case "micro_pullback", "shallow_pullback", "shallow_pullback_1h", "holding_1h", "accelerating_1h":
			return true
		}
	}
	return false
}

func hunterV7ConfirmationSummaryReviewPassed(coin CandidateCoin) bool {
	if coin.V7ConfirmSummary == nil {
		return false
	}
	return coin.V7ConfirmSummary.PassedHard && coin.V7ConfirmSummary.PassedReview
}

func hunterV7LeaderMomentumLatePullbackWait(coin CandidateCoin) bool {
	if coin.V7SetupType != "leader_momentum_long" || !strings.EqualFold(coin.Direction, "LONG") {
		return false
	}
	if coin.V7PriceContext == nil {
		return false
	}
	pos, ok := local.V7ZonePositionPct(coin.V7EntryZone, coin.V7PriceContext.Last)
	if !ok || pos < 70 {
		return false
	}
	pullback := coin.V7PriceContext.Change1h < 0 ||
		containsAnyStringValue(coin.V7ReasonCodes, []string{"micro_pullback", "shallow_pullback", "shallow_pullback_1h"})
	if !pullback {
		return false
	}
	if coin.V7DerivativesCtx != nil && coin.V7DerivativesCtx.TakerBuy15m >= 0.58 {
		return false
	}
	return true
}

func hunterV7LeaderMomentumLatePullbackReviewable(coin CandidateCoin) (bool, string) {
	if coin.V7AIPriority < 70 || coin.V7SetupScore < 75 || coin.V7TimingScore < 55 || coin.V7RiskScore >= 35 {
		return false, ""
	}
	if coin.V7LiquidityScore > 0 && coin.V7LiquidityScore < 50 {
		return false, ""
	}
	if containsStringValue(coin.V7ReasonCodes, "taker_weak_buy") ||
		containsStringValue(coin.V7RiskTags, "momentum_confirmation_missing") {
		return false, ""
	}
	if !hunterV7EntryZoneReachable(coin) || !hunterV7TakerBuyAtLeast(coin, 0.52) {
		return false, ""
	}
	if !containsAnyStringValue(coin.V7ReasonCodes, []string{"strong_4h_momentum", "taker_sustained_buy", "taker_buy_aggressive"}) {
		return false, ""
	}
	return true, "momentum_late_pullback_reviewable_trailing_entry"
}

func hunterV7HighRiskSignalWaitReason(coin CandidateCoin) string {
	switch coin.V7SetupType {
	case "panic_reversal_long":
		if containsStringValue(coin.V7RiskTags, "no_reclaim_signal") {
			return "panic_reversal_no_reclaim_wait"
		}
		if containsStringValue(coin.V7RiskTags, "oi_up_price_down") {
			return "panic_reversal_oi_up_price_down_wait"
		}
	case "funding_reversal":
		if strings.EqualFold(coin.Direction, "SHORT") &&
			containsStringValue(coin.V7RiskTags, "not_near_short_retest_zone") {
			return "funding_short_retest_zone_wait"
		}
		if strings.EqualFold(coin.Direction, "LONG") &&
			containsStringValue(coin.V7RiskTags, "not_near_long_reclaim_zone") {
			return "funding_long_reclaim_zone_wait"
		}
		if containsAnyStringValue(coin.V7RiskTags, []string{
			"late_short_after_deep_drop",
			"short_after_fast_drop_without_flush",
			"late_long_after_deep_pump",
			"long_after_fast_pump_without_flush",
		}) {
			return "funding_reversal_late_chase_no_flush"
		}
	case "short_squeeze_long":
		if containsAnyStringValue(coin.V7RiskTags, []string{
			"already_pumped_24h",
			"lsr_extreme_long",
			"funding_expensive",
		}) {
			return "short_squeeze_crowded_or_exhausted_wait"
		}
	case "accumulation_breakout_long":
		if containsStringValue(coin.V7RiskTags, "taker_sell_during_accumulation") {
			return "accumulation_sell_flow_wait"
		}
	case "pullback_reversal_long":
		if containsStringValue(coin.V7RiskTags, "not_near_long_reclaim_zone") {
			return "pullback_long_reclaim_zone_wait"
		}
	case "distribution_short", "long_squeeze_short", "range_reversion":
		if containsStringValue(coin.V7RiskTags, "not_near_short_retest_zone") {
			return "short_reversion_retest_zone_wait"
		}
	}
	return ""
}

func hunterV7TakerBuyAtLeast(coin CandidateCoin, threshold float64) bool {
	if coin.V7DerivativesCtx == nil || coin.V7DerivativesCtx.TakerBuy15m <= 0 {
		return true
	}
	return coin.V7DerivativesCtx.TakerBuy15m >= threshold
}

func hunterV7TakerBuyConfirmedAtLeast(coin CandidateCoin, threshold float64) bool {
	if coin.V7DerivativesCtx == nil || coin.V7DerivativesCtx.TakerBuy15m <= 0 {
		return false
	}
	return coin.V7DerivativesCtx.TakerBuy15m >= threshold
}

func hunterV7TakerBuyConfirmedAtMost(coin CandidateCoin, threshold float64) bool {
	if coin.V7DerivativesCtx == nil || coin.V7DerivativesCtx.TakerBuy15m <= 0 {
		return false
	}
	return coin.V7DerivativesCtx.TakerBuy15m <= threshold
}

func hunterV7TakerBuyAtMost(coin CandidateCoin, threshold float64) bool {
	if coin.V7DerivativesCtx == nil || coin.V7DerivativesCtx.TakerBuy15m <= 0 {
		return true
	}
	return coin.V7DerivativesCtx.TakerBuy15m <= threshold
}

func hunterV7StopDistancePct(coin CandidateCoin) float64 {
	if coin.V7Invalidation.Price <= 0 {
		return 0
	}
	price := 0.0
	if coin.V7PriceContext != nil && coin.V7PriceContext.Last > 0 {
		price = coin.V7PriceContext.Last
	}
	if price <= 0 {
		if strings.EqualFold(coin.Direction, "SHORT") && coin.V7EntryZone.Lower > 0 {
			price = coin.V7EntryZone.Lower
		}
		if strings.EqualFold(coin.Direction, "LONG") && coin.V7EntryZone.Upper > 0 {
			price = coin.V7EntryZone.Upper
		}
	}
	if price <= 0 {
		return 0
	}
	switch {
	case strings.EqualFold(coin.Direction, "SHORT"):
		return (coin.V7Invalidation.Price - price) / price * 100
	case strings.EqualFold(coin.Direction, "LONG"):
		return (price - coin.V7Invalidation.Price) / price * 100
	default:
		return 0
	}
}

const (
	hunterV7BackendMinRR            = local.V7MinExecutableRR
	hunterV7BackendMinStopPct       = 2.0
	hunterV7BackendStopRepairBuffer = 0.10
)

func hunterV7BackendCappedRRWaitReason(coin CandidateCoin, geometry HunterV7ExecutionGeometry) string {
	price := hunterV7ReferencePrice(coin)
	if price <= 0 || coin.V7Invalidation.Price <= 0 || len(coin.V7Targets) == 0 {
		return ""
	}
	geometry = hunterV7SetupExecutionGeometry(coin, geometry)
	if geometry.MinRR <= 0 {
		geometry.MinRR = hunterV7BackendMinRR
	}
	if geometry.MinSLMovePct <= 0 {
		geometry.MinSLMovePct = hunterV7BackendMinStopPct
	}
	if geometry.MaxTPMovePct <= 0 {
		geometry.MaxTPMovePct = HunterV7EffectiveExecutionGeometry(0, 0, 0, geometry.MinRR, true).MaxTPMovePct
	}

	riskPct := 0.0
	switch {
	case strings.EqualFold(coin.Direction, "SHORT"):
		riskPct = (coin.V7Invalidation.Price - price) / price * 100
	case strings.EqualFold(coin.Direction, "LONG"):
		riskPct = (price - coin.V7Invalidation.Price) / price * 100
	default:
		return ""
	}
	if riskPct <= 0 {
		return ""
	}

	effectiveRiskPct := riskPct
	if effectiveRiskPct < geometry.MinSLMovePct {
		effectiveRiskPct = geometry.MinSLMovePct + hunterV7BackendStopRepairBuffer
	}

	bestRewardPct := 0.0
	for _, target := range coin.V7Targets {
		rewardPct := hunterV7DirectionalRewardPct(price, target.Price, coin.Direction)
		if rewardPct <= 0 {
			continue
		}
		if rewardPct > geometry.MaxTPMovePct {
			rewardPct = geometry.MaxTPMovePct
		}
		if rewardPct > bestRewardPct {
			bestRewardPct = rewardPct
		}
	}
	if bestRewardPct <= 0 {
		return ""
	}

	if bestRewardPct/effectiveRiskPct < geometry.MinRR {
		return "backend_rr_infeasible"
	}
	return ""
}

func hunterV7SetupExecutionGeometry(coin CandidateCoin, geometry HunterV7ExecutionGeometry) HunterV7ExecutionGeometry {
	switch coin.V7SetupType {
	case "volatility_squeeze_breakout", "displacement_momentum_long":
		if geometry.MinRR <= 0 || geometry.MinRR > 1.35 {
			geometry.MinRR = 1.35
		}
		if geometry.MaxTPMovePct <= 0 || geometry.MaxTPMovePct < 12 {
			geometry.MaxTPMovePct = 12
		}
	case "intraday_scalp_long":
		if geometry.MinRR <= 0 || geometry.MinRR > 1.0 {
			geometry.MinRR = 1.0
		}
		if geometry.MaxTPMovePct <= 0 || geometry.MaxTPMovePct < 3 {
			geometry.MaxTPMovePct = 3
		}
	}
	return geometry
}

func hunterV7ReferencePrice(coin CandidateCoin) float64 {
	if coin.V7PriceContext != nil && coin.V7PriceContext.Last > 0 {
		return coin.V7PriceContext.Last
	}
	if strings.EqualFold(coin.Direction, "SHORT") && coin.V7EntryZone.Lower > 0 {
		return coin.V7EntryZone.Lower
	}
	if strings.EqualFold(coin.Direction, "LONG") && coin.V7EntryZone.Upper > 0 {
		return coin.V7EntryZone.Upper
	}
	return 0
}

func hunterV7DirectionalRewardPct(price, target float64, direction string) float64 {
	if price <= 0 || target <= 0 {
		return 0
	}
	if strings.EqualFold(direction, "SHORT") {
		return (price - target) / price * 100
	}
	if strings.EqualFold(direction, "LONG") {
		return (target - price) / price * 100
	}
	return 0
}

func hasHunterV7DangerRiskTag(tags []string) bool {
	for _, tag := range tags {
		if action, ok := local.HunterV7TagLLMAction(tag); ok && action == local.V7TagActionRejectOnly {
			return true
		}
		switch tag {
		case "do_not_market_chase", "wash_volume_high", "funding_extreme", "oi_anomaly", "extreme_volatility",
			"already_pumped_24h", "lsr_extreme_long", "funding_expensive",
			"late_short_after_deep_drop", "short_after_fast_drop_without_flush",
			"late_long_after_deep_pump", "long_after_fast_pump_without_flush",
			"taker_sell_during_accumulation", "no_reclaim_signal", "oi_up_price_down",
			"not_near_long_reclaim_zone":
			return true
		}
	}
	return false
}

func hunterV7DangerRiskTagBlocksOpenReview(coin CandidateCoin) bool {
	for _, tag := range coin.V7RiskTags {
		if action, ok := local.HunterV7TagLLMAction(tag); ok && action == local.V7TagActionRejectOnly {
			return true
		}
		switch tag {
		case "funding_extreme":
			if hunterV7FundingExtremePanicReversalReviewAllowed(coin) {
				continue
			}
			return true
		case "do_not_market_chase", "wash_volume_high", "oi_anomaly", "extreme_volatility",
			"already_pumped_24h", "lsr_extreme_long", "funding_expensive",
			"late_short_after_deep_drop", "short_after_fast_drop_without_flush",
			"late_long_after_deep_pump", "long_after_fast_pump_without_flush",
			"taker_sell_during_accumulation", "no_reclaim_signal", "oi_up_price_down",
			"not_near_long_reclaim_zone":
			return true
		}
	}
	return false
}

func hunterV7FundingExtremePanicReversalReviewAllowed(coin CandidateCoin) bool {
	if coin.V7SetupType != "panic_reversal_long" || !strings.EqualFold(coin.Direction, "LONG") {
		return false
	}
	if coin.V7ExecutionQuality != "ready" || coin.V7Status != "candidate" {
		return false
	}
	if coin.V7RiskScore >= 55 || coin.V7AIPriority < 55 || coin.V7SetupScore < 55 || coin.V7TimingScore < 45 {
		return false
	}
	if coin.V7LiquidityScore > 0 && coin.V7LiquidityScore < 50 {
		return false
	}
	if !hunterV7TakerBuyConfirmedAtLeast(coin, 0.52) {
		return false
	}
	return containsStringValue(coin.V7ReasonCodes, "strong_reclaim") &&
		containsAnyStringValue(coin.V7ReasonCodes, []string{"selling_decelerating", "selling_exhaustion"}) &&
		containsAnyStringValue(coin.V7ReasonCodes, []string{"oi_declining", "oi_flush", "oi_heavy_flush", "oi_massive_flush"})
}

func containsAnyStringValue(values []string, wants []string) bool {
	for _, want := range wants {
		if containsStringValue(values, want) {
			return true
		}
	}
	return false
}

func maxFloat64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// hunterScoresToCandidateCoins converts snapshot Hunter scores to CandidateCoin format.
func (e *StrategyEngine) hunterScoresToCandidateCoins(scores []local.HunterCoinScore, direction string) []CandidateCoin {
	var candidates []CandidateCoin
	for _, s := range scores {
		if s.Direction == "" {
			continue
		}
		cc := CandidateCoin{
			Symbol:     s.Symbol,
			Sources:    []string{"hunter"},
			Direction:  s.Direction,
			LongScore:  s.LongFinalScore,
			ShortScore: s.ShortFinalScore,
			LongTags:   s.LongTags,
			ShortTags:  s.ShortTags,
			SignalTags: s.Tags,
		}
		cc.CapitalLevel, cc.CapitalTier = local.ClassifyCapitalLevel(cc.SignalTags, cc.LongScore+cc.ShortScore)
		if direction != "" && direction != "BOTH" && cc.Direction != direction {
			continue
		}
		candidates = append(candidates, cc)
	}
	return candidates
}

// ai500CoinsToCandidateCoins converts snapshot AI500 results to CandidateCoin format.
func (e *StrategyEngine) ai500CoinsToCandidateCoins(coins []aitos.CoinData) []CandidateCoin {
	var candidates []CandidateCoin
	for _, c := range coins {
		candidates = append(candidates, CandidateCoin{
			Symbol:  c.Pair,
			Sources: []string{"ai500"},
		})
	}
	return candidates
}

// filterSniffCandidates applies the 3-condition resonance filter for hunter_sniff mode.
// For snapshot mode, this is a simplified version that filters by score thresholds.
func (e *StrategyEngine) filterSniffCandidates(candidates []CandidateCoin, snifferCfg *store.SnifferConfig) []CandidateCoin {
	if snifferCfg == nil {
		return candidates
	}
	var filtered []CandidateCoin
	for _, cc := range candidates {
		if cc.Direction == "LONG" && cc.LongScore >= snifferCfg.MinLongScore {
			cc.AmbushType = "LONG_AMBUSH"
			cc.AmbushReasons = []string{"snapshot_long_score_above_threshold"}
			filtered = append(filtered, cc)
		} else if cc.Direction == "SHORT" && cc.ShortScore >= snifferCfg.MinShortScore {
			cc.AmbushType = "SHORT_DISTRIBUTION"
			cc.AmbushReasons = []string{"snapshot_short_score_above_threshold"}
			filtered = append(filtered, cc)
		}
	}
	logger.Infof("🔍 Hunter Sniff (snapshot): %d/%d candidates passed threshold", len(filtered), len(candidates))
	return filtered
}

// ============================================================================
// Candidate Coins
// ============================================================================

// GetCandidateCoins gets candidate coins based on strategy configuration
func (e *StrategyEngine) GetCandidateCoins() ([]CandidateCoin, error) {
	var candidates []CandidateCoin
	symbolSources := make(map[string][]string)

	coinSource := e.config.CoinSource

	switch coinSource.SourceType {
	case "static":
		for _, symbol := range coinSource.StaticCoins {
			symbol = market.Normalize(symbol)
			candidates = append(candidates, CandidateCoin{
				Symbol:  symbol,
				Sources: []string{"static"},
			})
		}

		return e.filterExcludedCoins(candidates), nil

	case "ai500":
		// Check use_ai500 flag; if false, fall back to static coins
		if !coinSource.UseAI500 {
			logger.Infof("⚠️  source_type is 'ai500' but use_ai500 is false, falling back to static coins")
			for _, symbol := range coinSource.StaticCoins {
				symbol = market.Normalize(symbol)
				candidates = append(candidates, CandidateCoin{
					Symbol:  symbol,
					Sources: []string{"static"},
				})
			}
			return e.filterExcludedCoins(candidates), nil
		}
		coins, err := e.getAI500Coins(coinSource.AI500Limit)
		if err != nil {
			return nil, err
		}
		// Empty list is a normal condition, return directly
		return e.filterExcludedCoins(coins), nil

	case "oi_top":
		// Check use_oi_top flag; if false, fall back to static coins
		if !coinSource.UseOITop {
			logger.Infof("⚠️  source_type is 'oi_top' but use_oi_top is false, falling back to static coins")
			for _, symbol := range coinSource.StaticCoins {
				symbol = market.Normalize(symbol)
				candidates = append(candidates, CandidateCoin{
					Symbol:  symbol,
					Sources: []string{"static"},
				})
			}
			return e.filterExcludedCoins(candidates), nil
		}
		coins, err := e.getOITopCoins(coinSource.OITopLimit)
		if err != nil {
			return nil, err
		}
		// Empty list is a normal condition, return directly
		return e.filterExcludedCoins(coins), nil

	case "oi_low":
		// OI decrease ranking, suitable for short positions
		if !coinSource.UseOILow {
			logger.Infof("⚠️  source_type is 'oi_low' but use_oi_low is false, falling back to static coins")
			for _, symbol := range coinSource.StaticCoins {
				symbol = market.Normalize(symbol)
				candidates = append(candidates, CandidateCoin{
					Symbol:  symbol,
					Sources: []string{"static"},
				})
			}
			return e.filterExcludedCoins(candidates), nil
		}
		coins, err := e.getOILowCoins(coinSource.OILowLimit)
		if err != nil {
			return nil, err
		}
		// Empty list is a normal condition, return directly
		return e.filterExcludedCoins(coins), nil

	case "hyper_all":
		// All Hyperliquid perp coins
		if !coinSource.UseHyperAll {
			logger.Infof("⚠️  source_type is 'hyper_all' but use_hyper_all is false, falling back to static coins")
			for _, symbol := range coinSource.StaticCoins {
				symbol = market.Normalize(symbol)
				candidates = append(candidates, CandidateCoin{
					Symbol:  symbol,
					Sources: []string{"static"},
				})
			}
			return e.filterExcludedCoins(candidates), nil
		}
		coins, err := e.getHyperAllCoins()
		if err != nil {
			return nil, err
		}
		return e.filterExcludedCoins(coins), nil

	case "hyper_main":
		// Top N Hyperliquid coins by 24h volume
		if !coinSource.UseHyperMain {
			logger.Infof("⚠️  source_type is 'hyper_main' but use_hyper_main is false, falling back to static coins")
			for _, symbol := range coinSource.StaticCoins {
				symbol = market.Normalize(symbol)
				candidates = append(candidates, CandidateCoin{
					Symbol:  symbol,
					Sources: []string{"static"},
				})
			}
			return e.filterExcludedCoins(candidates), nil
		}
		coins, err := e.getHyperMainCoins(coinSource.HyperMainLimit)
		if err != nil {
			return nil, err
		}
		return e.filterExcludedCoins(coins), nil

	case "square_heat":
		// Binance Square social heat + on-chain signal scoring
		if e.squareClient == nil {
			return nil, fmt.Errorf("square_heat source not configured (squareClient is nil)")
		}
		limit := coinSource.SquareHeatLimit
		if limit <= 0 {
			limit = 10
		}
		symbols, err := e.squareClient.GetSquareHeatSymbols(limit)
		if err != nil || len(symbols) == 0 {
			reason := "0 items returned"
			if err != nil {
				reason = err.Error()
			}
			logger.Warnf("⚠ Square Heat unavailable (%s), falling back to ai500", reason)
			coins, fbErr := e.getAI500Coins(coinSource.AI500Limit)
			if fbErr != nil {
				return nil, fmt.Errorf("square_heat fallback also failed: %w", fbErr)
			}
			for i := range coins {
				coins[i].Sources = []string{"square_heat:ai500_fallback"}
			}
			return e.filterExcludedCoins(coins), nil
		}
		for _, sym := range symbols {
			normalized := market.Normalize(sym)
			candidates = append(candidates, CandidateCoin{
				Symbol:  normalized,
				Sources: []string{"square_heat"},
			})
		}
		logger.Infof("🔥 Square Heat: loaded %d candidates (limit=%d)", len(candidates), limit)
		return e.filterExcludedCoins(candidates), nil

	case "hunter":
		coins, err := e.getHunterCoins(coinSource.HunterLimit, coinSource.HunterDirection)
		if err != nil {
			return nil, err
		}
		return e.filterExcludedCoins(coins), nil

	case "hunter_sniff":
		coins, err := e.getHunterSniffCoins(coinSource.HunterLimit, coinSource.HunterSniffer)
		if err != nil {
			return nil, err
		}
		return e.filterExcludedCoins(coins), nil

	case "mixed":
		// Market environment pre-classifier: detect ranging vs trending before source assembly
		if coinSource.EnableADXPreClassifier {
			e.classifyMarketEnvironment(coinSource)
		}

		if coinSource.UseAI500 {
			poolCoins, err := e.getAI500Coins(coinSource.AI500Limit)
			if err != nil {
				logger.Infof("⚠️  Failed to get AI500 coins: %v", err)
			} else {
				for _, coin := range poolCoins {
					symbolSources[coin.Symbol] = append(symbolSources[coin.Symbol], "ai500")
				}
			}
		}

		if coinSource.UseOITop {
			oiCoins, err := e.getOITopCoins(coinSource.OITopLimit)
			if err != nil {
				logger.Infof("⚠️  Failed to get OI Top: %v", err)
			} else {
				for _, coin := range oiCoins {
					symbolSources[coin.Symbol] = append(symbolSources[coin.Symbol], "oi_top")
				}
			}
		}

		if coinSource.UseOILow {
			oiLowCoins, err := e.getOILowCoins(coinSource.OILowLimit)
			if err != nil {
				logger.Infof("⚠️  Failed to get OI Low: %v", err)
			} else {
				for _, coin := range oiLowCoins {
					symbolSources[coin.Symbol] = append(symbolSources[coin.Symbol], "oi_low")
				}
			}
		}

		if coinSource.UseHyperAll {
			hyperCoins, err := e.getHyperAllCoins()
			if err != nil {
				logger.Infof("⚠️  Failed to get Hyperliquid All coins: %v", err)
			} else {
				for _, coin := range hyperCoins {
					symbolSources[coin.Symbol] = append(symbolSources[coin.Symbol], "hyper_all")
				}
			}
		}

		if coinSource.UseHyperMain {
			hyperMainCoins, err := e.getHyperMainCoins(coinSource.HyperMainLimit)
			if err != nil {
				logger.Infof("⚠️  Failed to get Hyperliquid Main coins: %v", err)
			} else {
				for _, coin := range hyperMainCoins {
					symbolSources[coin.Symbol] = append(symbolSources[coin.Symbol], "hyper_main")
				}
			}
		}

		if coinSource.UseSquareHeat && e.squareClient != nil {
			limit := coinSource.SquareHeatLimit
			if limit <= 0 {
				limit = 10
			}
			symbols, err := e.squareClient.GetSquareHeatSymbols(limit)
			if err != nil {
				logger.Warnf("⚠ Square Heat unavailable in mixed mode: %v", err)
			} else {
				for _, sym := range symbols {
					sym = market.Normalize(sym)
					symbolSources[sym] = append(symbolSources[sym], "square_heat")
				}
				logger.Infof("🔥 Square Heat (mixed): contributed %d symbols", len(symbols))
			}
		}

		if coinSource.UseHunter {
			if limit := coinSource.HunterLimit; limit > 0 {
				if localClient, ok := e.aitosClient.(*local.Client); ok {
					symbols, err := localClient.GetHunterTopRatedCoins(limit, coinSource.Hunter)
					if err != nil {
						logger.Warnf("⚠ Hunter unavailable in mixed mode: %v", err)
					} else {
						for _, sym := range symbols {
							symbolSources[sym] = append(symbolSources[sym], "hunter")
						}
						logger.Infof("🎯 Hunter (mixed): contributed %d symbols", len(symbols))
					}
				}
			}
		}

		for _, symbol := range coinSource.StaticCoins {
			symbol = market.Normalize(symbol)
			if _, exists := symbolSources[symbol]; !exists {
				symbolSources[symbol] = []string{"static"}
			} else {
				symbolSources[symbol] = append(symbolSources[symbol], "static")
			}
		}

		for symbol, sources := range symbolSources {
			candidates = append(candidates, CandidateCoin{
				Symbol:  symbol,
				Sources: sources,
			})
		}
		return e.filterExcludedCoins(candidates), nil

	default:
		return nil, fmt.Errorf("unknown coin source type: %s", coinSource.SourceType)
	}
}

// filterExcludedCoins removes excluded coins from the candidates list
func (e *StrategyEngine) filterExcludedCoins(candidates []CandidateCoin) []CandidateCoin {
	if len(e.config.CoinSource.ExcludedCoins) == 0 {
		return candidates
	}

	// Build excluded set for O(1) lookup
	excluded := make(map[string]bool)
	for _, coin := range e.config.CoinSource.ExcludedCoins {
		normalized := market.Normalize(coin)
		excluded[normalized] = true
	}

	// Filter out excluded coins
	filtered := make([]CandidateCoin, 0, len(candidates))
	for _, c := range candidates {
		if !excluded[c.Symbol] {
			filtered = append(filtered, c)
		} else {
			logger.Infof("🚫 Excluded coin: %s", c.Symbol)
		}
	}

	return filtered
}

// classifyMarketEnvironment fetches BTC klines and classifies the current market regime
// using ADX (Average Directional Index). Called at the start of mixed mode to determine
// whether to favor mean-reversion (ranging) or trend-following (trending) sources.
func (e *StrategyEngine) classifyMarketEnvironment(coinSource store.CoinSourceConfig) {
	symbol := coinSource.ADXRegimeSymbol
	if symbol == "" {
		symbol = "BTCUSDT"
	}
	tf := coinSource.ADXRegimeTimeframe
	if tf == "" {
		tf = "4h"
	}
	bars := coinSource.ADXRegimeBars
	if bars <= 0 {
		bars = 100
	}
	period := coinSource.ADXPeriod
	if period <= 0 {
		period = 14
	}

	// Use market package's APIClient to fetch klines
	apiClient := market.NewAPIClient()
	klines, err := apiClient.GetKlines(symbol, tf, bars)
	if err != nil {
		logger.Warnf("⚠️  ADX pre-classifier: failed to fetch %s %s klines: %v", symbol, tf, err)
		return
	}
	if len(klines) < period*2+1 {
		logger.Warnf("⚠️  ADX pre-classifier: insufficient klines (%d < %d)", len(klines), period*2+1)
		return
	}

	e.marketEnv = market.ClassifyMarketRegime(klines, period, symbol)
	logger.Infof("📊 Market Environment: %s (ADX=%.1f, +DI=%.1f, -DI=%.1f)",
		e.marketEnv.Regime, e.marketEnv.ADX, e.marketEnv.PlusDI, e.marketEnv.MinusDI)
}

// ValidateDirectionWithADX checks if a coin's Hunter direction conflicts with the ADX market regime.
// Returns a confidence multiplier (0.80-1.15) and a warning string for AI prompt injection.
// Only activates when ADX > 20 (trending market) and |DI difference| > 5.
func ValidateDirectionWithADX(direction string, adx, plusDI, minusDI float64) (confidenceMultiplier float64, warning string) {
	if adx < 20 {
		return 1.0, "" // Ranging market — no direction validation
	}

	diDiff := plusDI - minusDI

	switch direction {
	case "SHORT":
		if diDiff > 5.0 {
			return 0.80, "ADX_DIRECTION_CONFLICT: SHORT against bullish trend (+DI dominant)"
		} else if diDiff < -5.0 {
			return 1.15, "" // Trend-aligned short, boost
		}
	case "LONG":
		if diDiff < -5.0 {
			return 0.80, "ADX_DIRECTION_CONFLICT: LONG against bearish trend (-DI dominant)"
		} else if diDiff > 5.0 {
			return 1.15, "" // Trend-aligned long, boost
		}
	}
	return 1.0, "" // |DI diff| ≤ 5, direction ambiguous
}

func (e *StrategyEngine) getAI500Coins(limit int) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = 30
	}

	symbols, err := e.aitosClient.GetTopRatedCoins(limit)
	if err != nil {
		return nil, err
	}

	var candidates []CandidateCoin
	for _, symbol := range symbols {
		candidates = append(candidates, CandidateCoin{
			Symbol:  symbol,
			Sources: []string{"ai500"},
		})
	}
	return candidates, nil
}

func (e *StrategyEngine) getHunterCoins(limit int, direction string) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = 10
	}
	localClient, ok := e.aitosClient.(*local.Client)
	if !ok {
		return nil, fmt.Errorf("hunter source requires local.Client, got %T", e.aitosClient)
	}
	symbols, preFetched, coinMeta, err := localClient.GetHunterCoinsWithData(limit, e.config.CoinSource.Hunter)
	if err != nil {
		return nil, err
	}
	var candidates []CandidateCoin
	for _, symbol := range symbols {
		cc := CandidateCoin{
			Symbol:         symbol,
			Sources:        []string{"hunter"},
			PreFetchedData: preFetched[symbol],
		}
		if meta, ok := coinMeta[symbol]; ok {
			cc.Direction = meta.Direction
			cc.SignalTags = meta.SignalTags
			cc.LongScore = meta.LongScore
			cc.ShortScore = meta.ShortScore
			cc.LongTags = meta.LongTags
			cc.ShortTags = meta.ShortTags
			// Classify capital confirmation level (three-level gate)
			cc.CapitalLevel, cc.CapitalTier = local.ClassifyCapitalLevel(cc.SignalTags, cc.LongScore+cc.ShortScore)
		}
		// Filter by direction if specified (LONG or SHORT)
		if direction != "" && direction != "BOTH" && cc.Direction != direction {
			continue
		}
		candidates = append(candidates, cc)
	}
	return candidates, nil
}

// getHunterSniffCoins returns coins filtered through the Hunter Sniff mode
// (institutional ambush pattern detector). It fetches raw Hunter scores,
// applies the 3-condition resonance filter, and returns only high-conviction
// LONG_AMBUSH and SHORT_DISTRIBUTION candidates.
func (e *StrategyEngine) getHunterSniffCoins(limit int, snifferCfg *store.SnifferConfig) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = 10
	}
	localClient, ok := e.aitosClient.(*local.Client)
	if !ok {
		return nil, fmt.Errorf("hunter_sniff source requires local.Client, got %T", e.aitosClient)
	}

	// Step 1: Get raw scored coins from Hunter
	allCoins, err := localClient.GetHunterList()
	if err != nil {
		return nil, fmt.Errorf("hunter_sniff: GetHunterList failed: %w", err)
	}

	// Step 2: Get coin meta (direction, scores, tags) + pre-fetched klines
	_, preFetched, coinMeta, err := localClient.GetHunterCoinsWithData(limit, e.config.CoinSource.Hunter)
	if err != nil {
		return nil, fmt.Errorf("hunter_sniff: GetHunterCoinsWithData failed: %w", err)
	}

	// Step 3: Apply sniff filter (3-condition resonance)
	snifferResult := localClient.FilterAmbushCandidates(allCoins, coinMeta)

	if len(snifferResult.LongAmbush) == 0 && len(snifferResult.ShortDist) == 0 {
		logger.Infof("🔍 Hunter Sniff: 0 ambush candidates from %d scanned (watch mode)", snifferResult.Stats.TotalScanned)
		return nil, nil
	}

	// Step 4: Merge Long Ambush + Short Distribution into CandidateCoins
	var candidates []CandidateCoin
	for _, amb := range append(snifferResult.LongAmbush, snifferResult.ShortDist...) {
		sym := market.Normalize(amb.Symbol)
		cc := CandidateCoin{
			Symbol:         sym,
			Sources:        []string{"hunter_sniff"},
			AmbushType:     string(amb.AmbushType),
			AmbushReasons:  amb.Reasons,
			PreFetchedData: preFetched[sym],
		}
		if meta, ok := coinMeta[sym]; ok {
			cc.Direction = meta.Direction
			cc.SignalTags = meta.SignalTags
			cc.LongScore = meta.LongScore
			cc.ShortScore = meta.ShortScore
			cc.LongTags = meta.LongTags
			cc.ShortTags = meta.ShortTags
		}
		candidates = append(candidates, cc)
	}

	logger.Infof("🔍 Hunter Sniff: %d candidates (LONG_AMBUSH: %d, SHORT_DIST: %d) from %d scanned",
		len(candidates), snifferResult.Stats.LongPassed, snifferResult.Stats.ShortPassed, snifferResult.Stats.TotalScanned)

	return candidates, nil
}

func (e *StrategyEngine) getOITopCoins(limit int) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = 10
	}

	positions, err := e.aitosClient.GetOITopPositions()
	if err != nil {
		return nil, err
	}

	var candidates []CandidateCoin
	for i, pos := range positions {
		if i >= limit {
			break
		}
		symbol := market.Normalize(pos.Symbol)
		candidates = append(candidates, CandidateCoin{
			Symbol:  symbol,
			Sources: []string{"oi_top"},
		})
	}
	return candidates, nil
}

func (e *StrategyEngine) getOILowCoins(limit int) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = 10
	}

	positions, err := e.aitosClient.GetOILowPositions()
	if err != nil {
		return nil, err
	}

	var candidates []CandidateCoin
	for i, pos := range positions {
		if i >= limit {
			break
		}
		symbol := market.Normalize(pos.Symbol)
		candidates = append(candidates, CandidateCoin{
			Symbol:  symbol,
			Sources: []string{"oi_low"},
		})
	}
	return candidates, nil
}

// getHyperAllCoins returns all available Hyperliquid perpetual coins
func (e *StrategyEngine) getHyperAllCoins() ([]CandidateCoin, error) {
	ctx := context.Background()
	symbols, err := hyperliquid.GetAllCoinSymbols(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get Hyperliquid coins: %w", err)
	}

	var candidates []CandidateCoin
	for _, symbol := range symbols {
		// Add USDT suffix for compatibility
		normalizedSymbol := market.Normalize(symbol + "USDT")
		candidates = append(candidates, CandidateCoin{
			Symbol:  normalizedSymbol,
			Sources: []string{"hyper_all"},
		})
	}
	logger.Infof("✅ Loaded %d Hyperliquid coins (hyper_all)", len(candidates))
	return candidates, nil
}

// getHyperMainCoins returns top N Hyperliquid coins by 24h volume
func (e *StrategyEngine) getHyperMainCoins(limit int) ([]CandidateCoin, error) {
	if limit <= 0 {
		limit = 20
	}

	ctx := context.Background()
	symbols, err := hyperliquid.GetMainCoinSymbols(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get Hyperliquid main coins: %w", err)
	}

	var candidates []CandidateCoin
	for _, symbol := range symbols {
		// Add USDT suffix for compatibility
		normalizedSymbol := market.Normalize(symbol + "USDT")
		candidates = append(candidates, CandidateCoin{
			Symbol:  normalizedSymbol,
			Sources: []string{"hyper_main"},
		})
	}
	logger.Infof("✅ Loaded %d Hyperliquid main coins (hyper_main) by 24h volume", len(candidates))
	return candidates, nil
}

// ============================================================================
// External & Quant Data
// ============================================================================

// FetchMarketData fetches market data based on strategy configuration
func (e *StrategyEngine) FetchMarketData(symbol string) (*market.Data, error) {
	return market.Get(symbol)
}

// FetchExternalData fetches external data sources
func (e *StrategyEngine) FetchExternalData() (map[string]interface{}, error) {
	externalData := make(map[string]interface{})

	for _, source := range e.config.Indicators.ExternalDataSources {
		data, err := e.fetchSingleExternalSource(source)
		if err != nil {
			logger.Infof("⚠️  Failed to fetch external data source [%s]: %v", source.Name, err)
			continue
		}
		externalData[source.Name] = data
	}

	return externalData, nil
}

func (e *StrategyEngine) fetchSingleExternalSource(source store.ExternalDataSource) (interface{}, error) {
	// SSRF Protection: Validate URL before making request
	if err := security.ValidateURL(source.URL); err != nil {
		return nil, fmt.Errorf("external source URL validation failed: %w", err)
	}

	timeout := time.Duration(source.RefreshSecs) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// Use SSRF-safe HTTP client
	client := security.SafeHTTPClient(timeout)

	req, err := http.NewRequest(source.Method, source.URL, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range source.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if source.DataPath != "" {
		result = extractJSONPath(result, source.DataPath)
	}

	return result, nil
}

func extractJSONPath(data interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	current := data

	for _, part := range parts {
		if m, ok := current.(map[string]interface{}); ok {
			current = m[part]
		} else {
			return nil
		}
	}

	return current
}

// FetchQuantData fetches quantitative data for a single coin
func (e *StrategyEngine) FetchQuantData(symbol string) (*QuantData, error) {
	if !e.config.Indicators.EnableQuantData {
		return nil, nil
	}

	// Use aitos client with unified API key
	include := "oi,price"
	if e.config.Indicators.EnableQuantNetflow {
		include = "netflow,oi,price"
	}

	aitosData, err := e.aitosClient.GetCoinData(symbol, include)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch quant data: %w", err)
	}

	if aitosData == nil {
		return nil, nil
	}

	// Convert aitos.QuantData to kernel.QuantData
	quantData := &QuantData{
		Symbol:      aitosData.Symbol,
		Price:       aitosData.Price,
		PriceChange: aitosData.PriceChange,
	}

	// Convert OI data
	if aitosData.OI != nil {
		quantData.OI = make(map[string]*OIData)
		for exchange, oiData := range aitosData.OI {
			if oiData != nil {
				kData := &OIData{
					CurrentOI: oiData.CurrentOI,
				}
				if oiData.Delta != nil {
					kData.Delta = make(map[string]*OIDeltaData)
					for dur, delta := range oiData.Delta {
						if delta != nil {
							kData.Delta[dur] = &OIDeltaData{
								OIDelta:        delta.OIDelta,
								OIDeltaValue:   delta.OIDeltaValue,
								OIDeltaPercent: delta.OIDeltaPercent,
							}
						}
					}
				}
				quantData.OI[exchange] = kData
			}
		}
	}

	// Convert Netflow data
	if aitosData.Netflow != nil {
		quantData.Netflow = &NetflowData{}
		if aitosData.Netflow.Institution != nil {
			quantData.Netflow.Institution = &FlowTypeData{
				Future: aitosData.Netflow.Institution.Future,
				Spot:   aitosData.Netflow.Institution.Spot,
			}
		}
		if aitosData.Netflow.Personal != nil {
			quantData.Netflow.Personal = &FlowTypeData{
				Future: aitosData.Netflow.Personal.Future,
				Spot:   aitosData.Netflow.Personal.Spot,
			}
		}
	}

	return quantData, nil
}

// FetchQuantDataBatch batch fetches quantitative data
func (e *StrategyEngine) FetchQuantDataBatch(symbols []string) map[string]*QuantData {
	result := make(map[string]*QuantData)

	if !e.config.Indicators.EnableQuantData {
		return result
	}

	for _, symbol := range symbols {
		data, err := e.FetchQuantData(symbol)
		if err != nil {
			logger.Infof("⚠️  Failed to fetch quantitative data for %s: %v", symbol, err)
			continue
		}
		if data != nil {
			result[symbol] = data
		}
	}

	return result
}

// FetchOIRankingData fetches market-wide OI ranking data
func (e *StrategyEngine) FetchOIRankingData() *aitos.OIRankingData {
	indicators := e.config.Indicators
	if !indicators.EnableOIRanking {
		return nil
	}

	duration := indicators.OIRankingDuration
	if duration == "" {
		duration = "1h"
	}

	limit := indicators.OIRankingLimit
	if limit <= 0 {
		limit = 10
	}

	logger.Infof("📊 Fetching OI ranking data (duration: %s, limit: %d)", duration, limit)

	data, err := e.aitosClient.GetOIRanking(duration, limit)
	if err != nil {
		logger.Warnf("⚠️  Failed to fetch OI ranking data: %v", err)
		return nil
	}

	logger.Infof("✓ OI ranking data ready: %d top, %d low positions",
		len(data.TopPositions), len(data.LowPositions))

	return data
}

// FetchNetFlowRankingData fetches market-wide NetFlow ranking data
func (e *StrategyEngine) FetchNetFlowRankingData() *aitos.NetFlowRankingData {
	indicators := e.config.Indicators
	if !indicators.EnableNetFlowRanking {
		return nil
	}

	duration := indicators.NetFlowRankingDuration
	if duration == "" {
		duration = "1h"
	}

	limit := indicators.NetFlowRankingLimit
	if limit <= 0 {
		limit = 10
	}

	logger.Infof("💰 Fetching NetFlow ranking data (duration: %s, limit: %d)", duration, limit)

	data, err := e.aitosClient.GetNetFlowRanking(duration, limit)
	if err != nil {
		logger.Warnf("⚠️  Failed to fetch NetFlow ranking data: %v", err)
		return nil
	}

	logger.Infof("✓ NetFlow ranking data ready: inst_in=%d, inst_out=%d, retail_in=%d, retail_out=%d",
		len(data.InstitutionFutureTop), len(data.InstitutionFutureLow),
		len(data.PersonalFutureTop), len(data.PersonalFutureLow))

	return data
}

// FetchPriceRankingData fetches market-wide price ranking data (gainers/losers)
func (e *StrategyEngine) FetchPriceRankingData() *aitos.PriceRankingData {
	indicators := e.config.Indicators
	if !indicators.EnablePriceRanking {
		return nil
	}

	durations := indicators.PriceRankingDuration
	if durations == "" {
		durations = "1h"
	}

	limit := indicators.PriceRankingLimit
	if limit <= 0 {
		limit = 10
	}

	logger.Infof("📈 Fetching Price ranking data (durations: %s, limit: %d)", durations, limit)

	data, err := e.aitosClient.GetPriceRanking(durations, limit)
	if err != nil {
		logger.Warnf("⚠️  Failed to fetch Price ranking data: %v", err)
		return nil
	}

	logger.Infof("✓ Price ranking data ready for %d durations", len(data.Durations))

	return data
}

// ============================================================================
// Helper Functions
// ============================================================================

// detectLanguage detects language from text content
// Returns LangChinese if text contains Chinese characters, otherwise LangEnglish
func detectLanguage(text string) Language {
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			return LangChinese
		}
	}
	return LangEnglish
}

func hunterV7AltLadderLateLongNeedsFreshFlow(coin CandidateCoin) bool {
	if coin.V7SetupType != "alt_ladder_momentum_long" || !strings.EqualFold(coin.Direction, "LONG") {
		return false
	}
	late := containsStringValue(coin.V7ReasonCodes, "alt_ladder_stage_late") ||
		containsStringValue(coin.V7RiskTags, "alt_ladder_late_chase_risk") ||
		containsStringValue(coin.V7RiskTags, "high_volatility")
	if !late {
		return false
	}
	if coin.V7DerivativesCtx == nil {
		return true
	}
	freshOI := coin.V7DerivativesCtx.OIChange1h > 0.5
	freshParticipation := containsStringValue(coin.V7ReasonCodes, "alt_ladder_volume_expansion") ||
		hunterV7TakerBuyConfirmedAtLeast(coin, 0.60)
	return !(freshOI && freshParticipation)
}

func hunterV7AltLadderLongExecutable(coin CandidateCoin) bool {
	// Same-direction OI participation is missing on both 1h and 4h; taker flow
	// alone is not enough to make this executable.
	if containsStringValue(coin.V7RiskTags, "fresh_oi_absent") {
		return false
	}
	if coin.V7AIPriority < 58 ||
		coin.V7SetupScore < 58 ||
		coin.V7TimingScore < 60 ||
		coin.V7RiskScore >= 55 ||
		!hunterV7PriceInsideEntryZone(coin) {
		return false
	}
	if !hunterV7TakerBuyConfirmedAtLeast(coin, 0.55) {
		return false
	}
	if !containsStringValue(coin.V7ReasonCodes, "alt_ladder_taker_buy") {
		return false
	}
	if !containsAnyStringValue(coin.V7ReasonCodes, []string{"alt_ladder_oi_inflow", "alt_ladder_volume_expansion"}) {
		return false
	}
	if containsAnyStringValue(coin.V7RiskTags, []string{"alt_ladder_late_chase_risk", "high_volatility"}) &&
		!containsStringValue(coin.V7ReasonCodes, "alt_ladder_oi_inflow") {
		return false
	}
	if coin.V7DerivativesCtx != nil && coin.V7DerivativesCtx.OIChange4h < -3 &&
		!containsStringValue(coin.V7ReasonCodes, "alt_ladder_oi_inflow") {
		return false
	}
	if containsStringValue(coin.V7ReasonCodes, "alt_ladder_stage_mid") &&
		!containsStringValue(coin.V7ReasonCodes, "alt_ladder_taker_buy") {
		return false
	}
	if hunterV7AltLadderLateLongNeedsFreshFlow(coin) {
		return false
	}
	if containsStringValue(coin.V7RiskTags, "execution_stop_tightened") {
		if !hunterV7TakerBuyConfirmedAtLeast(coin, 0.58) {
			return false
		}
		if !containsStringValue(coin.V7ReasonCodes, "alt_ladder_oi_inflow") &&
			(coin.V7DerivativesCtx == nil || coin.V7DerivativesCtx.OIChange1h < 0) {
			return false
		}
	}
	return true
}

func hunterV7BreakoutTriggerNearFlowReviewable(coin CandidateCoin) bool {
	if coin.V7EntrySignal != "entry_trigger_near" {
		return false
	}
	if coin.V7SetupType != "trend_breakout_long" && coin.V7SetupType != "accumulation_breakout_long" {
		return false
	}
	if coin.V7SetupScore < 74 || coin.V7AIPriority < 45 || coin.V7RiskScore >= 35 {
		return false
	}
	if coin.V7LiquidityScore > 0 && coin.V7LiquidityScore < 70 {
		return false
	}
	if !hunterV7TakerBuyAtLeast(coin, 0.58) {
		return false
	}
	if !containsStringValue(coin.V7ReasonCodes, "clear_air_above") {
		return false
	}
	if coin.V7DerivativesCtx != nil && coin.V7DerivativesCtx.OIChange1h < 0 {
		return false
	}
	rr, ok := hunterV7FinalConfirmationRR(coin)
	return ok && rr >= hunterV7BackendMinRR
}

func hunterV7DisplacementReviewOnlyRisk(coin CandidateCoin) bool {
	if coin.V7SetupType != "displacement_momentum_long" {
		return false
	}
	return containsAnyStringValue(coin.V7RiskTags, []string{
		"displacement_rr_insufficient",
		"displacement_rr_repaired_needs_review",
		"displacement_chase_risk_overextended",
		"displacement_chase_risk_extreme_1h_move",
	})
}

// hunterV7LeaderMomentumUpperChaseWait consumes the upper-zone chase verdict
// the provider already voted on at signal time (leaderMomentumUpperZoneChaseRisk,
// 5 weak-signal votes over RSI/taker, OI, volume burst, VWAP distance and BB
// proximity). The kernel used to re-run a drifted 3-vote copy of that formula
// when the tags were absent; the provider verdict is now the single authority
// (U3.5).
func hunterV7LeaderMomentumUpperChaseWait(coin CandidateCoin) bool {
	if coin.V7SetupType != "leader_momentum_long" || !strings.EqualFold(coin.Direction, "LONG") {
		return false
	}
	return containsAnyStringValue(coin.V7RiskTags, []string{"momentum_upper_zone_chase", "do_not_market_chase"}) ||
		containsStringValue(coin.V7ReasonCodes, "leader_momentum_upper_chase_wait")
}

func hunterV7MMSLongExecutableChaseBlock(coin CandidateCoin) bool {
	if coin.V7SetupType != "mms_trend_ride_long" && coin.V7SetupType != "mms_squeeze_engine_long" {
		return false
	}
	if containsStringValue(coin.V7RiskTags, "mms_weak_continuation_review_only") {
		return true
	}
	if !strings.EqualFold(coin.Direction, "LONG") || coin.V7PriceContext == nil {
		return false
	}
	if coin.V7PriceContext.Change24h < 18 || coin.V7PriceContext.VWAP15m <= 0 || coin.V7PriceContext.Last <= 0 {
		return false
	}
	vwapDistancePct := (coin.V7PriceContext.Last - coin.V7PriceContext.VWAP15m) / coin.V7PriceContext.VWAP15m * 100
	if vwapDistancePct < 4 {
		return false
	}
	return coin.V7DerivativesCtx == nil || coin.V7DerivativesCtx.OIChange4h < 0
}

func hunterV7MMSLongExecutableFreshEnough(coin CandidateCoin) bool {
	if coin.V7SetupType != "mms_trend_ride_long" || !strings.EqualFold(coin.Direction, "LONG") {
		return true
	}
	if !hunterV7PriceInsideEntryZone(coin) {
		return false
	}
	if coin.V7PriceContext != nil && coin.V7PriceContext.Change1h <= 0 && coin.V7PriceContext.Change4h <= 0 {
		return false
	}
	if coin.V7DerivativesCtx != nil &&
		coin.V7DerivativesCtx.OIChange1h < 0 &&
		coin.V7DerivativesCtx.OIChange4h <= 0 &&
		!containsStringValue(coin.V7ReasonCodes, "mms_trend_continuation") {
		return false
	}
	return true
}

func hunterV7PriceInsideEntryZone(coin CandidateCoin) bool {
	if coin.V7PriceContext == nil || coin.V7PriceContext.Last <= 0 ||
		coin.V7EntryZone.Lower <= 0 || coin.V7EntryZone.Upper <= 0 {
		return true
	}
	lower, upper := coin.V7EntryZone.Lower, coin.V7EntryZone.Upper
	if lower > upper {
		lower, upper = upper, lower
	}
	return coin.V7PriceContext.Last >= lower && coin.V7PriceContext.Last <= upper
}

func hunterV7TrendBreakoutStrongFlowReviewable(coin CandidateCoin) bool {
	if coin.V7SetupType != "trend_breakout_long" && coin.V7SetupType != "accumulation_breakout_long" {
		return false
	}
	if coin.V7AIPriority < 35 ||
		coin.V7SetupScore < 48 ||
		coin.V7RiskScore >= 35 ||
		(coin.V7LiquidityScore > 0 && coin.V7LiquidityScore < 30) {
		return false
	}
	if !containsStringValue(coin.V7ReasonCodes, "clear_air_above") {
		return false
	}
	if !containsAnyStringValue(coin.V7ReasonCodes, []string{"approaching_breakout", "confirmed_breakout", "strong_breakout", "breakout_attempt"}) {
		return false
	}
	flowOK := containsAnyStringValue(coin.V7ReasonCodes, []string{"taker_aggressive_buy", "taker_strong_buy", "taker_moderate_buy"})
	contextOK := coin.V7RiskScore < 16 &&
		containsAnyStringValue(coin.V7ReasonCodes, []string{"oi_increasing", "oi_stable_breakout"}) &&
		containsAnyStringValue(coin.V7ReasonCodes, []string{"volume_expansion", "volume_adequate", "volume_decent"})
	structureOnlyOK := coin.V7RiskScore < 25 &&
		!containsAnyStringValue(coin.V7ReasonCodes, []string{"some_resistance_overhead", "low_liquidity"})
	if !flowOK && !contextOK && !structureOnlyOK {
		return false
	}
	if coin.V7DerivativesCtx != nil && coin.V7DerivativesCtx.TakerBuy15m > 0 && coin.V7DerivativesCtx.TakerBuy15m < 0.52 {
		return false
	}
	if coin.V7DerivativesCtx != nil && coin.V7DerivativesCtx.OIChange1h < -0.5 &&
		!containsAnyStringValue(coin.V7ReasonCodes, []string{"volume_expansion", "volume_adequate"}) {
		return false
	}
	return true
}
