package kernel

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Aixxww/AiT/provider/local"
)

// #138 (R1-#4): a collapsed open-review candidate (compact_execution_json)
// can still be opened, so its compact view carries every decision-relevant
// pre-computed field the doctrine refers to.
func TestR14138CompactViewCarriesDecisionFields(t *testing.T) {
	coin := hunterV7PromptReadyLeaderCandidate("CMPUSDT", 100)
	coin.V7ExecutionTier, coin.V7TierReason = "REVIEWABLE", "near_confirm"
	coin.V7RiskTags = []string{"regime_against_direction"}
	coin.V7RequiredConfirms = []string{"taker_flow_confirms_long"}
	coin.V7ConfirmSummary = &local.V7ConfirmationSummary{PassedHard: true, PassedReview: false}
	coin.V7TP1Price, coin.V7TP2Price = 105, 110
	coin.V7DataFreshness = local.V7DataFreshness{SnapshotAgeMs: 1200, StaleThresholdMs: 90000}

	raw := (&StrategyEngine{}).formatHunterV7CompactSignalJSON(coin)
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("compact json: %v\n%s", err, raw)
	}
	for _, key := range []string{
		"setup_score", "timing_score", "regime_fit_score", "liquidity_score", "risk_score", "ai_priority",
		"reason_codes", "risk_tags", "required_confirmations", "confirmation_summary",
		"execution_geometry", "execution_contract", "tp1_price", "tp2_price",
		"derivatives_context", "data_freshness",
	} {
		if _, ok := m[key]; !ok {
			t.Errorf("compact view lacks %s", key)
		}
	}
	if !strings.Contains(raw, "regime_against_direction") || !strings.Contains(raw, "taker_flow_confirms_long") {
		t.Fatalf("compact view drops the tag / confirmation values: %s", raw)
	}
}

// The hybrid_v2 drops are deliberate (owner deletion list); the doctrine now
// says so, in both languages, so prompt and payload agree.
func TestR14138HybridDoctrineStatesDeliberateOmission(t *testing.T) {
	for lang, want := range map[string]string{
		"en": "The appendix is deliberately small (hybrid design)",
		"zh": "附录刻意精简（hybrid 设计）",
	} {
		_, _, doctrine, _ := u11RenderHybrid(u11HybridConfig(lang))
		if !strings.Contains(doctrine, want) || !strings.Contains(doctrine, "TP0") {
			t.Errorf("%s hybrid doctrine does not state the deliberately omitted fields", lang)
		}
	}
}
