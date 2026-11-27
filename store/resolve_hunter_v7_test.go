package store

import (
	"errors"
	"strings"
	"testing"
)

func ip(v int) *int { return &v }

func mustValue(t *testing.T, r ResolvedHunterV7, key string) ResolvedValue {
	t.Helper()
	v, ok := r.Value(key)
	if !ok {
		t.Fatalf("no resolved value %q", key)
	}
	return v
}

// #95: every effective value carries its source: config / default /
// safety_limit.
func TestResolveHunterV7Sources(t *testing.T) {
	cfg := &StrategyConfig{}
	cfg.CoinSource.SourceType = "hunter_v7"
	cfg.RiskControl.MaxPositions = 1
	cfg.RiskControl.MinConfidence = 75
	cfg.RiskControl.MinRiskRewardRatio = 1.2
	cfg.CoinSource.Hunter = &HunterConfig{V7MaxOutput: 30, V7MinAIPriority: 55}
	cfg.CoinSource.HunterLimit = 15

	r := ResolveHunterV7(cfg)

	// config
	if r.MaxPositions != 1 || r.MaxPositionsErr != nil || mustValue(t, r, "max_positions").Source != SourceConfig {
		t.Fatalf("max_positions: %d %v", r.MaxPositions, r.MaxPositionsErr)
	}
	if r.MinConfidence != 75 || mustValue(t, r, "min_confidence").Source != SourceConfig {
		t.Fatalf("min_confidence %d", r.MinConfidence)
	}
	if r.MinRR != 1.2 || mustValue(t, r, "min_risk_reward_ratio").Source != SourceConfig {
		t.Fatalf("min RR %v", r.MinRR)
	}
	// default
	if r.MaxEntryDriftPct != 0.5 || mustValue(t, r, "max_entry_price_deviation_pct").Source != SourceDefault {
		t.Fatalf("drift %v", r.MaxEntryDriftPct)
	}
	if r.MaxSingleTradeLossPct != 8 || r.MinPositionSizeUSD != 12 || r.BTCETHPositionValueRatio != 5 || r.AltcoinPositionValueRatio != 1 {
		t.Fatalf("sizing defaults: %+v", r)
	}
	if r.FailedOpenCooldownCycles != 3 || !r.RecentLossCooldown.Enabled || r.RecentLossCooldown.WindowMinutes != 60 ||
		r.RecentLossCooldown.MinLosses != 2 || r.RecentLossCooldown.LossPct != 5 {
		t.Fatalf("cooldown defaults: %d %+v", r.FailedOpenCooldownCycles, r.RecentLossCooldown)
	}
	if r.MinOutput != 3 || r.FallbackMinAIPriority != 45 || r.CorrelationMaxPerTheme != 3 || r.WatchOutput != 5 {
		t.Fatalf("router defaults: %+v", r)
	}
	// safety_limit: hunter_limit 15 -> 10, LLM count 10
	if v := mustValue(t, r, "hunter_limit"); v.Source != SourceSafetyLimit || v.Value != 10 || v.Configured != 15 {
		t.Fatalf("hunter_limit: %+v", v)
	}
	if r.MaxOutput != 10 || mustValue(t, r, "llm_candidate_count").Source != SourceSafetyLimit {
		t.Fatalf("llm count %d %+v", r.MaxOutput, mustValue(t, r, "llm_candidate_count"))
	}
	for _, k := range []string{"stop_tp_invariants", "stop_working_type", "naked_stop_backstop", "protector_hard_loss",
		"leverage_ceiling", "max_positions_ceiling", "token_budget", "margin_factor"} {
		if mustValue(t, r, k).Source != SourceSafetyLimit {
			t.Fatalf("%s must be a safety_limit", k)
		}
	}
	// max_margin_usage is enforced by the trader's pre-open margin gate
	// (#159); it must not be marked not-enforced.
	if v := mustValue(t, r, "max_margin_usage"); v.Enforced != nil && !*v.Enforced {
		t.Fatalf("max_margin_usage must not be marked not enforced: %+v", v)
	}
}

// Effective values are unchanged by the resolver: LLM count =
// min(v7_max_output, hunter_limit), capped at 10.
func TestResolveHunterV7CandidateCount(t *testing.T) {
	for _, tc := range []struct {
		maxOut, limit, want int
		src                 ConfigSource
	}{
		{0, 0, 10, SourceDefault},       // limit unset -> ClampLimits default 10
		{0, 5, 5, SourceConfig},         // limit 5
		{8, 0, 8, SourceConfig},         // v7_max_output 8
		{8, 5, 5, SourceConfig},         // min
		{30, 12, 10, SourceSafetyLimit}, // both above the cap
	} {
		cfg := &StrategyConfig{}
		cfg.CoinSource.SourceType = "hunter_v7"
		cfg.CoinSource.HunterLimit = tc.limit
		cfg.CoinSource.Hunter = &HunterConfig{V7MaxOutput: tc.maxOut}
		r := ResolveHunterV7(cfg)
		if r.MaxOutput != tc.want || mustValue(t, r, "llm_candidate_count").Source != tc.src {
			t.Errorf("max_output %d limit %d: got %d (%s), want %d (%s)", tc.maxOut, tc.limit, r.MaxOutput,
				mustValue(t, r, "llm_candidate_count").Source, tc.want, tc.src)
		}
	}
}

// Unset vs off (§5.4.2): nil = default, 0 = off.
func TestResolveHunterV7UnsetVsOff(t *testing.T) {
	cfg := &StrategyConfig{}
	cfg.CoinSource.SourceType = "hunter_v7"
	cfg.RiskControl.MaxPositions = 1
	cfg.RiskControl.Cooldowns = &CooldownConfig{FailedOpenCycles: ip(0), RecentLoss: &RecentLossCooldownConfig{WindowMinutes: ip(0)}}
	cfg.CoinSource.Hunter = &HunterConfig{V7CorrelationMaxPerTheme: ip(0), V7MinOutput: ip(4)}
	r := ResolveHunterV7(cfg)
	if r.FailedOpenCooldownCycles != 0 || mustValue(t, r, "failed_open_cycles").Source != SourceConfig {
		t.Fatal("failed_open_cycles 0 must be off")
	}
	if r.RecentLossCooldown.Enabled {
		t.Fatal("recent-loss window 0 must be off")
	}
	if r.CorrelationMaxPerTheme != -1 {
		t.Fatalf("correlation 0 must map to the router's off (-1), got %d", r.CorrelationMaxPerTheme)
	}
	if r.MinOutput != 4 {
		t.Fatalf("v7_min_output %d", r.MinOutput)
	}
}

// RR unset resolves to the documented 1.2, never 3.
func TestResolveHunterV7UnsetRRIsDefault(t *testing.T) {
	r := ResolveHunterV7(&StrategyConfig{})
	if r.MinRR != 1.2 || mustValue(t, r, "min_risk_reward_ratio").Source != SourceDefault {
		t.Fatalf("unset RR = %v", r.MinRR)
	}
	if r.MinConfidence != 60 || mustValue(t, r, "min_confidence").Source != SourceDefault {
		t.Fatalf("unset confidence = %d", r.MinConfidence)
	}
}

// Setup guards: the strategy's entry overrides the built-in default.
func TestResolveHunterV7SetupGuard(t *testing.T) {
	cfg := &StrategyConfig{}
	cfg.RiskControl.SetupGuard = map[string]SetupGuardConfig{"range_reversion": {MinZonePosShort: 70, MaxZonePosLong: 30}}
	r := ResolveHunterV7(cfg)
	if g, ok := r.SetupGuardFor("range_reversion"); !ok || g.MinZonePosShort != 70 {
		t.Fatalf("override: %+v", g)
	}
	if g, ok := r.SetupGuardFor("funding_reversal"); !ok || !g.RequireOIFlush || g.MaxZonePosLong != 35 {
		t.Fatalf("builtin: %+v", g)
	}
	if _, ok := r.SetupGuardFor("unknown_setup"); ok {
		t.Fatal("unknown setup has no guard")
	}
	if mustValue(t, r, "setup_guard.range_reversion").Source != SourceConfig || mustValue(t, r, "setup_guard.funding_reversal").Source != SourceDefault {
		t.Fatal("setup guard sources")
	}
}

// §5.3 item 6: max positions above the distinct safety ceiling fails loudly.
func TestMaxPositionsSafetyCeiling(t *testing.T) {
	rc := RiskControlConfig{MaxPositions: 4}
	if _, err := rc.ValidateMaxPositions(); !errors.Is(err, ErrMaxPositionsAboveSafetyCeiling) {
		t.Fatalf("4 > ceiling 3 must fail, got %v", err)
	}
	r := ResolveHunterV7(&StrategyConfig{RiskControl: rc})
	if v := mustValue(t, r, "max_positions"); v.Source != SourceSafetyLimit || !strings.Contains(v.Note, "opens blocked") {
		t.Fatalf("resolved max_positions: %+v", v)
	}
	rc.MaxPositions = 3
	if n, err := rc.ValidateMaxPositions(); err != nil || n != 3 {
		t.Fatalf("3: %d %v", n, err)
	}
}

// §5.4.3: out-of-range values fail the save with a message.
func TestValidateTradingConfig(t *testing.T) {
	ok := GetDefaultStrategyConfig("en")
	if err := ValidateTradingConfig(&ok); err != nil {
		t.Fatalf("the default config must validate: %v", err)
	}
	for name, mutate := range map[string]func(c *StrategyConfig){
		"max_positions":         func(c *StrategyConfig) { c.RiskControl.MaxPositions = 5 },
		"btc_eth_max_leverage":  func(c *StrategyConfig) { c.RiskControl.BTCETHMaxLeverage = 25 },
		"min_confidence":        func(c *StrategyConfig) { c.RiskControl.MinConfidence = 120 },
		"min_risk_reward_ratio": func(c *StrategyConfig) { c.RiskControl.MinRiskRewardRatio = -1 },
		"geometry_repair":       func(c *StrategyConfig) { c.RiskControl.GeometryRepair = "fix" },
		"max_margin_usage":      func(c *StrategyConfig) { c.RiskControl.MaxMarginUsage = 1.5 },
		"daily_loss_breaker.mode": func(c *StrategyConfig) {
			c.RiskControl.DailyLossBreaker = &DailyLossBreakerConfig{Mode: "on"}
		},
		"failed_open_cycles": func(c *StrategyConfig) {
			c.RiskControl.Cooldowns = &CooldownConfig{FailedOpenCycles: ip(-1)}
		},
		"tp0_close_ratio": func(c *StrategyConfig) { c.RiskControl.Protector = &ProtectorConfig{TP0CloseRatio: 1.5} },
		"v7_min_output": func(c *StrategyConfig) {
			c.CoinSource.Hunter = &HunterConfig{V7MinOutput: ip(0)}
		},
		"v7_fallback_min_ai_priority": func(c *StrategyConfig) {
			c.CoinSource.Hunter = &HunterConfig{V7FallbackMinAIPriority: f64p(150)}
		},
	} {
		c := GetDefaultStrategyConfig("en")
		mutate(&c)
		err := ValidateTradingConfig(&c)
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: want a validation error naming the field, got %v", name, err)
		}
	}
}
func f64p(v float64) *float64 { return &v }
