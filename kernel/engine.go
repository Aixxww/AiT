package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/logger"
	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/aitos"
	"github.com/Aixxww/AiT/provider/hyperliquid"
	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/provider/square"
	"github.com/Aixxww/AiT/security"
	"github.com/Aixxww/AiT/store"
	"io"
	"net/http"
	"strings"
	"time"
)

var snapshotReadyTimeout = 30 * time.Second

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
	V7SetupType        string                      `json:"-"`
	V7Status           string                      `json:"-"`
	V7AIPriority       float64                     `json:"-"`
	V7SetupScore       float64                     `json:"-"`
	V7RiskScore        float64                     `json:"-"`
	V7LiquidityScore   float64                     `json:"-"`
	V7TimingScore      float64                     `json:"-"`
	V7RegimeFitScore   float64                     `json:"-"`
	V7RiskLevel        string                      `json:"-"`
	V7EntryMode        string                      `json:"-"`
	V7MarketRegime     string                      `json:"-"`
	V7Confidence       string                      `json:"-"`
	V7ReasonCodes      []string                    `json:"-"`
	V7RiskTags         []string                    `json:"-"`
	V7RequiredConfirms []string                    `json:"-"`
	V7EntryZone        local.V7PriceZone           `json:"-"`
	V7Invalidation     local.V7InvalidationRule    `json:"-"`
	V7Targets          []local.V7Target            `json:"-"`
	V7PriceContext     *local.V7PriceContext       `json:"-"`
	V7DerivativesCtx   *local.V7DerivativesContext `json:"-"`

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
	Confidence int     `json:"confidence,omitempty"` // Confidence level (0-100)
	RiskUSD    float64 `json:"risk_usd,omitempty"`   // Maximum USD risk
	Reasoning  string  `json:"reasoning"`
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
	config         *store.StrategyConfig
	aitosClient    aitos.DataProvider
	squareClient   *square.Client            // nil when square_heat not configured
	marketEnv      *market.MarketEnvironment // cached market regime classification (ADX-based)
	snapshotEngine *SnapshotEngine           // nil = use legacy coin sources
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
		if coinSource.Hunter != nil && coinSource.Hunter.V7MinAIPriority > 0 {
			v7cfg.MinAIPriority = coinSource.Hunter.V7MinAIPriority
		}
		if coinSource.Hunter != nil && coinSource.Hunter.V7Aggressive {
			v7cfg.Aggressive = true
		}
		if coinSource.HunterLimit > 0 && (v7cfg.MaxOutput <= 0 || coinSource.HunterLimit < v7cfg.MaxOutput) {
			v7cfg.MaxOutput = coinSource.HunterLimit
		}
		signals := local.ScoreHunterV7(snap, v7cfg)
		candidates = e.hunterV7SignalsToCandidateCoins(signals, coinSource.HunterDirection)

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

		cc := CandidateCoin{
			Symbol:     sig.Symbol,
			Sources:    []string{"hunter_v7"},
			Direction:  string(sig.Direction),
			SignalTags: tags,
			LongTags:   nil,
			ShortTags:  nil,

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
			V7MarketRegime:     string(sig.MarketRegime),
			V7Confidence:       sig.Confidence,
			V7ReasonCodes:      append([]string{}, sig.ReasonCodes...),
			V7RiskTags:         append([]string{}, sig.RiskTags...),
			V7RequiredConfirms: append([]string{}, sig.RequiredConfirms...),
			V7EntryZone:        sig.EntryZone,
			V7Invalidation:     sig.Invalidation,
			V7Targets:          append([]local.V7Target{}, sig.Targets...),
			V7PriceContext:     sig.PriceCtx,
			V7DerivativesCtx:   sig.DerivativesCtx,
		}

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
