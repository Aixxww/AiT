package store

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

// #95 / design §5.4: one resolver for the trading-behaviour config.
//
// ResolveHunterV7 turns a strategy config into every effective value the
// kernel and the trader act on, each with its source:
//   - "config":       the strategy sets it and it is used as set;
//   - "default":      the strategy leaves it unset; the documented default applies;
//   - "safety_limit": a §5.3 hard limit in code bounds or replaces the value.
//
// The resolver does not change what any value resolves to (except RR when
// unset: it resolves to DefaultMinRiskRewardRatio 1.2 everywhere, never to
// the old kernel-only 3.0, as #95 specifies). It only makes the resolution
// single-sourced and visible: the kernel's router config, the trader's
// guards and the decision record all read it.

// ConfigSource is where an effective value comes from.
type ConfigSource string

const (
	SourceConfig      ConfigSource = "config"
	SourceDefault     ConfigSource = "default"
	SourceSafetyLimit ConfigSource = "safety_limit"
)

// Router defaults (provider/local DefaultV7Config; kernel tests pin them).
const (
	DefaultV7MaxOutput              = 30
	DefaultV7WatchOutput            = 5
	DefaultV7MinAIPriority          = 55.0
	DefaultV7MinOutput              = 3
	DefaultV7FallbackMinAIPriority  = 45.0
	DefaultV7CorrelationMaxPerTheme = 3
)

// Trader fallbacks (unchanged values, now named).
const (
	DefaultHunterV7MaxEntryDriftPct     = 0.5
	DefaultHunterV7MaxTakeProfitMovePct = 3.0
	DefaultHunterV7MinStopLossMovePct   = 2.0
	DefaultHunterV7MaxSingleTradeLoss   = 8.0
	DefaultBTCETHPositionValueRatio     = 5.0
	DefaultAltcoinPositionValueRatio    = 1.0
	DefaultMinPositionSizeUSD           = 12.0
	// DefaultMinConfidenceFloor is the open-confidence floor when
	// min_confidence is unset (the kernel's legacy floor, #77).
	DefaultMinConfidenceFloor = 60
)

// Cooldown defaults (were literals in trader/auto_trader_loop.go).
const (
	DefaultFailedOpenCooldownCycles       = 3
	DefaultRecentLossCooldownWindowMins   = 60
	DefaultRecentLossCooldownMinLosses    = 2
	DefaultRecentLossCooldownLossPct      = 5.0
	DefaultRecentLossCooldownEnabledForV7 = true
)

// §5.3 safety hard limits. They stay in code, outside frontend control; the
// owner decides each (see the #95 PR). Values are today's.
const (
	// SafetyMaxPositionsCeiling is a distinct ceiling above the trading
	// setting max_positions. 3 is carried over from the pre-#82 enforced
	// bound (store.MaxPositions); it is NOT a new recommendation.
	SafetyMaxPositionsCeiling = 3
	// SafetyMaxLeverage is the UI leverage ceiling (RiskControlEditor max 20).
	SafetyMaxLeverage = 20
	// SafetyMaxScanIntervalMinutes / SafetyMinScanIntervalMinutes bound the
	// trader's decision loop (UI 3..60).
	SafetyMinScanIntervalMinutes = 3
	SafetyMaxScanIntervalMinutes = 60
	// SafetyMarginFactor documents the order sizing margin factor
	// 1.01/leverage + 0.001 (trader/auto_trader_orders.go).
	SafetyMarginFactorBuffer = 0.01
	SafetyMarginFactorFee    = 0.001
	// SafetyNakedStopGraceSeconds / SafetyEmergencyCloseAttempts document the
	// #76 P0 backstop (trader/auto_trader_naked_guard.go, auto_trader_protection.go).
	SafetyNakedStopGraceSeconds  = 20
	SafetyEmergencyCloseAttempts = 3
)

// ErrMaxPositionsAboveSafetyCeiling: max_positions above the §5.3 ceiling.
var ErrMaxPositionsAboveSafetyCeiling = errors.New("risk_control.max_positions above the safety ceiling")

// CooldownConfig is RiskControlConfig.Cooldowns (#95 §5.4.2). Pointer fields
// separate "unset" (nil: the documented default) from "off" (0).
type CooldownConfig struct {
	// FailedOpenCycles: after a rejected open, the symbol is skipped for
	// this many decision records. nil = 3, 0 = off.
	FailedOpenCycles *int `json:"failed_open_cycles,omitempty"`
	// RecentLoss: Hunter v7 same-symbol-and-direction loss cooldown.
	RecentLoss *RecentLossCooldownConfig `json:"recent_loss,omitempty"`
}

// RecentLossCooldownConfig: skip a symbol/direction after MinLosses closed
// trades losing at least LossPct within WindowMinutes. nil fields = the
// defaults (60 min, 2 losses, 5%); WindowMinutes 0 = off.
type RecentLossCooldownConfig struct {
	WindowMinutes *int     `json:"window_minutes,omitempty"`
	MinLosses     *int     `json:"min_losses,omitempty"`
	LossPct       *float64 `json:"loss_pct,omitempty"`
}

// ResolvedValue is one effective value with its source, for display and
// for the decision record.
type ResolvedValue struct {
	Key        string       `json:"key"`
	Group      string       `json:"group"`
	Value      interface{}  `json:"value"`
	Source     ConfigSource `json:"source"`
	Configured interface{}  `json:"configured,omitempty"`
	Limit      string       `json:"limit,omitempty"`
	Note       string       `json:"note,omitempty"`
	// Enforced=false marks a value that is shown to the LLM but not
	// enforced in execution (max_margin_usage).
	Enforced *bool `json:"enforced,omitempty"`
}

// RecentLossCooldown is the effective recent-loss cooldown.
type RecentLossCooldown struct {
	Enabled       bool
	WindowMinutes int
	MinLosses     int
	LossPct       float64
}

// ResolvedHunterV7 is the effective trading-behaviour config.
type ResolvedHunterV7 struct {
	HunterV7 bool

	// Router (provider/local V7Config).
	MaxOutput              int
	WatchOutput            int
	MinAIPriority          float64
	Aggressive             bool
	MinOutput              int
	FallbackMinAIPriority  float64
	CorrelationMaxPerTheme int // -1 = off

	// Decision gates.
	MinRR           float64
	MaxPositions    int
	MaxPositionsErr error
	MinConfidence   int

	// Execution geometry and sizing.
	GeometryRepair            string
	MaxEntryDriftPct          float64 // 0 = no drift check
	MaxTakeProfitMovePct      float64 // before the trader's feasibility raise; 0 = no cap
	MinStopLossMovePct        float64
	MaxSingleTradeLossPct     float64 // 0 = no cap
	BTCETHPositionValueRatio  float64
	AltcoinPositionValueRatio float64
	MinPositionSizeUSD        float64
	BTCETHMaxLeverage         int // 0 = no cap in the trader (§5.3 item 5)
	AltcoinMaxLeverage        int
	MaxMarginUsage            float64 // prompt-only, NOT enforced

	// Cooldowns.
	FailedOpenCooldownCycles int // 0 = off
	RecentLossCooldown       RecentLossCooldown

	// Setup guards: strategy entries over the built-in defaults.
	SetupGuard map[string]SetupGuardConfig

	DailyLossBreaker DailyLossBreakerConfig

	// Values lists every resolved value, grouped, for display/records.
	Values []ResolvedValue
}

// BuiltinSetupGuards are the trader's built-in setup guard defaults (were
// trader/auto_trader_risk.go builtinSetupGuardDefaults; values unchanged).
// MinAIPriority is not part of the built-ins and is not enforced by the
// trader (only zone position and OI flush are).
func BuiltinSetupGuards() map[string]SetupGuardConfig {
	return map[string]SetupGuardConfig{
		"funding_reversal":           {MinZonePosShort: 0, MaxZonePosLong: 35, RequireOIFlush: true},
		"distribution_short":         {MinZonePosShort: 60, MaxZonePosLong: 100},
		"long_squeeze_short":         {MinZonePosShort: 60, MaxZonePosLong: 100},
		"breakdown_momentum_short":   {MinZonePosShort: 45, MaxZonePosLong: 100},
		"alt_ladder_breakdown_short": {MinZonePosShort: 40, MaxZonePosLong: 100},
		"range_reversion":            {MinZonePosShort: 55, MaxZonePosLong: 45},
		"pullback_reversal_long":     {MinZonePosShort: 0, MaxZonePosLong: 50},
		"alt_ladder_momentum_long":   {MinZonePosShort: 0, MaxZonePosLong: 72},
		"mms_bottom_wake_long":       {MinZonePosShort: 0, MaxZonePosLong: 65},
		"mms_trend_ride_long":        {MinZonePosShort: 0, MaxZonePosLong: 70},
		"mms_squeeze_engine_long":    {MinZonePosShort: 0, MaxZonePosLong: 75},
	}
}

// SetupGuardFor returns the effective guard of a setup type: the strategy's
// entry (exact, canonical or prefixed name) or the built-in default.
func (r ResolvedHunterV7) SetupGuardFor(setupType string) (SetupGuardConfig, bool) {
	if g, ok := LookupV7Setup(r.SetupGuard, setupType); ok {
		return g, true
	}
	return SetupGuardConfig{}, false
}

// Value returns one resolved value by key.
func (r ResolvedHunterV7) Value(key string) (ResolvedValue, bool) {
	for _, v := range r.Values {
		if v.Key == key {
			return v, true
		}
	}
	return ResolvedValue{}, false
}

type resolver struct{ out *ResolvedHunterV7 }

func (rs resolver) add(group, key string, value interface{}, src ConfigSource, configured interface{}, limit, note string) {
	rs.out.Values = append(rs.out.Values, ResolvedValue{Key: key, Group: group, Value: value, Source: src, Configured: configured, Limit: limit, Note: note})
}

func srcOf(set bool) ConfigSource {
	if set {
		return SourceConfig
	}
	return SourceDefault
}

// ResolveHunterV7 resolves cfg (nil = the defaults of an empty strategy).
func ResolveHunterV7(cfg *StrategyConfig) ResolvedHunterV7 {
	if cfg == nil {
		cfg = &StrategyConfig{}
	}
	out := ResolvedHunterV7{HunterV7: strings.EqualFold(cfg.CoinSource.SourceType, "hunter_v7")}
	rs := resolver{out: &out}
	rc := cfg.RiskControl
	cs := cfg.CoinSource
	h := cs.Hunter

	// ---- Router ----
	maxOut, maxOutSrc := DefaultV7MaxOutput, SourceDefault
	if h != nil && h.V7MaxOutput > 0 {
		maxOut, maxOutSrc = h.V7MaxOutput, SourceConfig
	}
	rs.add("router", "v7_max_output", maxOut, maxOutSrc, nil, "", "")
	hunterLimit, hlSrc, hlNote := cs.HunterLimit, SourceConfig, ""
	switch {
	case cs.HunterLimit <= 0:
		hunterLimit, hlSrc, hlNote = MaxCandidateCoins, SourceDefault, "unset: ClampLimits sets it to the cap"
	case cs.HunterLimit > MaxCandidateCoins:
		hunterLimit, hlSrc, hlNote = MaxCandidateCoins, SourceSafetyLimit, fmt.Sprintf("configured %d clamped to %d (token budget)", cs.HunterLimit, MaxCandidateCoins)
	}
	rs.add("router", "hunter_limit", hunterLimit, hlSrc, cs.HunterLimit, fmt.Sprintf("1..%d", MaxCandidateCoins), hlNote)
	effOut, effSrc, effNote := maxOut, maxOutSrc, ""
	if hunterLimit > 0 && hunterLimit < effOut {
		effOut, effSrc = hunterLimit, hlSrc
	}
	if effOut <= 0 || effOut > MaxCandidateCoins {
		effOut, effSrc, effNote = MaxCandidateCoins, SourceSafetyLimit, fmt.Sprintf("LLM output capped at MaxCandidateCoins %d", MaxCandidateCoins)
	}
	out.MaxOutput = effOut
	rs.add("router", "llm_candidate_count", effOut, effSrc, nil, fmt.Sprintf("<= %d", MaxCandidateCoins), firstNonEmpty(effNote, "min(v7_max_output, hunter_limit)"))

	out.WatchOutput = DefaultV7WatchOutput
	if h != nil && h.V7WatchOutput > 0 {
		out.WatchOutput = h.V7WatchOutput
	}
	rs.add("router", "v7_watch_output", out.WatchOutput, srcOf(h != nil && h.V7WatchOutput > 0), nil, "", "")
	out.MinAIPriority = DefaultV7MinAIPriority
	if h != nil && h.V7MinAIPriority > 0 {
		out.MinAIPriority = h.V7MinAIPriority
	}
	rs.add("router", "v7_min_ai_priority", out.MinAIPriority, srcOf(h != nil && h.V7MinAIPriority > 0), nil, "", "")
	out.Aggressive = h != nil && h.V7Aggressive
	rs.add("router", "v7_aggressive", out.Aggressive, srcOf(h != nil && h.V7Aggressive), nil, "", "")
	out.MinOutput = DefaultV7MinOutput
	if h != nil && h.V7MinOutput != nil {
		out.MinOutput = *h.V7MinOutput
	}
	rs.add("router", "v7_min_output", out.MinOutput, srcOf(h != nil && h.V7MinOutput != nil), nil, ">= 1", "correlation-filter backfill floor")
	out.FallbackMinAIPriority = DefaultV7FallbackMinAIPriority
	if h != nil && h.V7FallbackMinAIPriority != nil {
		out.FallbackMinAIPriority = *h.V7FallbackMinAIPriority
	}
	rs.add("router", "v7_fallback_min_ai_priority", out.FallbackMinAIPriority, srcOf(h != nil && h.V7FallbackMinAIPriority != nil), nil, "", "")
	out.CorrelationMaxPerTheme = DefaultV7CorrelationMaxPerTheme
	corrNote := ""
	if h != nil && h.V7CorrelationMaxPerTheme != nil {
		out.CorrelationMaxPerTheme = *h.V7CorrelationMaxPerTheme
		if out.CorrelationMaxPerTheme == 0 {
			out.CorrelationMaxPerTheme, corrNote = -1, "0 = off"
		}
	}
	rs.add("router", "v7_correlation_max_per_theme", out.CorrelationMaxPerTheme, srcOf(h != nil && h.V7CorrelationMaxPerTheme != nil), nil, "", corrNote)

	// ---- Decision gates ----
	out.MinRR = DefaultMinRiskRewardRatio
	if rc.MinRiskRewardRatio > 0 {
		out.MinRR = rc.MinRiskRewardRatio
	}
	rs.add("decision", "min_risk_reward_ratio", out.MinRR, srcOf(rc.MinRiskRewardRatio > 0), nil, "", "")
	out.MaxPositions, out.MaxPositionsErr = rc.ValidateMaxPositions()
	mpSrc, mpNote := SourceConfig, ""
	if out.MaxPositionsErr != nil {
		mpSrc, mpNote = SourceSafetyLimit, "opens blocked: "+out.MaxPositionsErr.Error()
	}
	rs.add("decision", "max_positions", rc.MaxPositions, mpSrc, nil, fmt.Sprintf("1..%d (safety ceiling)", SafetyMaxPositionsCeiling), mpNote)
	out.MinConfidence, _ = ResolveMinConfidence(rc.MinConfidence)
	rs.add("decision", "min_confidence", out.MinConfidence, srcOf(rc.MinConfidence > 0), nil, "0..100", "")

	// ---- Execution ----
	out.GeometryRepair = "repair"
	if strings.EqualFold(strings.TrimSpace(rc.GeometryRepair), "reject") {
		out.GeometryRepair = "reject"
	}
	rs.add("execution", "geometry_repair", out.GeometryRepair, srcOf(strings.TrimSpace(rc.GeometryRepair) != ""), nil, "repair | reject", "")
	fallbackPct := func(key string, configured, hunterDefault float64, note string) float64 {
		v, src := configured, SourceConfig
		if configured <= 0 {
			v, src = 0, SourceDefault
			if out.HunterV7 {
				v = hunterDefault
			}
		}
		rs.add("execution", key, v, src, nil, "", note)
		return v
	}
	out.MaxEntryDriftPct = fallbackPct("max_entry_price_deviation_pct", rc.MaxEntryPriceDeviationPct, DefaultHunterV7MaxEntryDriftPct, "")
	out.MaxTakeProfitMovePct = fallbackPct("max_take_profit_price_move_pct", rc.MaxTakeProfitPriceMovePct, DefaultHunterV7MaxTakeProfitMovePct, "the trader may raise it to keep min RR feasible")
	out.MinStopLossMovePct = fallbackPct("min_stop_loss_price_move_pct", rc.MinStopLossPriceMovePct, DefaultHunterV7MinStopLossMovePct, "")
	out.MaxSingleTradeLossPct = fallbackPct("max_single_trade_loss_pct", rc.MaxSingleTradeLossPct, DefaultHunterV7MaxSingleTradeLoss, "")

	ratio := func(key string, configured, def float64) float64 {
		v, src := configured, SourceConfig
		if configured <= 0 {
			v, src = def, SourceDefault
		}
		rs.add("sizing", key, v, src, nil, "", "")
		return v
	}
	out.BTCETHPositionValueRatio = ratio("btc_eth_max_position_value_ratio", rc.BTCETHMaxPositionValueRatio, DefaultBTCETHPositionValueRatio)
	out.AltcoinPositionValueRatio = ratio("altcoin_max_position_value_ratio", rc.AltcoinMaxPositionValueRatio, DefaultAltcoinPositionValueRatio)
	out.MinPositionSizeUSD = ratio("min_position_size", rc.MinPositionSize, DefaultMinPositionSizeUSD)
	lev := func(key string, v int) int {
		note := ""
		src := SourceConfig
		if v <= 0 {
			src, note = SourceDefault, "0/unset = no leverage cap in the trader (§5.3 item 5, owner decision)"
		}
		rs.add("sizing", key, v, src, nil, fmt.Sprintf("<= %d", SafetyMaxLeverage), note)
		return v
	}
	out.BTCETHMaxLeverage = lev("btc_eth_max_leverage", rc.BTCETHMaxLeverage)
	out.AltcoinMaxLeverage = lev("altcoin_max_leverage", rc.AltcoinMaxLeverage)
	out.MaxMarginUsage = rc.MaxMarginUsage
	notEnforced := false
	out.Values = append(out.Values, ResolvedValue{Key: "max_margin_usage", Group: "sizing", Value: rc.MaxMarginUsage, Source: srcOf(rc.MaxMarginUsage > 0),
		Note: "shown to the LLM only; no execution code enforces it (confirmed gap, owner decision)", Enforced: &notEnforced})

	// ---- Cooldowns ----
	out.FailedOpenCooldownCycles = DefaultFailedOpenCooldownCycles
	foSet := rc.Cooldowns != nil && rc.Cooldowns.FailedOpenCycles != nil
	if foSet {
		out.FailedOpenCooldownCycles = *rc.Cooldowns.FailedOpenCycles
	}
	rs.add("cooldown", "failed_open_cycles", out.FailedOpenCooldownCycles, srcOf(foSet), nil, ">= 0", "0 = off; keyed by symbol only (both directions)")
	// The caller applies it to Hunter v7 strategies only.
	rl := RecentLossCooldown{Enabled: DefaultRecentLossCooldownEnabledForV7, WindowMinutes: DefaultRecentLossCooldownWindowMins,
		MinLosses: DefaultRecentLossCooldownMinLosses, LossPct: DefaultRecentLossCooldownLossPct}
	var rlc *RecentLossCooldownConfig
	if rc.Cooldowns != nil {
		rlc = rc.Cooldowns.RecentLoss
	}
	if rlc != nil && rlc.WindowMinutes != nil {
		rl.WindowMinutes = *rlc.WindowMinutes
	}
	if rlc != nil && rlc.MinLosses != nil {
		rl.MinLosses = *rlc.MinLosses
	}
	if rlc != nil && rlc.LossPct != nil {
		rl.LossPct = *rlc.LossPct
	}
	if rl.WindowMinutes <= 0 || rl.MinLosses <= 0 {
		rl.Enabled = false
	}
	out.RecentLossCooldown = rl
	rs.add("cooldown", "recent_loss_window_minutes", rl.WindowMinutes, srcOf(rlc != nil && rlc.WindowMinutes != nil), nil, ">= 0", "0 = off; Hunter v7 only; per symbol and direction")
	rs.add("cooldown", "recent_loss_min_losses", rl.MinLosses, srcOf(rlc != nil && rlc.MinLosses != nil), nil, ">= 1", "")
	rs.add("cooldown", "recent_loss_loss_pct", rl.LossPct, srcOf(rlc != nil && rlc.LossPct != nil), nil, "> 0", "a trade counts when PnL% <= -loss_pct")

	// ---- Setup guards ----
	out.SetupGuard = BuiltinSetupGuards()
	for k, v := range rc.SetupGuard {
		out.SetupGuard[V7CanonicalSetupType(k)] = v
	}
	keys := make([]string, 0, len(out.SetupGuard))
	for k := range out.SetupGuard {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, fromCfg := LookupV7Setup(rc.SetupGuard, k)
		g := out.SetupGuard[k]
		rs.add("setup_guard", "setup_guard."+k, fmt.Sprintf("min_zone_pos_short=%d max_zone_pos_long=%d require_oi_flush=%v", g.MinZonePosShort, g.MaxZonePosLong, g.RequireOIFlush),
			srcOf(fromCfg), nil, "", "trader execution guard; the router uses its own setup thresholds")
	}

	// ---- Daily-loss breaker ----
	out.DailyLossBreaker = EffectiveDailyLossBreaker(rc.DailyLossBreaker)
	dl := rc.DailyLossBreaker
	rs.add("breaker", "daily_loss_breaker.mode", out.DailyLossBreaker.Mode, srcOf(dl != nil && dl.Mode != ""), nil, "off | shadow | armed", "")
	rs.add("breaker", "daily_loss_breaker.max_daily_loss_pct", out.DailyLossBreaker.MaxDailyLossPct, srcOf(dl != nil && dl.MaxDailyLossPct > 0), nil, "", "")
	rs.add("breaker", "daily_loss_breaker.near_trip_fraction", out.DailyLossBreaker.NearTripFraction, srcOf(dl != nil && dl.NearTripFraction > 0 && dl.NearTripFraction < 1), nil, "0..1", "")

	// ---- Protector core (read through EffectiveProtector) ----
	p := EffectiveProtector(rc.Protector)
	pc := rc.Protector
	rs.add("protector", "protector.tp0", fmt.Sprintf("%g %s", p.TP0.Value, p.TP0.Unit), srcOf(pc != nil && pc.TP0.Unit != ""), nil, "", "")
	rs.add("protector", "protector.tp1", fmt.Sprintf("%g %s", p.TP1.Value, p.TP1.Unit), srcOf(pc != nil && pc.TP1.Unit != ""), nil, "", "")
	rs.add("protector", "protector.tp0_close_ratio", p.TP0CloseRatio, srcOf(pc != nil && pc.TP0CloseRatio > 0), nil, "0..1", "")
	rs.add("protector", "protector.tp1_close_ratio", p.TP1CloseRatio, srcOf(pc != nil && pc.TP1CloseRatio > 0), nil, "0..1", "")
	rs.add("protector", "protector.giveback_close_ratio", p.GivebackCloseRatio, srcOf(pc != nil && pc.GivebackCloseRatio > 0), nil, "0..1", "")
	rs.add("protector", "protector.breakeven_buffer_frac", p.BreakevenBufferFrac, srcOf(pc != nil && pc.BreakevenBufferFrac > 0), nil, "", "")

	// ---- §5.3 safety hard limits (code only) ----
	safety := func(key string, value interface{}, note string) {
		rs.add("safety_limit", key, value, SourceSafetyLimit, nil, "", note)
	}
	safety("stop_tp_invariants", "stop on the protective side; TP beyond entry; ClosePosition(true) stops", "§5.3 item 1")
	safety("stop_working_type", "CONTRACT_PRICE", "§5.3 item 2")
	safety("naked_stop_backstop", fmt.Sprintf("grace %ds; per-poll stop audit; emergency close %d attempts", SafetyNakedStopGraceSeconds, SafetyEmergencyCloseAttempts), "§5.3 item 3")
	safety("protector_hard_loss", fmt.Sprintf("%g %s market close", p.HardLoss.Value, p.HardLoss.Unit), "§5.3 item 4")
	safety("leverage_ceiling", SafetyMaxLeverage, "§5.3 item 5; 0 = no cap in the trader")
	safety("max_positions_ceiling", SafetyMaxPositionsCeiling, "§5.3 item 6; carried over from the pre-#82 bound, owner to confirm")
	safety("token_budget", fmt.Sprintf("MaxCandidateCoins %d; MaxTimeframes %d; kline count %d..%d", MaxCandidateCoins, MaxTimeframes, MinKlineCount, MaxKlineCount), "§5.3 item 7")
	safety("margin_factor", fmt.Sprintf("%.2f/leverage + %.3f", 1+SafetyMarginFactorBuffer, SafetyMarginFactorFee), "§5.3 item 8")
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ResolveMinConfidence is the single place that decides the effective
// open-confidence floor (#105 / AUD-B4): the configured min_confidence when
// set (> 0), else DefaultMinConfidenceFloor. configured reports which. The
// kernel validator, the kernel prompt and the trader all read it.
func ResolveMinConfidence(configured int) (floor int, isConfigured bool) {
	if configured > 0 {
		return configured, true
	}
	return DefaultMinConfidenceFloor, false
}

// ValidateTradingConfig fails a strategy save on out-of-range values
// (#95 §5.4.3: validate, do not clamp silently). The §5.3 safety limits are
// the only upper bounds. Messages name the field.
func ValidateTradingConfig(cfg *StrategyConfig) error {
	if cfg == nil {
		return nil
	}
	var errs []string
	add := func(format string, args ...interface{}) { errs = append(errs, fmt.Sprintf(format, args...)) }
	rc := cfg.RiskControl
	if rc.MaxPositions > SafetyMaxPositionsCeiling {
		add("risk_control.max_positions %d is above the safety ceiling %d", rc.MaxPositions, SafetyMaxPositionsCeiling)
	}
	if rc.MaxPositions < 0 {
		add("risk_control.max_positions must be >= 1")
	}
	for _, l := range []struct {
		name string
		v    int
	}{{"btc_eth_max_leverage", rc.BTCETHMaxLeverage}, {"altcoin_max_leverage", rc.AltcoinMaxLeverage}} {
		if l.v < 0 || l.v > SafetyMaxLeverage {
			add("risk_control.%s %d must be 0..%d", l.name, l.v, SafetyMaxLeverage)
		}
	}
	if rc.MinConfidence < 0 || rc.MinConfidence > 100 {
		add("risk_control.min_confidence %d must be 0..100", rc.MinConfidence)
	}
	if rc.MinRiskRewardRatio < 0 || rc.MinRiskRewardRatio > 10 {
		add("risk_control.min_risk_reward_ratio %.2f must be 0 (unset) or up to 10", rc.MinRiskRewardRatio)
	}
	if rc.MaxMarginUsage < 0 || rc.MaxMarginUsage > 1 {
		add("risk_control.max_margin_usage %.2f must be 0..1", rc.MaxMarginUsage)
	}
	for _, f := range []struct {
		name string
		v    float64
		max  float64
	}{
		{"max_entry_price_deviation_pct", rc.MaxEntryPriceDeviationPct, 10},
		{"max_take_profit_price_move_pct", rc.MaxTakeProfitPriceMovePct, 100},
		{"min_stop_loss_price_move_pct", rc.MinStopLossPriceMovePct, 100},
		{"max_single_trade_loss_pct", rc.MaxSingleTradeLossPct, 100},
		{"btc_eth_max_position_value_ratio", rc.BTCETHMaxPositionValueRatio, 100},
		{"altcoin_max_position_value_ratio", rc.AltcoinMaxPositionValueRatio, 100},
	} {
		if f.v < 0 || f.v > f.max || math.IsNaN(f.v) {
			add("risk_control.%s %.4g must be 0 (unset) to %g", f.name, f.v, f.max)
		}
	}
	if rc.MinPositionSize < 0 {
		add("risk_control.min_position_size must be >= 0")
	}
	switch strings.ToLower(strings.TrimSpace(rc.GeometryRepair)) {
	case "", "repair", "reject":
	default:
		add("risk_control.geometry_repair %q must be repair or reject", rc.GeometryRepair)
	}
	if b := rc.DailyLossBreaker; b != nil {
		switch b.Mode {
		case "", DailyLossBreakerOff, DailyLossBreakerShadow, DailyLossBreakerArmed:
		default:
			add("risk_control.daily_loss_breaker.mode %q must be off, shadow or armed", b.Mode)
		}
		if b.MaxDailyLossPct < 0 || b.MaxDailyLossPct > 100 {
			add("risk_control.daily_loss_breaker.max_daily_loss_pct must be 0 (unset) to 100")
		}
		if b.NearTripFraction < 0 || b.NearTripFraction >= 1 {
			add("risk_control.daily_loss_breaker.near_trip_fraction must be 0 (unset) to below 1")
		}
	}
	if c := rc.Cooldowns; c != nil {
		if c.FailedOpenCycles != nil && *c.FailedOpenCycles < 0 {
			add("risk_control.cooldowns.failed_open_cycles must be >= 0 (0 = off)")
		}
		if r := c.RecentLoss; r != nil {
			if r.WindowMinutes != nil && *r.WindowMinutes < 0 {
				add("risk_control.cooldowns.recent_loss.window_minutes must be >= 0 (0 = off)")
			}
			if r.MinLosses != nil && *r.MinLosses < 1 {
				add("risk_control.cooldowns.recent_loss.min_losses must be >= 1")
			}
			if r.LossPct != nil && *r.LossPct <= 0 {
				add("risk_control.cooldowns.recent_loss.loss_pct must be > 0")
			}
		}
	}
	if p := rc.Protector; p != nil {
		for _, f := range []struct {
			name string
			v    float64
		}{{"tp0_close_ratio", p.TP0CloseRatio}, {"tp1_close_ratio", p.TP1CloseRatio}, {"tp2_close_ratio", p.TP2CloseRatio}, {"giveback_close_ratio", p.GivebackCloseRatio}} {
			if f.v < 0 || f.v > 1 {
				add("risk_control.protector.%s must be 0 (unset) to 1", f.name)
			}
		}
		for _, l := range []struct {
			name string
			lv   ProtectorLevel
		}{{"tp0", p.TP0}, {"tp1", p.TP1}} {
			if l.lv.Unit != "" && !ValidProtectorUnit(l.lv.Unit) {
				add("risk_control.protector.%s.unit %q is not a level unit", l.name, l.lv.Unit)
			}
		}
	}
	if h := cfg.CoinSource.Hunter; h != nil {
		if h.V7MinOutput != nil && *h.V7MinOutput < 1 {
			add("coin_source.hunter_config.v7_min_output must be >= 1")
		}
		if h.V7CorrelationMaxPerTheme != nil && *h.V7CorrelationMaxPerTheme < 0 {
			add("coin_source.hunter_config.v7_correlation_max_per_theme must be >= 0 (0 = off)")
		}
		if h.V7FallbackMinAIPriority != nil && (*h.V7FallbackMinAIPriority < 0 || *h.V7FallbackMinAIPriority > 100) {
			add("coin_source.hunter_config.v7_fallback_min_ai_priority must be 0..100")
		}
		if h.V7MinAIPriority < 0 || h.V7MinAIPriority > 100 {
			add("coin_source.hunter_config.v7_min_ai_priority must be 0..100")
		}
	}
	if cfg.CoinSource.HunterLimit < 0 {
		add("coin_source.hunter_limit must be >= 0")
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.New(strings.Join(errs, "; "))
}
