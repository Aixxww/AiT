package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Aixxww/AiT/kernel"
	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// Fix 15 (P2-15): the flags and settings that actually governed a round,
// as resolved at runtime (process flags, the strategy row loaded for the
// round, and the kernel / module fallbacks applied on top), so a report can
// be audited against what was really in effect. Record only: everything
// here is computed from the same values and functions the round used, and
// nothing reads it back.

type harnessFlags struct {
	StrategyID           string  `json:"strategy_id"`
	DB                   string  `json:"db"`
	TopDetail            int     `json:"top_detail"`
	MaxWorkers           int     `json:"max_workers"`
	CLIMinPriority       float64 `json:"cli_min_priority"` // the strategy's value wins when set (see router)
	CLIMaxOutput         int     `json:"cli_max_output"`
	CLIWatchOutput       int     `json:"cli_watch_output"`
	CLIAggressive        bool    `json:"cli_aggressive"`
	Rounds               int     `json:"rounds"`
	RoundInterval        string  `json:"round_interval"`
	PersistSignals       bool    `json:"persist_signals"`
	TrackOutcomes        bool    `json:"track_outcomes"`
	PostTrackDuration    string  `json:"post_track_duration"`
	TrackActiveOnly      bool    `json:"track_active_only"`
	BinanceURL           string  `json:"binance_url"` // "" = https://fapi.binance.com
	FetchBudget          string  `json:"fetch_budget"`
	MaxDetailSkipPct     float64 `json:"max_detail_skip_pct"`
	PromptReprice        bool    `json:"prompt_reprice"`
	PromptPriceMaxAge    string  `json:"prompt_price_max_age"`
	TwoTier              bool    `json:"two_tier"`
	SlowEvery            int     `json:"slow_every,omitempty"`
	WeightLimitPerMin    float64 `json:"weight_limit_per_min,omitempty"`
	FuturesDataLimit5Min int     `json:"futures_data_limit_per_5min,omitempty"`
	RunID                string  `json:"run_id,omitempty"`
	Resume               bool    `json:"resume"`
	FeeNetCostPct        float64 `json:"fee_net_cost_pct,omitempty"` // the outcome tracker's (set once at start); 0 = tracking off
}

type strategyResolution struct {
	Loaded                  bool     `json:"loaded"` // false = no --strategy-id: harness defaults
	ConfiguredMinRR         float64  `json:"configured_min_risk_reward_ratio"`
	NativeMinRRSource       string   `json:"native_min_rr_source"`
	ConfiguredT2MinRR       float64  `json:"configured_t2_min_risk_reward_ratio"`
	T2Enabled               bool     `json:"v7_t2_enabled"`
	ConfiguredT2Setups      []string `json:"configured_v7_t2_setups"`
	IncludeNonCryptoFutures bool     `json:"include_non_crypto_futures"`
}

type routerFlags struct {
	MaxOutput              int     `json:"max_output"`
	MinOutput              int     `json:"min_output"`
	WatchOutput            int     `json:"watch_output"`
	MinAIPriority          float64 `json:"min_ai_priority"`
	FallbackMinAIPriority  float64 `json:"fallback_min_ai_priority"`
	CorrelationMaxPerTheme int     `json:"correlation_max_per_theme"`
	Aggressive             bool    `json:"aggressive"`
	ClosedBarFeatures      bool    `json:"closed_bar_features"`
}

type finalConflictFlags struct {
	Enabled          bool    `json:"enabled"`
	MinAIPriority    float64 `json:"min_ai_priority"`
	TieWinner        string  `json:"tie_winner"`
	ContextNonVoting bool    `json:"context_non_voting"`
}

// effectiveFlags is one round's resolved settings.
type effectiveFlags struct {
	Harness       harnessFlags                `json:"harness"`
	Strategy      strategyResolution          `json:"strategy"`
	Router        routerFlags                 `json:"router"`
	T2            local.V7T2EffectiveSettings `json:"t2_module"`
	FinalConflict finalConflictFlags          `json:"final_conflict"`
	// AssemblyGeometry decided the assembled tier (DB execution_tier);
	// PromptGeometry the prompt-final tier (what the LLM saw). They come
	// from different builders and can differ (see Notes).
	AssemblyGeometry     kernel.HunterV7EffectiveGeometry `json:"assembly_geometry"`
	PromptGeometry       kernel.HunterV7EffectiveGeometry `json:"prompt_geometry"`
	PromptPayloadVersion string                           `json:"prompt_payload_version"`
	DoctrineVersion      string                           `json:"doctrine_version"`
	Notes                []string                         `json:"notes,omitempty"`
}

func optsHarnessFlags(opts validationOptions) harnessFlags {
	h := harnessFlags{
		StrategyID: opts.strategyID, DB: opts.dbPath, TopDetail: opts.topDetail, MaxWorkers: opts.maxWorkers,
		CLIMinPriority: opts.minPriority, CLIMaxOutput: opts.maxOutput, CLIWatchOutput: opts.watchOutput, CLIAggressive: opts.aggressive,
		Rounds: opts.rounds, RoundInterval: opts.roundInterval.String(), PersistSignals: opts.persistSignals,
		TrackOutcomes: opts.trackOutcomes && opts.outcomeTracker != nil, PostTrackDuration: opts.postTrackDuration.String(),
		TrackActiveOnly: opts.trackActiveOnly, BinanceURL: opts.binanceURL, FetchBudget: roundFetchBudget(opts).String(),
		MaxDetailSkipPct: opts.maxDetailSkipPct, PromptReprice: opts.promptReprice, PromptPriceMaxAge: opts.promptPriceMaxAge.String(),
		TwoTier: opts.twoTier, RunID: opts.runID, Resume: opts.resume,
	}
	if opts.twoTier {
		h.SlowEvery, h.WeightLimitPerMin, h.FuturesDataLimit5Min = opts.slowEvery, opts.weightLimitPerMin, opts.futuresDataLimitPer5Min
	}
	if h.TrackOutcomes {
		h.FeeNetCostPct = opts.feeNetCostPct
	}
	return h
}

// buildEffectiveFlags resolves one round. cfg is the router config the
// round scored with, assembly the geometry it assembled with, promptCfg the
// strategy config its prompt was built with.
func buildEffectiveFlags(opts validationOptions, strategyCfg *store.StrategyConfig, cfg local.V7Config,
	assembly kernel.HunterV7ExecutionGeometry, promptCfg store.StrategyConfig) effectiveFlags {
	ef := effectiveFlags{
		Harness: optsHarnessFlags(opts),
		Router: routerFlags{MaxOutput: cfg.MaxOutput, MinOutput: cfg.MinOutput, WatchOutput: cfg.WatchOutput, MinAIPriority: cfg.MinAIPriority,
			FallbackMinAIPriority: cfg.FallbackMinAIPriority, CorrelationMaxPerTheme: cfg.CorrelationMaxPerTheme,
			Aggressive: cfg.Aggressive, ClosedBarFeatures: cfg.ClosedBarFeatures},
		T2: cfg.T2.EffectiveSettings(),
		FinalConflict: finalConflictFlags{Enabled: cfg.FinalConflict.Enabled, MinAIPriority: cfg.FinalConflict.MinAIPriority,
			TieWinner: string(cfg.FinalConflict.TieWinner), ContextNonVoting: cfg.FinalConflict.ContextNonVoting},
		AssemblyGeometry: kernel.HunterV7EffectiveGeometryOf(assembly),
		PromptGeometry:   kernel.HunterV7EffectiveGeometryOf(kernel.HunterV7ExecutionGeometryForConfig(&promptCfg)),
	}
	var pc *store.V7PromptConfig
	if promptCfg.CoinSource.Hunter != nil {
		pc = promptCfg.CoinSource.Hunter.V7Prompt
	}
	ep := store.EffectiveV7Prompt(pc)
	ef.PromptPayloadVersion, ef.DoctrineVersion = ep.PayloadVersion, ep.DoctrineVersion()

	s := &ef.Strategy
	s.ConfiguredT2Setups = []string{}
	if strategyCfg != nil {
		s.Loaded = true
		s.ConfiguredMinRR = strategyCfg.RiskControl.MinRiskRewardRatio
		s.ConfiguredT2MinRR = strategyCfg.RiskControl.T2MinRiskRewardRatio
		if h := strategyCfg.CoinSource.Hunter; h != nil {
			s.T2Enabled, s.IncludeNonCryptoFutures = h.V7T2Enabled, h.IncludeNonCryptoFutures
			s.ConfiguredT2Setups = append(s.ConfiguredT2Setups, h.V7T2Setups...)
		}
	}
	switch {
	case !s.Loaded:
		s.NativeMinRRSource = fmt.Sprintf("no strategy: default strategy config %.2f (store.GetDefaultStrategyConfig)", store.GetDefaultStrategyConfig("zh").RiskControl.MinRiskRewardRatio)
	case s.ConfiguredMinRR > 0:
		s.NativeMinRRSource = "strategy min_risk_reward_ratio"
	default:
		s.NativeMinRRSource = fmt.Sprintf("unset: store default %.2f (store.DefaultMinRiskRewardRatio)", store.DefaultMinRiskRewardRatio)
	}
	ef.Notes = effectiveFlagNotes(ef)
	return ef
}

// effectiveFlagNotes states, in words, where the effective values differ
// from what a reader might assume from code defaults or the stored row.
func effectiveFlagNotes(ef effectiveFlags) []string {
	var notes []string
	a, p := ef.AssemblyGeometry, ef.PromptGeometry
	if a.NativeMinRR != p.NativeMinRR || a.T2MinRR != p.T2MinRR || a.MaxTPMovePct != p.MaxTPMovePct || a.T2MinSLMovePct != p.T2MinSLMovePct {
		notes = append(notes, fmt.Sprintf("assembly and prompt geometries differ: native min RR %.2f vs %.2f, T2 min RR %.2f vs %.2f, TP cap %.2f%% vs %.2f%%, T2 min stop %.2f%% vs %.2f%% (assembled tiers use the first, prompt-final tiers the second)",
			a.NativeMinRR, p.NativeMinRR, a.T2MinRR, p.T2MinRR, a.MaxTPMovePct, p.MaxTPMovePct, a.T2MinSLMovePct, p.T2MinSLMovePct))
	}
	if ef.Strategy.ConfiguredT2MinRR > 0 && ef.Strategy.ConfiguredT2MinRR != a.T2MinRR {
		notes = append(notes, fmt.Sprintf("the strategy sets t2_min_risk_reward_ratio %.2f, but the assembly used %.2f",
			ef.Strategy.ConfiguredT2MinRR, a.T2MinRR))
	}
	if ef.Strategy.Loaded && ef.Strategy.ConfiguredMinRR != store.GetDefaultStrategyConfig("en").RiskControl.MinRiskRewardRatio && ef.Strategy.ConfiguredMinRR > 0 {
		notes = append(notes, fmt.Sprintf("the stored strategy's min_risk_reward_ratio (%.2f) differs from the code default (%.2f); the stored value was in effect",
			ef.Strategy.ConfiguredMinRR, store.GetDefaultStrategyConfig("en").RiskControl.MinRiskRewardRatio))
	}
	if !ef.T2.Enabled {
		notes = append(notes, "the T2 module was off for this round (v7_t2_enabled false or no strategy)")
	} else if len(ef.T2.UnknownConfiguredSetups) > 0 {
		notes = append(notes, fmt.Sprintf("configured v7_t2_setups contains names the module does not know: %v", ef.T2.UnknownConfiguredSetups))
	}
	return notes
}

// flagsFingerprint is a comparable form of the round-resolved settings
// (everything except the process flags, which are the same in one run).
func flagsFingerprint(ef effectiveFlags) string {
	ef.Harness = harnessFlags{}
	b, _ := json.Marshal(ef)
	return string(b)
}

// runEffectiveFlags is the run summary's record.
type runEffectiveFlags struct {
	// FromRound is the round whose resolved settings are shown.
	FromRound int `json:"from_round"`
	effectiveFlags
	// RoundsWithDifferentSettings lists rounds whose resolved settings
	// differ from FromRound's (the strategy row is re-read every round).
	RoundsWithDifferentSettings []int `json:"rounds_with_different_settings,omitempty"`
}

func buildRunEffectiveFlags(opts validationOptions, reports []validationReport) *runEffectiveFlags {
	if len(reports) == 0 {
		return &runEffectiveFlags{effectiveFlags: effectiveFlags{Harness: optsHarnessFlags(opts),
			Notes: []string{"no accepted round in this process: only the process flags are known"}}}
	}
	first := reports[0]
	out := &runEffectiveFlags{FromRound: first.Round, effectiveFlags: first.EffectiveFlags}
	out.Harness = optsHarnessFlags(opts)
	fp := flagsFingerprint(first.EffectiveFlags)
	for _, r := range reports[1:] {
		if flagsFingerprint(r.EffectiveFlags) != fp {
			out.RoundsWithDifferentSettings = append(out.RoundsWithDifferentSettings, r.Round)
		}
	}
	return out
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// effectiveFlagsMarkdown is the "Effective settings" section.
func effectiveFlagsMarkdown(ef effectiveFlags, title string) string {
	var sb strings.Builder
	sb.WriteString(title)
	h, s, a, p := ef.Harness, ef.Strategy, ef.AssemblyGeometry, ef.PromptGeometry
	strategy := "none (harness defaults)"
	if s.Loaded {
		strategy = "`" + h.StrategyID + "`"
	}
	sb.WriteString("| Setting | In effect |\n|---|---|\n")
	row := func(k, v string) { sb.WriteString(fmt.Sprintf("| %s | %s |\n", k, v)) }
	row("strategy", strategy)
	row("native RR floor (assembly)", fmt.Sprintf("%.2f (%s; stored value %.2f)", a.NativeMinRR, s.NativeMinRRSource, s.ConfiguredMinRR))
	row("native RR floor (prompt)", fmt.Sprintf("%.2f", p.NativeMinRR))
	row("T2 RR floor (assembly / prompt)", fmt.Sprintf("%.2f / %.2f (stored %.2f; 0 = default)", a.T2MinRR, p.T2MinRR, s.ConfiguredT2MinRR))
	row("min stop % native / T2 (assembly)", fmt.Sprintf("%.2f / %.2f", a.NativeMinSLMovePct, a.T2MinSLMovePct))
	row("TP1 cap % (assembly / prompt)", fmt.Sprintf("%.2f / %.2f", a.MaxTPMovePct, p.MaxTPMovePct))
	row("max entry drift %", fmt.Sprintf("%.2f", a.MaxDriftPct))
	row("shadow-only setups (capped at WATCH)", fmt.Sprintf("%v", a.ShadowOnlySetups))
	row("conditional entry classes", fmt.Sprintf("%v", a.ConditionalClasses))
	row("T2 module", onOff(ef.T2.Enabled))
	if ef.T2.Enabled {
		row("T2 enabled setups", fmt.Sprintf("%d (%s): %v", len(ef.T2.EnabledSetups), ef.T2.EnabledSetupsFrom, ef.T2.EnabledSetups))
		row("T2 disabled setups", fmt.Sprintf("%v", ef.T2.DisabledSetups))
		row("T2 observe-only setups", fmt.Sprintf("%v (%s)", ef.T2.ObserveOnlySetups, ef.T2.ObserveOnlyFrom))
		row("T2 lifecycle", fmt.Sprintf("ARMED >= %.0f, watch promote >= %.0f, expiry ARMED %.1fh / WATCH %.1fh, cooldown %.0f min, trigger %s, TRIGGERED surfaced %.0f min",
			ef.T2.MinAlertScore, ef.T2.MinWatchScore, ef.T2.ArmedExpiryHours, ef.T2.WatchExpiryHours, ef.T2.CooldownMinutes, onOff(ef.T2.TriggerEnabled), ef.T2.TriggeredSurfaceMinutes))
		row("T2 symbol gate / setup prefix", fmt.Sprintf("%s / %s", onOff(ef.T2.SymbolValidityGate), onOff(ef.T2.SetupPrefix)))
	}
	row("final conflict pass", fmt.Sprintf("%s (min priority %.0f, tie winner %s, CONTEXT non-voting %v)", onOff(ef.FinalConflict.Enabled),
		ef.FinalConflict.MinAIPriority, ef.FinalConflict.TieWinner, ef.FinalConflict.ContextNonVoting))
	row("router", fmt.Sprintf("min priority %.0f, max output %d, watch output %d, aggressive %v, closed-bar features %v",
		ef.Router.MinAIPriority, ef.Router.MaxOutput, ef.Router.WatchOutput, ef.Router.Aggressive, ef.Router.ClosedBarFeatures))
	row("prompt payload / doctrine", ef.PromptPayloadVersion+" / "+ef.DoctrineVersion)
	row("fetch", fmt.Sprintf("two-tier %s (slow every %d), top detail %d, workers %d, budget %s, max detail skip %.0f%%, reprice %s (max age %s)",
		onOff(h.TwoTier), h.SlowEvery, h.TopDetail, h.MaxWorkers, h.FetchBudget, h.MaxDetailSkipPct, onOff(h.PromptReprice), h.PromptPriceMaxAge))
	row("outcome tracking", fmt.Sprintf("%s (fee-net cost %.4f, post-track %s)", onOff(h.TrackOutcomes), h.FeeNetCostPct, h.PostTrackDuration))
	sb.WriteString("\n")
	if len(ef.Notes) > 0 {
		notes := append([]string(nil), ef.Notes...)
		sort.Strings(notes)
		for _, n := range notes {
			sb.WriteString("> NOTE: " + n + "\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
