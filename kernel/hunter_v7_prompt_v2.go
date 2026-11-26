package kernel

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Aixxww/AiT/market"
	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// U11: the hybrid_v2 candidate payload (fields audit, hybrid spec A/B/C/D;
// Task C pass 1). Raw closed bars are the primary input; a small renamed
// appendix carries what only code can compute. Selected by
// HunterConfig.V7Prompt.PayloadVersion = "hybrid_v2"; the default legacy
// payload never reaches this file.
//
// Pass-1 naming rules (dictionary): three namespaces market / signal /
// execution, unit suffixes (_pct percent, _frac 0-1, _ratio, _usdt, _ms,
// _price), one name per concept, enum values with a domain prefix. The
// deletion list (spec C) is enforced by TestU11DeletedFieldsNeverReachHybridPrompt.

// hunterV7PromptConfig is the strategy's effective U11 prompt config.
func (e *StrategyEngine) hunterV7PromptConfig() store.V7PromptConfig {
	if e == nil || e.config == nil || e.config.CoinSource.Hunter == nil {
		return store.EffectiveV7Prompt(nil)
	}
	return store.EffectiveV7Prompt(e.config.CoinSource.Hunter.V7Prompt)
}

// hunterV7PromptHybridV2 reports whether this strategy prompts with the
// hybrid_v2 payload and doctrine.
func (e *StrategyEngine) hunterV7PromptHybridV2() bool {
	return e != nil && e.config != nil && strings.EqualFold(e.config.CoinSource.SourceType, "hunter_v7") &&
		e.hunterV7PromptConfig().HybridV2()
}

// hunterV7MergeTimeframes returns have plus every timeframe of want it
// lacks, as a new slice (have may alias strategy config).
func hunterV7MergeTimeframes(have, want []string) []string {
	out := append([]string(nil), have...)
	seen := make(map[string]bool, len(out))
	for _, tf := range out {
		seen[tf] = true
	}
	for _, tf := range want {
		if !seen[tf] {
			out = append(out, tf)
			seen[tf] = true
		}
	}
	return out
}

// hunterV7PromptTime is the prompt build time (ctx.PromptTime, else now).
func hunterV7PromptTime(ctx *Context) time.Time {
	if ctx != nil && !ctx.PromptTime.IsZero() {
		return ctx.PromptTime
	}
	return time.Now()
}

// ---------------------------------------------------------------------------
// Payload
// ---------------------------------------------------------------------------

type hunterV7V2Payload struct {
	Market    hunterV7V2Market    `json:"market"`
	Signal    hunterV7V2Signal    `json:"signal"`
	Execution hunterV7V2Execution `json:"execution"`
}

// hunterV7V2Market: live market facts code computes and the bars cannot
// carry (spec D: OI / funding / taker aggregates, data ages).
type hunterV7V2Market struct {
	// PriceAtPromptBuild is the single reference price of every derived
	// number in the payload (fields audit N01).
	PriceAtPromptBuild float64 `json:"price_at_prompt_build"`
	// OINotionalUSDT is open interest in USDT notional (contracts x price,
	// hunter_v7_universe.go symbolOINotional; see the N15 note in the PR).
	OINotionalUSDT *float64 `json:"oi_notional_usdt,omitempty"`
	// OIChangeLast1hPct is the last 1h step of the OI history, percent;
	// OIChangeLast4x1hSumPct the sum of the last four 1h steps
	// (datafetch fetcher_per_symbol.go).
	OIChangeLast1hPct      *float64 `json:"oi_change_last_1h_pct,omitempty"`
	OIChangeLast4x1hSumPct *float64 `json:"oi_change_last_4x1h_sum_pct,omitempty"`
	// FundingRatePerSettlementFrac is one settlement's funding rate as a
	// fraction (N16), not annualised.
	FundingRatePerSettlementFrac *float64 `json:"funding_rate_per_settlement_frac,omitempty"`
	// TakerBuyRatioLatest15mBarFrac is the taker-buy share of the latest
	// 15m bar, which may be unfinished (N17).
	TakerBuyRatioLatest15mBarFrac *float64 `json:"taker_buy_ratio_latest_15m_bar_frac,omitempty"`
	SnapshotAgeMs                 int64    `json:"snapshot_age_ms,omitempty"`
	PriceAgeMs                    int64    `json:"price_age_ms,omitempty"`
	Kline1mAgeMs                  int64    `json:"kline_1m_age_ms,omitempty"`
	Kline5mAgeMs                  int64    `json:"kline_5m_age_ms,omitempty"`
	StaleThresholdMs              int64    `json:"stale_threshold_ms,omitempty"`
}

// hunterV7V2Signal: the module signal (identity, levels, one zone
// position, three named RRs, the M2 menus).
type hunterV7V2Signal struct {
	SignalID       string `json:"signal_id,omitempty"`
	Symbol         string `json:"symbol"`
	Direction      string `json:"direction"`
	SetupType      string `json:"setup_type"`
	Class          string `json:"class,omitempty"`
	LifecycleState string `json:"lifecycle_state,omitempty"`
	Status         string `json:"status"`
	// CellStatus is the U5 cell_status tag (plan §5.3; every cell is
	// probation until a cell evaluation exists).
	CellStatus string `json:"cell_status"`

	EntryZoneLowerPrice     float64  `json:"entry_zone_lower_price"`
	EntryZoneUpperPrice     float64  `json:"entry_zone_upper_price"`
	ZonePositionPct         *float64 `json:"zone_position_pct,omitempty"`
	TriggerPrice            float64  `json:"trigger_price,omitempty"`
	InvalidationPrice       float64  `json:"invalidation_price"`
	InvalidationDistancePct *float64 `json:"invalidation_distance_pct,omitempty"`

	NearestTargetPrice   float64  `json:"nearest_target_price,omitempty"`
	NearestTargetRRRatio *float64 `json:"nearest_target_rr_ratio,omitempty"`
	TP1Price             float64  `json:"tp1_price,omitempty"`
	TP1RRRatio           *float64 `json:"tp1_rr_ratio,omitempty"`
	TP2Price             float64  `json:"tp2_price,omitempty"`
	TP2RRRatio           *float64 `json:"tp2_rr_ratio,omitempty"`
	// RequiredConfirmations is the module's must-check condition list for
	// the LLM (fields audit N27: distinct from confirmation_summary, the
	// module's own confirmation evidence, which is not presented). Owner
	// decision on PR #28: kept in hybrid_v2; not on the deletion list.
	RequiredConfirmations []string                  `json:"required_confirmations"`
	RRBasis               hunterV7V2Basis           `json:"rr_basis"`
	StopCandidates        []local.V7StopCandidate   `json:"stop_candidates"`
	TargetCandidates      []local.V7TargetCandidate `json:"target_candidates"`
	MenuBasis             hunterV7V2MenuBasis       `json:"menu_basis"`
}

// hunterV7V2Basis states the RR basis (dictionary: "各自标注基准").
type hunterV7V2Basis struct {
	Entry  string `json:"entry"`
	Stop   string `json:"stop"`
	Target string `json:"target_cap"`
}

// hunterV7V2MenuBasis states how the M2 menus were computed.
type hunterV7V2MenuBasis struct {
	Mode          string  `json:"mode"`
	Status        string  `json:"status"`
	HorizonClass  string  `json:"horizon_class"`
	EntryPrice    float64 `json:"entry_price"`
	StructureTF   string  `json:"structure_tf"`
	HigherTF      string  `json:"higher_tf"`
	StructureBars int     `json:"structure_closed_bars"`
	HigherBars    int     `json:"higher_closed_bars"`
}

// hunterV7V2Execution: the backend envelope (code admission state and the
// limits the backend validates).
type hunterV7V2Execution struct {
	Tier            string           `json:"tier"`
	TierReason      string           `json:"tier_reason"`
	EntryPermission string           `json:"entry_permission"`
	LLMRole         string           `json:"llm_role"`
	DoctrineVersion string           `json:"doctrine_version"`
	Limits          hunterV7V2Limits `json:"limits"`
}

type hunterV7V2Limits struct {
	MinRRRatio         float64 `json:"min_rr_ratio"`
	MinStopDistancePct float64 `json:"min_stop_distance_pct"`
	MaxTPDistancePct   float64 `json:"max_tp_distance_pct"`
	MaxEntryDriftPct   float64 `json:"max_entry_drift_pct"`
	DriftReference     string  `json:"drift_reference"`
}

// Enum domain prefixes (dictionary: tier_EXECUTABLE, status_TRIGGERED).
func hunterV7V2Enum(domain, value string) string {
	if value == "" {
		return ""
	}
	return domain + "_" + value
}

// hunterV7V2SetupType presents a Task-2 setup type under its t2_ name
// whatever the emission setting (D2 (a)); native names are unchanged.
func hunterV7V2SetupType(coin CandidateCoin) string {
	if coin.V7Stack == local.V7StackT2 {
		return store.V7T2SetupPrefix + store.V7CanonicalSetupType(coin.V7SetupType)
	}
	return coin.V7SetupType
}

func hunterV7V2Round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

func hunterV7V2Ptr(v float64) *float64 { return &v }

// hunterV7V2RR is reward/risk from entry to target with stop as the stop.
func hunterV7V2RR(direction string, entry, stop, target float64) *float64 {
	if entry <= 0 || stop <= 0 || target <= 0 {
		return nil
	}
	dir := 1.0
	if strings.EqualFold(direction, "SHORT") {
		dir = -1
	}
	risk := dir * (entry - stop)
	reward := dir * (target - entry)
	if risk <= 0 || reward <= 0 {
		return nil
	}
	return hunterV7V2Ptr(hunterV7V2Round(reward/risk, 3))
}

// hunterV7V2ZonePositionPct is the single clamped zone position (dictionary
// signal.zone_position_pct): LONG from the lower edge, SHORT from the upper
// edge, at price. Undefined for a single-price zone.
func hunterV7V2ZonePositionPct(direction string, zone local.V7PriceZone, price float64) *float64 {
	width := zone.Upper - zone.Lower
	if price <= 0 || zone.Lower <= 0 || width <= 0 {
		return nil
	}
	pos := (price - zone.Lower) / width * 100
	if strings.EqualFold(direction, "SHORT") {
		pos = (zone.Upper - price) / width * 100
	}
	return hunterV7V2Ptr(hunterV7V2Round(math.Min(100, math.Max(0, pos)), 2))
}

func (e *StrategyEngine) buildHunterV7V2Payload(item hunterV7PromptCandidate, promptTime time.Time, geometry HunterV7ExecutionGeometry) hunterV7V2Payload {
	coin := item.Coin
	coin.V7ExecutionTier = item.Tier
	coin.V7TierReason = item.Reason
	price := 0.0
	if item.Data != nil {
		price = item.Data.CurrentPrice
	}
	if price <= 0 {
		price = hunterV7ReferencePrice(coin)
	}

	m := hunterV7V2Market{
		PriceAtPromptBuild: price,
		SnapshotAgeMs:      coin.V7DataFreshness.SnapshotAgeMs,
		PriceAgeMs:         coin.V7DataFreshness.PriceAgeMs,
		Kline1mAgeMs:       coin.V7DataFreshness.Kline1mAgeMs,
		Kline5mAgeMs:       coin.V7DataFreshness.Kline5mAgeMs,
		StaleThresholdMs:   coin.V7DataFreshness.StaleThresholdMs,
	}
	// A zero aggregate is read as missing and omitted rather than shown as
	// a fact (the derivatives context is all zeros without a snapshot).
	funding := 0.0
	if d := coin.V7DerivativesCtx; d != nil {
		if d.OIValue > 0 {
			m.OINotionalUSDT = hunterV7V2Ptr(d.OIValue)
		}
		if d.OIValue > 0 || d.OIChange1h != 0 || d.OIChange4h != 0 {
			m.OIChangeLast1hPct = hunterV7V2Ptr(d.OIChange1h)
			m.OIChangeLast4x1hSumPct = hunterV7V2Ptr(d.OIChange4h)
		}
		funding = d.FundingRate
		if d.TakerBuy15m > 0 {
			m.TakerBuyRatioLatest15mBarFrac = hunterV7V2Ptr(d.TakerBuy15m)
		}
	}
	if funding == 0 && item.Data != nil {
		funding = item.Data.FundingRate
	}
	if funding != 0 {
		m.FundingRatePerSettlementFrac = hunterV7V2Ptr(funding)
	}

	s := hunterV7V2Signal{
		SignalID:              coin.V7SignalID,
		Symbol:                coin.Symbol,
		Direction:             coin.Direction,
		SetupType:             hunterV7V2SetupType(coin),
		Class:                 hunterV7V2Enum("class", coin.V7SignalClass),
		Status:                hunterV7V2Enum("status", coin.V7Status),
		CellStatus:            hunterV7V2Enum("cell", store.V7CellStatusProbation),
		EntryZoneLowerPrice:   coin.V7EntryZone.Lower,
		EntryZoneUpperPrice:   coin.V7EntryZone.Upper,
		ZonePositionPct:       hunterV7V2ZonePositionPct(coin.Direction, coin.V7EntryZone, price),
		TriggerPrice:          coin.V7TriggerPrice,
		InvalidationPrice:     coin.V7Invalidation.Price,
		TP1Price:              coin.V7TP1Price,
		TP2Price:              coin.V7TP2Price,
		RequiredConfirmations: append([]string{}, coin.V7RequiredConfirms...),
		RRBasis:               hunterV7V2Basis{Entry: "price_at_prompt_build", Stop: "invalidation_price", Target: "uncapped"},
	}
	if coin.V7Stack == local.V7StackT2 {
		s.LifecycleState = hunterV7V2Enum("lifecycle", coin.V7LifecycleState)
	}
	if price > 0 && coin.V7Invalidation.Price > 0 {
		s.InvalidationDistancePct = hunterV7V2Ptr(hunterV7V2Round((coin.V7Invalidation.Price-price)/price*100, 4))
	}
	nearest, nearestReward := 0.0, math.Inf(1)
	for _, t := range coin.V7Targets {
		r := hunterV7DirectionalRewardPct(price, t.Price, coin.Direction)
		if r > 0 && r < nearestReward {
			nearest, nearestReward = t.Price, r
		}
	}
	s.NearestTargetPrice = nearest
	s.NearestTargetRRRatio = hunterV7V2RR(coin.Direction, price, coin.V7Invalidation.Price, nearest)
	s.TP1RRRatio = hunterV7V2RR(coin.Direction, price, coin.V7Invalidation.Price, coin.V7TP1Price)
	s.TP2RRRatio = hunterV7V2RR(coin.Direction, price, coin.V7Invalidation.Price, coin.V7TP2Price)
	s.StopCandidates, s.TargetCandidates, s.MenuBasis = hunterV7V2Menus(coin, item.Data, price, promptTime)

	minRR, minSL, maxTP := hunterV7CandidateRisk(coin, geometry)
	maxDrift, driftRef := hunterV7CandidateDrift(coin, geometry)
	if coin.V7RequiredMaxDrift > 0 {
		maxDrift = coin.V7RequiredMaxDrift
	}
	if coin.V7DriftReference != "" {
		driftRef = coin.V7DriftReference
	}
	x := hunterV7V2Execution{
		Tier:            hunterV7V2Enum("tier", item.Tier),
		TierReason:      hunterV7V2ReasonCode(item.Reason),
		EntryPermission: buildHunterV7ExecutionContract(coin).EntryPermission,
		LLMRole:         hunterV7V2Enum("role", e.hunterV7PromptConfig().LLMRole(coin.V7SignalClass, coin.V7ConditionalEntry)),
		DoctrineVersion: e.hunterV7PromptConfig().DoctrineVersion(),
		Limits: hunterV7V2Limits{
			MinRRRatio:         minRR,
			MinStopDistancePct: minSL,
			MaxTPDistancePct:   hunterV7V2Round(maxTP, 4),
			MaxEntryDriftPct:   maxDrift,
			DriftReference:     driftRef,
		},
	}
	return hunterV7V2Payload{Market: m, Signal: s, Execution: x}
}

// hunterV7V2Menus builds the M2 stop / target menus from the closed bars
// of the setup's horizon class, measured from price. Reference levels only
// (calculation mode): nothing reads the plan's own choice.
func hunterV7V2Menus(coin CandidateCoin, data *market.Data, price float64, promptTime time.Time) ([]local.V7StopCandidate, []local.V7TargetCandidate, hunterV7V2MenuBasis) {
	cfg := local.V7StructureConfigForSetup(coin.V7SetupType)
	basis := hunterV7V2MenuBasis{
		Mode: "reference_levels", Status: "no_data", HorizonClass: cfg.Class, EntryPrice: price,
		StructureTF: hunterV7V2TFLabel(cfg.TFSIntervalMs), HigherTF: hunterV7V2TFLabel(cfg.TFHIntervalMs),
	}
	stops, targets := []local.V7StopCandidate{}, []local.V7TargetCandidate{}
	if data == nil || price <= 0 {
		return stops, targets, basis
	}
	nowMs := promptTime.UnixMilli()
	barsOf := func(tf string, ivMs int64) ([]local.V7StructureBar, int) {
		series := data.TimeframeData[tf]
		if series == nil {
			return nil, 0
		}
		out := make([]local.V7StructureBar, 0, len(series.Klines))
		closed := 0
		for _, k := range series.Klines {
			out = append(out, local.V7StructureBar{OpenTimeMs: k.Time, Open: k.Open, High: k.High, Low: k.Low, Close: k.Close})
			if k.Time+ivMs <= nowMs {
				closed++
			}
		}
		return out, closed
	}
	barsS, nS := barsOf(basis.StructureTF, cfg.TFSIntervalMs)
	barsH, nH := barsOf(basis.HigherTF, cfg.TFHIntervalMs)
	basis.StructureBars, basis.HigherBars = nS, nH
	dir := local.V7DirLong
	if strings.EqualFold(coin.Direction, "SHORT") {
		dir = local.V7DirShort
	}
	plan := local.BuildV7StructurePlan(local.V7StructureInput{
		Direction: dir, EntryPrice: price, DecisionTimeMs: nowMs,
		BarsTFS: barsS, BarsTFH: barsH,
		Family:            local.V7StructureFamilyForSetup(coin.V7SetupType),
		InvalidationPrice: coin.V7Invalidation.Price,
		Config:            cfg,
	})
	if plan == nil || plan.Reason == "no_data" {
		return stops, targets, basis
	}
	basis.Status = "ok"
	if plan.StopCandidates != nil {
		stops = plan.StopCandidates
	}
	if plan.TargetCandidates != nil {
		targets = plan.TargetCandidates
	}
	return stops, targets, basis
}

func hunterV7V2TFLabel(ms int64) string {
	switch {
	case ms <= 0:
		return ""
	case ms%(24*3600*1000) == 0:
		return strconv.FormatInt(ms/(24*3600*1000), 10) + "d"
	case ms%(3600*1000) == 0:
		return strconv.FormatInt(ms/(3600*1000), 10) + "h"
	default:
		return strconv.FormatInt(ms/(60*1000), 10) + "m"
	}
}

// hunterV7V2TFInterval parses "5m" / "15m" / "1h" / "4h" / "1d" / "1w".
func hunterV7V2TFInterval(tf string) time.Duration {
	if len(tf) < 2 {
		return 0
	}
	n, err := strconv.Atoi(tf[:len(tf)-1])
	if err != nil || n <= 0 {
		return 0
	}
	switch tf[len(tf)-1] {
	case 'm':
		return time.Duration(n) * time.Minute
	case 'h':
		return time.Duration(n) * time.Hour
	case 'd':
		return time.Duration(n) * 24 * time.Hour
	case 'w':
		return time.Duration(n) * 7 * 24 * time.Hour
	}
	return 0
}

// hunterV7V2DeletedReasonRenames applies pass-1 renames inside reason
// codes, so a tier reason never carries a deleted field name (spec C).
// risk_score -> risk_penalty_0_100 is the dictionary entry; the flow
// labels are deleted outright, so a reason naming one reads as the gate it
// is (taker_flow_gate); the remaining deleted score names have no
// dictionary entry free of the deleted token and read as module_metric.
var hunterV7V2DeletedReasonRenames = []struct{ from, to string }{
	{"risk_score", "risk_penalty_0_100"},
	{"setup_score", "module_metric"},
	{"timing_score", "module_metric"},
	{"liquidity_score", "module_metric"},
	{"regime_fit_score", "module_metric"},
	{"ai_priority", "module_metric"},
	{"confidence", "module_metric"},
	{"market_regime", "module_metric"},
	{"market_shape", "module_metric"},
	{"entry_signal", "module_metric"},
}

func hunterV7V2ReasonCode(reason string) string {
	for _, tag := range hunterV7V2FlowLabels() {
		reason = strings.ReplaceAll(reason, tag, "taker_flow_gate")
	}
	reason = hunterV7V2FlowTokenRe.ReplaceAllString(reason, "${1}taker_flow_gate")
	for _, r := range hunterV7V2DeletedReasonRenames {
		reason = strings.ReplaceAll(reason, r.from, r.to)
	}
	return reason
}

// hunterV7V2FlowTokenRe matches a flow_ label that starts a token (not the
// "flow_" inside a setup name such as whale_flow_reversal).
var hunterV7V2FlowTokenRe = regexp.MustCompile(`(^|[^a-z0-9_])flow_[a-z0-9_]+`)

// hunterV7V2FlowLabels lists the catalog's flow_ tag vocabulary, longest
// first (so a longer tag is replaced before its prefix).
func hunterV7V2FlowLabels() []string {
	var tags []string
	for _, def := range local.HunterV7TagCatalogEntries() {
		if strings.HasPrefix(def.Tag, "flow_") {
			tags = append(tags, def.Tag)
		}
	}
	sort.SliceStable(tags, func(i, j int) bool { return len(tags[i]) > len(tags[j]) })
	return tags
}

// ---------------------------------------------------------------------------
// Writer
// ---------------------------------------------------------------------------

func (e *StrategyEngine) writeHunterV7HybridV2CandidatePrompt(sb *strings.Builder, ctx *Context, items []hunterV7PromptCandidate) {
	pc := e.hunterV7PromptConfig()
	promptTime := hunterV7PromptTime(ctx)
	geometry := e.hunterV7ExecutionGeometry()
	zh := e.GetLanguage() == LangChinese
	execCount, reviewableCount, watchCount, rejectedCount := hunterV7PromptTierCounts(items)

	positionLimitReached := e.config.RiskControl.MaxPositions > 0 && len(ctx.Positions) >= e.config.RiskControl.MaxPositions
	sb.WriteString(fmt.Sprintf("## Hunter v7 Candidate Tiers (%d total)\n\n", len(items)))
	sb.WriteString(fmt.Sprintf("%s | prompt_time_utc=%s\n",
		hunterV7PromptHeader(pc), promptTime.UTC().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("Tier Summary: EXECUTABLE=%d | REVIEWABLE=%d | WATCH=%d | REJECTED=%d\n\n", execCount, reviewableCount, watchCount, rejectedCount))
	switch {
	case positionLimitReached && zh:
		sb.WriteString("Decision policy: 当前持仓数量已达到 Max Positions；候选只作背景摘要，不展开、不要求逐个决策，除非先明确 close 现有仓位，否则禁止新开仓。\n\n")
	case positionLimitReached:
		sb.WriteString("Decision policy: Current positions have reached Max Positions; candidates are summary-only context. Do not expand or decide each candidate, and do not open unless an existing position is explicitly closed first.\n\n")
	case execCount+reviewableCount > 0 && zh:
		sb.WriteString("Decision policy: 按系统提示的 Hunter v7 混合制执行框架处理本轮候选——先逐个审 tier_EXECUTABLE，再审 tier_REVIEWABLE，各按其 execution.llm_role；空仓且存在上述候选时，必须给出最佳 open 或恰好一个 blocked_reason_code（附 blocked_signal_symbol）。\n\n")
	case execCount+reviewableCount > 0:
		sb.WriteString("Decision policy: apply the Hunter v7 hybrid framework from the system prompt to this cycle's pool — audit every tier_EXECUTABLE first, then tier_REVIEWABLE, each by its execution.llm_role; when flat with such candidates present, output the best open or exactly one blocked_reason_code (with blocked_signal_symbol).\n\n")
	case zh:
		sb.WriteString("Decision policy: 当前没有 EXECUTABLE/REVIEWABLE 候选。不要强行开仓；只允许 wait 或管理已有持仓。\n\n")
	default:
		sb.WriteString("Decision policy: No EXECUTABLE/REVIEWABLE candidates are available. Do not force a new open; only wait or manage existing positions.\n\n")
	}

	openReviewLimit := hunterV7OpenReviewExpansionLimit(execCount+reviewableCount, len(ctx.Positions), e.config.RiskControl.MaxPositions)
	expanded := hunterV7SelectExpandedOpenReview(items, openReviewLimit)
	sb.WriteString(fmt.Sprintf("### Open-review candidates (full context, max %d)\n\n", openReviewLimit))
	displayed := 0
	if positionLimitReached {
		sb.WriteString("- None (position limit reached; open-review candidates are summary-only below)\n\n")
	} else {
		for idx, item := range items {
			if item.Tier != "EXECUTABLE" && item.Tier != "REVIEWABLE" {
				continue
			}
			if !expanded[idx] {
				sb.WriteString(fmt.Sprintf("- %s %s setup_type=%s tier=%s tier_reason=%s (not expanded: lower rank)\n",
					item.Coin.Symbol, item.Coin.Direction, hunterV7V2SetupType(item.Coin), hunterV7V2Enum("tier", item.Tier), hunterV7V2ReasonCode(item.Reason)))
				continue
			}
			displayed++
			e.writeHunterV7V2ExpandedCandidate(sb, item, displayed, promptTime, geometry, pc)
		}
	}
	if displayed == 0 && !positionLimitReached {
		sb.WriteString("- None\n\n")
	}

	summaryLine := func(item hunterV7PromptCandidate) string {
		return fmt.Sprintf("- %s %s setup_type=%s tier=%s status=%s tier_reason=%s\n",
			item.Coin.Symbol, item.Coin.Direction, hunterV7V2SetupType(item.Coin), hunterV7V2Enum("tier", item.Tier),
			hunterV7V2Enum("status", item.Coin.V7Status), hunterV7V2ReasonCode(item.Reason))
	}
	if positionLimitReached {
		sb.WriteString("### Open-disabled candidate summary\n\n")
		shown := 0
		for _, item := range items {
			if item.Tier == "REJECTED" {
				continue
			}
			if shown >= 6 {
				break
			}
			shown++
			sb.WriteString(summaryLine(item))
		}
		if shown == 0 {
			sb.WriteString("- None\n")
		}
		if n := execCount + reviewableCount + watchCount; n > shown {
			sb.WriteString(fmt.Sprintf("- ... %d more non-rejected candidates omitted\n", n-shown))
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("### WATCH candidates (summary only; not direct open)\n\n")
		shown := 0
		for _, item := range items {
			if item.Tier != "WATCH" {
				continue
			}
			if shown >= 6 {
				break
			}
			shown++
			sb.WriteString(summaryLine(item))
		}
		if shown == 0 {
			sb.WriteString("- None\n")
		}
		if watchCount > shown {
			sb.WriteString(fmt.Sprintf("- ... %d more WATCH candidates omitted\n", watchCount-shown))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("### REJECTED candidates (count only)\n\n")
	if rejectedCount == 0 {
		sb.WriteString("- None\n\n")
		return
	}
	counts := map[string]int{}
	for _, item := range items {
		if item.Tier == "REJECTED" {
			counts[hunterV7V2ReasonCode(item.Reason)]++
		}
	}
	reasons := make([]string, 0, len(counts))
	for r := range counts {
		reasons = append(reasons, r)
	}
	sort.Strings(reasons)
	for _, r := range reasons {
		sb.WriteString(fmt.Sprintf("- %s: %d\n", r, counts[r]))
	}
	sb.WriteString("\n")
}

func (e *StrategyEngine) writeHunterV7V2ExpandedCandidate(sb *strings.Builder, item hunterV7PromptCandidate, index int, promptTime time.Time, geometry HunterV7ExecutionGeometry, pc store.V7PromptConfig) {
	coin := item.Coin
	directionTag := ""
	if coin.Direction == "LONG" || coin.Direction == "SHORT" {
		directionTag = fmt.Sprintf(" [%s]", coin.Direction)
	}
	sb.WriteString(fmt.Sprintf("#### %d. %s%s\n\n", index, coin.Symbol, directionTag))
	sb.WriteString("hunter_v7_signal_v2_json: ")
	sb.WriteString(marshalHunterV7JSON(e.buildHunterV7V2Payload(item, promptTime, geometry)))
	sb.WriteString("\n\n")
	writeHunterV7V2Bars(sb, item.Data, pc, promptTime)
}

// writeHunterV7V2Bars prints, per timeframe, the closed bars old->new and
// the unfinished bar on its own line (hybrid spec A).
func writeHunterV7V2Bars(sb *strings.Builder, data *market.Data, pc store.V7PromptConfig, promptTime time.Time) {
	nowMs := promptTime.UnixMilli()
	for _, tf := range pc.Timeframes {
		var series *market.TimeframeSeriesData
		if data != nil {
			series = data.TimeframeData[tf]
		}
		iv := hunterV7V2TFInterval(tf).Milliseconds()
		if series == nil || len(series.Klines) == 0 || iv <= 0 {
			sb.WriteString(fmt.Sprintf("bars_%s: not available\n\n", tf))
			continue
		}
		n := len(series.Klines)
		closed := make([]int, 0, n)
		var unfinished []int
		for i, k := range series.Klines {
			if k.Time+iv <= nowMs {
				closed = append(closed, i)
			} else {
				unfinished = append(unfinished, i)
			}
		}
		if len(closed) > pc.BarsPerTimeframe {
			closed = closed[len(closed)-pc.BarsPerTimeframe:]
		}
		sb.WriteString(fmt.Sprintf("bars_%s (%d closed, old->new, UTC; columns: time_utc open high low close volume ema20 ema50 macd rsi7 rsi14 boll_upper boll_middle boll_lower):\n", tf, len(closed)))
		for _, i := range closed {
			sb.WriteString(hunterV7V2BarRow(series, i))
			sb.WriteString("\n")
		}
		for _, i := range unfinished {
			sb.WriteString(fmt.Sprintf("unfinished_%s (unfinished — do not use for levels): ", tf))
			sb.WriteString(hunterV7V2BarRow(series, i))
			sb.WriteString("\n")
		}
		if series.ATR14 > 0 {
			sb.WriteString(fmt.Sprintf("atr14_%s_price (computed through the unfinished bar): %s\n", tf, hunterV7V2Num(series.ATR14)))
		}
		sb.WriteString("\n")
	}
}

// hunterV7V2BarRow renders bar i with its indicator values. Indicator
// series are right-aligned to the bars (shorter while warming up).
func hunterV7V2BarRow(s *market.TimeframeSeriesData, i int) string {
	k := s.Klines[i]
	n := len(s.Klines)
	at := func(series []float64) string {
		j := i - (n - len(series))
		if j < 0 || j >= len(series) {
			return "-"
		}
		return hunterV7V2Num(series[j])
	}
	cols := []string{
		time.UnixMilli(k.Time).UTC().Format("2006-01-02T15:04Z"),
		hunterV7V2Num(k.Open), hunterV7V2Num(k.High), hunterV7V2Num(k.Low), hunterV7V2Num(k.Close), hunterV7V2Num(k.Volume),
		at(s.EMA20Values), at(s.EMA50Values), at(s.MACDValues), at(s.RSI7Values), at(s.RSI14Values),
		at(s.BOLLUpper), at(s.BOLLMiddle), at(s.BOLLLower),
	}
	return strings.Join(cols, " ")
}

// hunterV7V2Num formats with 7 significant digits in plain notation.
func hunterV7V2Num(v float64) string {
	if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return "0"
	}
	d := 6 - int(math.Floor(math.Log10(math.Abs(v))))
	if d < 0 {
		d = 0
	}
	if d > 12 {
		d = 12
	}
	out := strconv.FormatFloat(v, 'f', d, 64)
	if strings.Contains(out, ".") {
		out = strings.TrimRight(strings.TrimRight(out, "0"), ".")
	}
	return out
}
