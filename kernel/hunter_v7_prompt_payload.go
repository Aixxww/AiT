package kernel

import (
	"encoding/json"
	"strings"

	local "github.com/Aixxww/AiT/provider/local"
)

// V7PromptPayload is the single serialization surface for a Hunter v7 signal
// on its way into the LLM prompt (U4.1). The full JSON, the compact JSON and
// the execution-compact text line are all views over this one struct, so a
// field added here reaches every encoding without touching three encoders.
// Field order and JSON tags are frozen: they define the prompt byte layout.
type V7PromptPayload struct {
	SignalID              string                       `json:"signal_id,omitempty"`
	Symbol                string                       `json:"symbol"`
	Direction             string                       `json:"direction"`
	SetupType             string                       `json:"setup_type"`
	ExecutionTier         string                       `json:"execution_tier,omitempty"`
	TierReason            string                       `json:"tier_reason,omitempty"`
	Status                string                       `json:"status"`
	MarketRegime          string                       `json:"market_regime"`
	EntryMode             string                       `json:"entry_mode"`
	MarketShape           string                       `json:"market_shape,omitempty"`
	EntrySignal           string                       `json:"entry_signal,omitempty"`
	ExecutionQuality      string                       `json:"execution_quality,omitempty"`
	ExecutionPolicy       string                       `json:"execution_policy,omitempty"`
	DoNotOpenUntilConfirm bool                         `json:"do_not_open_until_confirmed,omitempty"`
	Confidence            string                       `json:"confidence"`
	RiskLevel             string                       `json:"risk_level"`
	AIPriority            float64                      `json:"ai_priority"`
	SetupScore            float64                      `json:"setup_score"`
	TimingScore           float64                      `json:"timing_score"`
	RegimeFitScore        float64                      `json:"regime_fit_score"`
	LiquidityScore        float64                      `json:"liquidity_score"`
	RiskScore             float64                      `json:"risk_score"`
	ReasonCodes           []string                     `json:"reason_codes"`
	RiskTags              []string                     `json:"risk_tags"`
	RequiredConfirmations []string                     `json:"required_confirmations"`
	ConfirmationSummary   *local.V7ConfirmationSummary `json:"confirmation_summary,omitempty"`
	ExecutionGeometry     *hunterV7ExecutionGeometry   `json:"execution_geometry,omitempty"`
	TP0DistancePct        float64                      `json:"tp0_distance_pct,omitempty"`
	MoveStopToBreakeven   bool                         `json:"move_stop_to_breakeven,omitempty"`
	PositionSizeHint      string                       `json:"position_size_hint,omitempty"`
	// ExecutionContract is the machine-derived permission envelope.  It keeps
	// entry and protection authority with the backend rather than asking the
	// LLM to reconstruct it from prose examples in the doctrine.
	ExecutionContract  hunterV7ExecutionContract     `json:"execution_contract"`
	SuggestedTrigger   *DecisionTrigger              `json:"suggested_trigger,omitempty"`
	TagSemantics       []local.HunterV7TagDefinition `json:"tag_semantics,omitempty"`
	EntryZone          local.V7PriceZone             `json:"entry_zone"`
	Invalidation       local.V7InvalidationRule      `json:"invalidation"`
	Targets            []local.V7Target              `json:"targets"`
	TP0Price           float64                       `json:"tp0_price,omitempty"`
	TP0RR              float64                       `json:"tp0_rr,omitempty"`
	TP0TimeWindow      string                        `json:"tp0_time_window,omitempty"`
	TP0Method          string                        `json:"tp0_method,omitempty"`
	TP1Price           float64                       `json:"tp1_price,omitempty"`
	TP1RR              float64                       `json:"tp1_rr,omitempty"`
	TP1TimeWindow      string                        `json:"tp1_time_window,omitempty"`
	TP1Method          string                        `json:"tp1_method,omitempty"`
	TP2Price           float64                       `json:"tp2_price,omitempty"`
	TP2RR              float64                       `json:"tp2_rr,omitempty"`
	TP2TimeWindow      string                        `json:"tp2_time_window,omitempty"`
	TP2Method          string                        `json:"tp2_method,omitempty"`
	TakeProfitPlan     *local.V7TakeProfitPlan       `json:"take_profit_plan,omitempty"`
	PriceContext       *local.V7PriceContext         `json:"price_context,omitempty"`
	DerivativesContext *local.V7DerivativesContext   `json:"derivatives_context,omitempty"`
	DataFreshness      local.V7DataFreshness         `json:"data_freshness,omitempty"`
	ExecutionReadiness *local.V7ExecutionReadiness   `json:"execution_readiness,omitempty"`
	ExecutionContext   *local.V7ExecutionContext     `json:"execution_context,omitempty"`
}

// buildHunterV7PromptPayload assembles the payload from the candidate's
// cached verdict and derived execution fields. Since the U6.3 emission flip,
// modules emit the canonical flow_ taker vocabulary directly, so the payload
// carries the candidate's tags verbatim.
func buildHunterV7PromptPayload(coin CandidateCoin) V7PromptPayload {
	reasonCodes := coin.V7ReasonCodes
	riskTags := coin.V7RiskTags
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
		ExecutionContract:     buildHunterV7ExecutionContract(coin),
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

// hunterV7ExecutionContract is intentionally descriptive, not a second
// decision engine.  The router/tier gate remains the sole authority.  This
// compact contract gives the LLM a stable answer to the five operational
// questions it must not infer from a setup's narrative: may it enter, what
// must be confirmed, what blocks it, how should size be constrained, and how
// is the position protected after entry.
type hunterV7ExecutionContract struct {
	EntryPermission string   `json:"entry_permission"`
	MustConfirm     []string `json:"must_confirm,omitempty"`
	HardBlocks      []string `json:"hard_blocks,omitempty"`
	SizePolicy      string   `json:"size_policy"`
	ExitPolicy      string   `json:"exit_policy"`
	// MinRR is this candidate's RR admission floor when it differs from the
	// strategy floor (Task-2 candidates, U2/P2a). Omitted for native.
	MinRR float64 `json:"min_rr,omitempty"`
	// MinStopPct is this candidate's minimum stop distance (percent) when it
	// differs from the strategy floor (Task-2 candidates, U4/P2b). Omitted
	// for native.
	MinStopPct float64 `json:"min_stop_pct,omitempty"`
	// MaxTPPct is this candidate's TP1 distance cap (percent) when its class
	// profile differs from the strategy cap (U6). Omitted otherwise.
	MaxTPPct float64 `json:"max_tp_pct,omitempty"`
	// U9: drift contract when the class profile differs from the strategy,
	// and the trigger of a conditional entry. Omitted otherwise.
	MaxDriftPct    float64 `json:"max_drift_pct,omitempty"`
	DriftReference string  `json:"drift_reference,omitempty"`
	TriggerPrice   float64 `json:"trigger_price,omitempty"`
}

func buildHunterV7ExecutionContract(coin CandidateCoin) hunterV7ExecutionContract {
	contract := hunterV7ExecutionContract{
		MustConfirm: append([]string{}, coin.V7RequiredConfirms...),
		ExitPolicy:  "backend_stop_and_target_only",
		MinRR:       coin.V7RequiredRR,
		MinStopPct:  coin.V7RequiredMinStop,
		MaxTPPct:    coin.V7RequiredMaxTP,

		MaxDriftPct:    coin.V7RequiredMaxDrift,
		DriftReference: coin.V7DriftReference,
	}

	switch {
	case coin.V7ConditionalEntry:
		contract.EntryPermission = "conditional_on_trigger"
		contract.TriggerPrice = coin.V7TriggerPrice
	case coin.V7ExecutionTier == "EXECUTABLE":
		contract.EntryPermission = "backend_checks_required"
	case coin.V7ExecutionTier == "REVIEWABLE":
		contract.EntryPermission = "live_confirmation_required"
	default:
		contract.EntryPermission = "no_direct_open"
	}

	for _, tag := range append(append([]string{}, coin.V7ReasonCodes...), coin.V7RiskTags...) {
		action, ok := local.HunterV7TagLLMAction(tag)
		if !ok || (action != local.V7TagActionWaitOnly && action != local.V7TagActionRejectOnly) {
			continue
		}
		contract.HardBlocks = appendUniqueHunterV7ContractValue(contract.HardBlocks, tag)
	}
	if coin.V7ExecutionTier == "WATCH" || coin.V7ExecutionTier == "REJECTED" {
		contract.HardBlocks = appendUniqueHunterV7ContractValue(contract.HardBlocks, "execution_tier_"+strings.ToLower(coin.V7ExecutionTier))
	}

	contract.SizePolicy = hunterV7PositionSizeHint(coin)
	if contract.SizePolicy == "" {
		contract.SizePolicy = "normal_only_after_backend_checks"
		for _, tag := range coin.V7RiskTags {
			action, ok := local.HunterV7TagLLMAction(tag)
			if ok && (action == local.V7TagActionReduceOrWait || action == local.V7TagActionReviewableOnly) {
				contract.SizePolicy = "conservative_only_after_live_checks"
				break
			}
		}
	}
	if coin.V7ExecutionTier == "WATCH" || coin.V7ExecutionTier == "REJECTED" {
		contract.SizePolicy = "no_position"
	}

	if hunterV7EffectiveTP0Price(coin) > 0 && hunterV7EffectiveMoveStopToBreakeven(coin) {
		contract.ExitPolicy = "partial_tp0_then_breakeven_runner_to_tp1_tp2"
	}
	if coin.V7SetupType == "whale_flow_reversal" {
		contract.ExitPolicy = "mfe_fast_protect_then_partial_tp0_then_breakeven_runner_to_tp1_tp2"
	}
	return contract
}

func appendUniqueHunterV7ContractValue(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// hunterV7CompactPromptView is the compact-JSON mask over V7PromptPayload
// (U4.2): the subset of payload fields the collapsed candidate list carries.
// Tags mirror the payload's so both encodings stay aligned by construction.
//
// The collapsed list holds EXECUTABLE/REVIEWABLE candidates beyond the
// open-review expansion limit, which the LLM may still open (#138, R1-#4).
// The view therefore carries every decision-relevant pre-computed field the
// doctrine tells the LLM to read before an open: the scores (the weakest-
// score rule), reason_codes / risk_tags (the counter-trend boundary),
// required_confirmations and confirmation_summary (REVIEWABLE opens need
// every item visible), execution_geometry, the TP levels, the derivatives
// context and the data freshness. Deliberately dropped, to stay compact:
// tag_semantics (the tag policy block above the list defines the actions),
// the TP time windows and methods, take_profit_plan, suggested_trigger and
// position_size_hint (execution_contract.size_policy carries the sizing).
type hunterV7CompactPromptView struct {
	Symbol                string                       `json:"symbol"`
	Direction             string                       `json:"direction"`
	SetupType             string                       `json:"setup_type"`
	ExecutionTier         string                       `json:"execution_tier"`
	TierReason            string                       `json:"tier_reason"`
	AIPriority            float64                      `json:"ai_priority"`
	ExecutionQuality      string                       `json:"execution_quality,omitempty"`
	SetupScore            float64                      `json:"setup_score"`
	TimingScore           float64                      `json:"timing_score"`
	RegimeFitScore        float64                      `json:"regime_fit_score"`
	LiquidityScore        float64                      `json:"liquidity_score"`
	RiskScore             float64                      `json:"risk_score"`
	ReasonCodes           []string                     `json:"reason_codes"`
	RiskTags              []string                     `json:"risk_tags"`
	RequiredConfirmations []string                     `json:"required_confirmations"`
	ConfirmationSummary   *local.V7ConfirmationSummary `json:"confirmation_summary,omitempty"`
	ExecutionGeometry     *hunterV7ExecutionGeometry   `json:"execution_geometry,omitempty"`
	ExecutionContract     hunterV7ExecutionContract    `json:"execution_contract"`
	EntryZone             local.V7PriceZone            `json:"entry_zone"`
	Invalidation          local.V7InvalidationRule     `json:"invalidation"`
	Targets               []local.V7Target             `json:"targets"`
	TP0Price              float64                      `json:"tp0_price,omitempty"`
	TP1Price              float64                      `json:"tp1_price,omitempty"`
	TP2Price              float64                      `json:"tp2_price,omitempty"`
	PriceContext          *local.V7PriceContext        `json:"price_context,omitempty"`
	DerivativesContext    *local.V7DerivativesContext  `json:"derivatives_context,omitempty"`
	DataFreshness         local.V7DataFreshness        `json:"data_freshness,omitempty"`
	ExecutionReadiness    *local.V7ExecutionReadiness  `json:"execution_readiness,omitempty"`
	ExecutionContext      *local.V7ExecutionContext    `json:"execution_context,omitempty"`
}

func (p V7PromptPayload) compactView() hunterV7CompactPromptView {
	return hunterV7CompactPromptView{
		Symbol:                p.Symbol,
		Direction:             p.Direction,
		SetupType:             p.SetupType,
		ExecutionTier:         p.ExecutionTier,
		TierReason:            p.TierReason,
		AIPriority:            p.AIPriority,
		ExecutionQuality:      p.ExecutionQuality,
		SetupScore:            p.SetupScore,
		TimingScore:           p.TimingScore,
		RegimeFitScore:        p.RegimeFitScore,
		LiquidityScore:        p.LiquidityScore,
		RiskScore:             p.RiskScore,
		ReasonCodes:           p.ReasonCodes,
		RiskTags:              p.RiskTags,
		RequiredConfirmations: p.RequiredConfirmations,
		ConfirmationSummary:   p.ConfirmationSummary,
		ExecutionGeometry:     p.ExecutionGeometry,
		ExecutionContract:     p.ExecutionContract,
		EntryZone:             p.EntryZone,
		Invalidation:          p.Invalidation,
		Targets:               p.Targets,
		TP0Price:              p.TP0Price,
		TP1Price:              p.TP1Price,
		TP2Price:              p.TP2Price,
		PriceContext:          p.PriceContext,
		DerivativesContext:    p.DerivativesContext,
		DataFreshness:         p.DataFreshness,
		ExecutionReadiness:    p.ExecutionReadiness,
		ExecutionContext:      p.ExecutionContext,
	}
}

func marshalHunterV7JSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}
