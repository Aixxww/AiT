package kernel

import (
	"testing"

	"github.com/Aixxww/AiT/provider/local"
)

// The shadow harness that guarded the U3.3 migrations was retired in U3.4
// after every setup landed with diff=0 and a live validation round matched
// the pre-refactor baseline. These tests keep the evaluator semantics and
// registry shape pinned.

func TestHunterV7TierRuleMatcherSemantics(t *testing.T) {
	base := CandidateCoin{
		Direction:          "LONG",
		V7SetupType:        "trend_breakout_long",
		V7ExecutionQuality: "ready",
		V7AIPriority:       60,
		V7SetupScore:       60,
		V7TimingScore:      60,
		V7RiskScore:        45,
		V7LiquidityScore:   60,
		V7ReasonCodes:      []string{"confirmed_breakout", "clear_air_above"},
	}

	tests := []struct {
		name string
		coin func(CandidateCoin) CandidateCoin
		rule hunterV7TierRule
		want bool
	}{
		{
			name: "risk below is exclusive",
			coin: func(c CandidateCoin) CandidateCoin { c.V7RiskScore = 45; return c },
			rule: hunterV7TierRule{RiskBelow: 45},
			want: false,
		},
		{
			name: "risk at most is inclusive",
			coin: func(c CandidateCoin) CandidateCoin { c.V7RiskScore = 45; return c },
			rule: hunterV7TierRule{RiskAtMost: 45},
			want: true,
		},
		{
			name: "unknown liquidity passes the floor",
			coin: func(c CandidateCoin) CandidateCoin { c.V7LiquidityScore = 0; return c },
			rule: hunterV7TierRule{MinLiquidity: 70},
			want: true,
		},
		{
			name: "known liquidity below the floor fails",
			coin: func(c CandidateCoin) CandidateCoin { c.V7LiquidityScore = 69; return c },
			rule: hunterV7TierRule{MinLiquidity: 70},
			want: false,
		},
		{
			name: "at_least passes when taker data is missing",
			coin: func(c CandidateCoin) CandidateCoin { c.V7DerivativesCtx = nil; return c },
			rule: hunterV7TierRule{Taker: hunterV7TakerGate{Kind: "at_least", Threshold: 0.52}},
			want: true,
		},
		{
			name: "confirmed_at_least fails when taker data is missing",
			coin: func(c CandidateCoin) CandidateCoin { c.V7DerivativesCtx = nil; return c },
			rule: hunterV7TierRule{Taker: hunterV7TakerGate{Kind: "confirmed_at_least", Threshold: 0.52}},
			want: false,
		},
		{
			name: "boundary taker value passes at_least",
			coin: func(c CandidateCoin) CandidateCoin {
				c.V7DerivativesCtx = &local.V7DerivativesContext{TakerBuy15m: 0.52}
				return c
			},
			rule: hunterV7TierRule{Taker: hunterV7TakerGate{Kind: "at_least", Threshold: 0.52}},
			want: true,
		},
		{
			name: "require any needs one code from each group",
			coin: func(c CandidateCoin) CandidateCoin { return c },
			rule: hunterV7TierRule{RequireAny: [][]string{
				{"confirmed_breakout", "breakout_attempt"},
				{"volume_adequate", "oi_increasing"},
			}},
			want: false,
		},
		{
			name: "forbid blocks on any present code",
			coin: func(c CandidateCoin) CandidateCoin { return c },
			rule: hunterV7TierRule{ForbidAll: []string{"clear_air_above"}},
			want: false,
		},
		{
			name: "direction match is case insensitive",
			coin: func(c CandidateCoin) CandidateCoin { c.Direction = "long"; return c },
			rule: hunterV7TierRule{Direction: "LONG"},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coin := tt.coin(base)
			if got := hunterV7TierRuleMatches(coin, &tt.rule); got != tt.want {
				t.Fatalf("match = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHunterV7TierRuleReasonFuncFallsThrough(t *testing.T) {
	rules := []hunterV7TierRule{
		{
			MinAIPriority: 50,
			ReasonFunc: func(CandidateCoin) (bool, string) {
				return false, ""
			},
		},
		{MinAIPriority: 50, Reason: "static_fallback"},
	}
	ok, reason := hunterV7EvalTierRules(CandidateCoin{V7AIPriority: 60}, rules)
	if !ok || reason != "static_fallback" {
		t.Fatalf("eval = (%v, %q), want (true, static_fallback)", ok, reason)
	}
}

func TestHunterV7MMSLongLiveSupportGuardBlocksWeak5mRetest(t *testing.T) {
	coin := v7Candidate("mms_trend_ride_long",
		withDirection("LONG"),
		withQuality("ready"),
		withScores(80, 85, 70, 10, 90),
		withReasons("mms_trend_ride", "mms_ema_fan_bullish", "mms_ema25_retest_hold", "mms_low_volume_retest", "shape_clean_momentum", "entry_open_now"),
		withPriceCtx(&local.V7PriceContext{Last: 2.32}),
		withExecutionContext(&local.V7ExecutionContext{
			DataQuality: "complete_for_execution",
			Timeframes: map[string]local.V7ExecutionTimeframeSummary{
				"5m": {
					Timeframe:        "5m",
					CandleCount:      50,
					HasEMA20:         true,
					CloseVsEMA20Pct:  -0.18,
					HasVWAP20:        true,
					CloseVsVWAP20Pct: -0.26,
					VolumeVsAvg5:     0.52,
				},
			},
		}),
	)
	if hunterV7MMSLongLiveSupportOK(coin) {
		t.Fatalf("weak 5m retest should fail live support gate")
	}
}

func TestHunterV7MMSLongLiveSupportGuardAllowsHealthyRetest(t *testing.T) {
	coin := v7Candidate("mms_trend_ride_long",
		withDirection("LONG"),
		withQuality("ready"),
		withScores(80, 85, 70, 10, 90),
		withReasons("mms_trend_ride", "mms_ema_fan_bullish", "mms_ema25_retest_hold", "shape_clean_momentum", "entry_open_now"),
		withPriceCtx(&local.V7PriceContext{Last: 2.32}),
		withExecutionContext(&local.V7ExecutionContext{
			DataQuality: "complete_for_execution",
			Timeframes: map[string]local.V7ExecutionTimeframeSummary{
				"5m": {
					Timeframe:        "5m",
					CandleCount:      50,
					HasEMA20:         true,
					CloseVsEMA20Pct:  0.21,
					HasVWAP20:        true,
					CloseVsVWAP20Pct: 0.14,
					VolumeVsAvg5:     1.04,
				},
			},
		}),
	)
	if !hunterV7MMSLongLiveSupportOK(coin) {
		t.Fatalf("healthy 5m retest should pass live support gate")
	}
}

// TestHunterV7TierSpecShape validates registered specs: every rule must emit
// a reason or delegate to a reason func (OpenRateFloor rules excepted — the
// caller only consumes the match).
func TestHunterV7TierSpecShape(t *testing.T) {
	if len(hunterV7SetupTierSpecs) < 20 {
		t.Fatalf("registry unexpectedly small: %d setups", len(hunterV7SetupTierSpecs))
	}
	for setup, spec := range hunterV7SetupTierSpecs {
		for listName, rules := range map[string][]hunterV7TierRule{
			"Ready": spec.Ready, "NearConfirm": spec.NearConfirm, "Reviewable": spec.Reviewable,
		} {
			for i, rule := range rules {
				if rule.Reason == "" && rule.ReasonFunc == nil {
					t.Errorf("%s %s rule %d has neither reason nor reason func", setup, listName, i)
				}
			}
		}
	}
}
