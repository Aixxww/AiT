package kernel

import (
	"strings"
	"testing"

	"github.com/Aixxww/AiT/store"
)

// #77: min RR, max positions and min confidence come from the strategy
// config; code no longer overrides them. Owner values: RR 1.2, positions 1,
// confidence 75.

func ownerHunterV7Config() *store.StrategyConfig {
	cfg := store.GetDefaultStrategyConfig("en")
	cfg.CoinSource.SourceType = "hunter_v7"
	cfg.RiskControl.MinRiskRewardRatio = 1.2
	cfg.RiskControl.MaxPositions = 1
	cfg.RiskControl.MinConfidence = 75
	return &cfg
}

func TestConfiguredMinConfidenceIsEffective(t *testing.T) {
	e := NewStrategyEngine(ownerHunterV7Config())
	if got := e.effectiveMinOpenConfidence(75); got != 75 {
		t.Fatalf("effective min-open confidence %d, want 75 (no hunter_v7 cap at 70)", got)
	}
	p := e.BuildSystemPrompt(1000, "")
	for _, want := range []string{"Min Confidence: ≥75 to open position", "Confidence below 75 must output wait"} {
		if !strings.Contains(p, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
	if strings.Contains(p, "≥70 to open") {
		t.Error("system prompt still states 70")
	}
}

func TestKernelConfidenceFloorAndCapFollowConfig(t *testing.T) {
	mk := func(conf int) *Decision {
		return &Decision{Symbol: "XUSDT", Action: "open_long", Leverage: 5, PositionSizeUSD: 100, StopLoss: 98, TakeProfit: 103, Confidence: conf}
	}
	prices := map[string]float64{"XUSDT": 100}
	if err := validateDecision(mk(74), 1000, 20, 20, 10, 10, nil, prices, FixedDecisionRRFloor(1.2), 75); err == nil {
		t.Fatal("confidence 74 must fail a configured 75 floor")
	}
	if err := validateDecision(mk(75), 1000, 20, 20, 10, 10, nil, prices, FixedDecisionRRFloor(1.2), 75); err != nil {
		t.Fatalf("confidence 75 must pass: %v", err)
	}
	// Direction contradiction caps to the configured threshold, not 70.
	d := mk(90)
	hunter := map[string]HunterScoreInfo{"XUSDT": {LongScore: 20, ShortScore: 60}}
	if err := validateDecision(d, 1000, 20, 20, 10, 10, hunter, prices, FixedDecisionRRFloor(1.2), 75); err != nil {
		t.Fatal(err)
	}
	if d.Confidence != 75 {
		t.Fatalf("contradiction cap %d, want the configured 75", d.Confidence)
	}
}

func TestConfiguredMinRRIsTheBackendFloor(t *testing.T) {
	cfg := ownerHunterV7Config()
	g := HunterV7ExecutionGeometryForConfig(cfg)
	if g.MinRR != 1.2 {
		t.Fatalf("assembly floor %.2f, want the configured 1.2", g.MinRR)
	}
	// RR 1.3 (stop 2.5%, capped reward 3.25%) passes the configured 1.2;
	// before #77 the code floor of 1.5 rejected it as backend_rr_infeasible.
	coin := rrCoin("", 3.25)
	if r := hunterV7BackendCappedRRWaitReason(coin, g); r != "" {
		t.Fatalf("native RR 1.3 under a configured 1.2 floor: %q", r)
	}
	// The router receives the same floor.
	e := NewStrategyEngine(cfg)
	if got := e.config.RiskControl.MinRiskRewardRatio; got != 1.2 {
		t.Fatalf("config %v", got)
	}
}

func TestDoctrineNoLongerStatesA15Floor(t *testing.T) {
	e := NewStrategyEngine(ownerHunterV7Config())
	for _, lang := range []Language{LangEnglish, LangChinese} {
		d := hunterV7DoctrinePrompt(lang, e.hunterV7DoctrineInputs())
		if strings.Contains(d, "1.50") {
			t.Errorf("%s doctrine still names 1.50", lang)
		}
		if !strings.Contains(d, "effective_rr >= 1.20") && !strings.Contains(d, "effective_rr ≥ 1.20") {
			t.Errorf("%s doctrine displacement review floor is not the configured 1.20", lang)
		}
	}
}

func TestMaxPositionsNotClamped(t *testing.T) {
	cfg := ownerHunterV7Config()
	cfg.RiskControl.MaxPositions = 5
	cfg.ClampLimits()
	if cfg.RiskControl.MaxPositions != 5 {
		t.Fatalf("max positions clamped to %d", cfg.RiskControl.MaxPositions)
	}
	if n, err := ownerHunterV7Config().RiskControl.ValidateMaxPositions(); err != nil || n != 1 {
		t.Fatalf("configured 1: %d %v", n, err)
	}
	cfg.RiskControl.MaxPositions = 0
	if _, err := cfg.RiskControl.ValidateMaxPositions(); err == nil {
		t.Fatal("unset max positions must be a validation error, not a default of 3")
	}
}
