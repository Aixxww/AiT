package kernel

import (
	"encoding/json"

	local "github.com/Aixxww/AiT/provider/local"
)

// V7PromptPayload is the single serialization surface for a Hunter v7 signal
// on its way into the LLM prompt (U4.1). The full JSON, the compact JSON and
// the execution-compact text line are all views over this one struct, so a
// field added here reaches every encoding without touching three encoders.
// Field order and JSON tags are frozen: they define the prompt byte layout.
type V7PromptPayload struct {
	SignalID              string                        `json:"signal_id,omitempty"`
	Symbol                string                        `json:"symbol"`
	Direction             string                        `json:"direction"`
	SetupType             string                        `json:"setup_type"`
	ExecutionTier         string                        `json:"execution_tier,omitempty"`
	TierReason            string                        `json:"tier_reason,omitempty"`
	Status                string                        `json:"status"`
	MarketRegime          string                        `json:"market_regime"`
	EntryMode             string                        `json:"entry_mode"`
	MarketShape           string                        `json:"market_shape,omitempty"`
	EntrySignal           string                        `json:"entry_signal,omitempty"`
	ExecutionQuality      string                        `json:"execution_quality,omitempty"`
	ExecutionPolicy       string                        `json:"execution_policy,omitempty"`
	DoNotOpenUntilConfirm bool                          `json:"do_not_open_until_confirmed,omitempty"`
	Confidence            string                        `json:"confidence"`
	RiskLevel             string                        `json:"risk_level"`
	AIPriority            float64                       `json:"ai_priority"`
	SetupScore            float64                       `json:"setup_score"`
	TimingScore           float64                       `json:"timing_score"`
	RegimeFitScore        float64                       `json:"regime_fit_score"`
	LiquidityScore        float64                       `json:"liquidity_score"`
	RiskScore             float64                       `json:"risk_score"`
	ReasonCodes           []string                      `json:"reason_codes"`
	RiskTags              []string                      `json:"risk_tags"`
	RequiredConfirmations []string                      `json:"required_confirmations"`
	ConfirmationSummary   *local.V7ConfirmationSummary  `json:"confirmation_summary,omitempty"`
	ExecutionGeometry     *hunterV7ExecutionGeometry    `json:"execution_geometry,omitempty"`
	TP0DistancePct        float64                       `json:"tp0_distance_pct,omitempty"`
	MoveStopToBreakeven   bool                          `json:"move_stop_to_breakeven,omitempty"`
	PositionSizeHint      string                        `json:"position_size_hint,omitempty"`
	SuggestedTrigger      *DecisionTrigger              `json:"suggested_trigger,omitempty"`
	TagSemantics          []local.HunterV7TagDefinition `json:"tag_semantics,omitempty"`
	EntryZone             local.V7PriceZone             `json:"entry_zone"`
	Invalidation          local.V7InvalidationRule      `json:"invalidation"`
	Targets               []local.V7Target              `json:"targets"`
	TP0Price              float64                       `json:"tp0_price,omitempty"`
	TP0RR                 float64                       `json:"tp0_rr,omitempty"`
	TP0TimeWindow         string                        `json:"tp0_time_window,omitempty"`
	TP0Method             string                        `json:"tp0_method,omitempty"`
	TP1Price              float64                       `json:"tp1_price,omitempty"`
	TP1RR                 float64                       `json:"tp1_rr,omitempty"`
	TP1TimeWindow         string                        `json:"tp1_time_window,omitempty"`
	TP1Method             string                        `json:"tp1_method,omitempty"`
	TP2Price              float64                       `json:"tp2_price,omitempty"`
	TP2RR                 float64                       `json:"tp2_rr,omitempty"`
	TP2TimeWindow         string                        `json:"tp2_time_window,omitempty"`
	TP2Method             string                        `json:"tp2_method,omitempty"`
	TakeProfitPlan        *local.V7TakeProfitPlan       `json:"take_profit_plan,omitempty"`
	PriceContext          *local.V7PriceContext         `json:"price_context,omitempty"`
	DerivativesContext    *local.V7DerivativesContext   `json:"derivatives_context,omitempty"`
	DataFreshness         local.V7DataFreshness         `json:"data_freshness,omitempty"`
	ExecutionReadiness    *local.V7ExecutionReadiness   `json:"execution_readiness,omitempty"`
	ExecutionContext      *local.V7ExecutionContext     `json:"execution_context,omitempty"`
}

// buildHunterV7PromptPayload assembles the payload from the candidate's
// cached verdict and derived execution fields.
//
// Reason codes and risk tags are rendered in the canonical flow_ vocabulary
// (U6.3 alias period): the candidate's internal state keeps the historical
// spellings — tier rules and trader guards still match those — while the LLM
// sees one unified taker ladder with tag_semantics explaining both names.
func buildHunterV7PromptPayload(coin CandidateCoin) V7PromptPayload {
	reasonCodes := local.V7CanonicalizeTags(coin.V7ReasonCodes)
	riskTags := local.V7CanonicalizeTags(coin.V7RiskTags)
	return V7PromptPayload{
		SignalID:              coin.V7SignalID,
		Symbol:                coin.Symbol,
		Direction:             coin.Direction,
		SetupType:             coin.V7SetupType,
		ExecutionTier:         coin.V7ExecutionTier,
		TierReason:            coin.V7TierReason,
		Status:                coin.V7Status,
		MarketRegime:          coin.V7MarketRegime,
		EntryMode:             coin.V7EntryMode,
		MarketShape:           coin.V7MarketShape,
		EntrySignal:           coin.V7EntrySignal,
		ExecutionQuality:      coin.V7ExecutionQuality,
		ExecutionPolicy:       hunterV7ExecutionPolicy(coin),
		DoNotOpenUntilConfirm: hunterV7DoNotOpenUntilConfirmed(coin),
		Confidence:            coin.V7Confidence,
		RiskLevel:             coin.V7RiskLevel,
		AIPriority:            coin.V7AIPriority,
		SetupScore:            coin.V7SetupScore,
		TimingScore:           coin.V7TimingScore,
		RegimeFitScore:        coin.V7RegimeFitScore,
		LiquidityScore:        coin.V7LiquidityScore,
		RiskScore:             coin.V7RiskScore,
		ReasonCodes:           reasonCodes,
		RiskTags:              riskTags,
		RequiredConfirmations: coin.V7RequiredConfirms,
		ConfirmationSummary:   coin.V7ConfirmSummary,
		ExecutionGeometry:     buildHunterV7ExecutionGeometry(coin),
		TP0DistancePct:        hunterV7EffectiveTP0DistancePct(coin),
		MoveStopToBreakeven:   hunterV7EffectiveMoveStopToBreakeven(coin),
		PositionSizeHint:      hunterV7PositionSizeHint(coin),
		SuggestedTrigger:      buildHunterV7SuggestedTrigger(coin),
		TagSemantics:          local.DescribeHunterV7Tags(reasonCodes, riskTags, coin.V7RequiredConfirms),
		EntryZone:             coin.V7EntryZone,
		Invalidation:          coin.V7Invalidation,
		Targets:               coin.V7Targets,
		TP0Price:              hunterV7EffectiveTP0Price(coin),
		TP0RR:                 coin.V7TP0RR,
		TP0TimeWindow:         coin.V7TP0TimeWindow,
		TP0Method:             coin.V7TP0Method,
		TP1Price:              coin.V7TP1Price,
		TP1RR:                 coin.V7TP1RR,
		TP1TimeWindow:         coin.V7TP1TimeWindow,
		TP1Method:             coin.V7TP1Method,
		TP2Price:              coin.V7TP2Price,
		TP2RR:                 coin.V7TP2RR,
		TP2TimeWindow:         coin.V7TP2TimeWindow,
		TP2Method:             coin.V7TP2Method,
		TakeProfitPlan:        coin.V7TPPlan,
		PriceContext:          coin.V7PriceContext,
		DerivativesContext:    coin.V7DerivativesCtx,
		DataFreshness:         coin.V7DataFreshness,
		ExecutionReadiness:    coin.V7Readiness,
		ExecutionContext:      coin.V7ExecutionContext,
	}
}

// hunterV7CompactPromptView is the compact-JSON mask over V7PromptPayload
// (U4.2): the subset of payload fields the collapsed candidate list carries.
// Tags mirror the payload's so both encodings stay aligned by construction.
type hunterV7CompactPromptView struct {
	Symbol             string                      `json:"symbol"`
	Direction          string                      `json:"direction"`
	SetupType          string                      `json:"setup_type"`
	ExecutionTier      string                      `json:"execution_tier"`
	TierReason         string                      `json:"tier_reason"`
	AIPriority         float64                     `json:"ai_priority"`
	ExecutionQuality   string                      `json:"execution_quality,omitempty"`
	RiskScore          float64                     `json:"risk_score"`
	EntryZone          local.V7PriceZone           `json:"entry_zone"`
	Invalidation       local.V7InvalidationRule    `json:"invalidation"`
	Targets            []local.V7Target            `json:"targets"`
	PriceContext       *local.V7PriceContext       `json:"price_context,omitempty"`
	ExecutionReadiness *local.V7ExecutionReadiness `json:"execution_readiness,omitempty"`
	ExecutionContext   *local.V7ExecutionContext   `json:"execution_context,omitempty"`
}

func (p V7PromptPayload) compactView() hunterV7CompactPromptView {
	return hunterV7CompactPromptView{
		Symbol:             p.Symbol,
		Direction:          p.Direction,
		SetupType:          p.SetupType,
		ExecutionTier:      p.ExecutionTier,
		TierReason:         p.TierReason,
		AIPriority:         p.AIPriority,
		ExecutionQuality:   p.ExecutionQuality,
		RiskScore:          p.RiskScore,
		EntryZone:          p.EntryZone,
		Invalidation:       p.Invalidation,
		Targets:            p.Targets,
		PriceContext:       p.PriceContext,
		ExecutionReadiness: p.ExecutionReadiness,
		ExecutionContext:   p.ExecutionContext,
	}
}

func marshalHunterV7JSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}
