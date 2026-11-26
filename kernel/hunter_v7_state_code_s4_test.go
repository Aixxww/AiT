package kernel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	local "github.com/Aixxww/AiT/provider/local"
)

// #111 S4: the state code built from real golden values matches the two
// examples in the design review.
func TestS4StateCodeGoldenExamples(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "provider", "local", "testdata", "golden", "route-output-20260726.json"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var golden struct {
		OutputSignals []local.V7SignalOutput `json:"output_signals"`
	}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("decode golden: %v", err)
	}
	bySymbol := map[string]local.V7SignalOutput{}
	for _, s := range golden.OutputSignals {
		bySymbol[s.Symbol] = s
	}
	// Tiers as in kernel/testdata/golden-prompt-20260726.txt (prompt-final).
	for _, c := range []struct{ symbol, tier, want string }{
		{"OPUSDT", "REVIEWABLE", "V7|OPUSDT|LONG|mms_trend_ride_long|trend_up|P85|S100/T68/F80/L60/R0|REVIEWABLE|candidate"},
		{"CHILLGUYUSDT", "WATCH", "V7|CHILLGUYUSDT|LONG|alt_ladder_momentum_long|trend_up|P84|S89/T76/F80/L70/R0|WATCH|candidate"},
	} {
		sig, ok := bySymbol[c.symbol]
		if !ok {
			t.Fatalf("%s not in the golden output", c.symbol)
		}
		if got := hunterV7SignalStateCode(sig, c.tier); got != c.want {
			t.Errorf("signal: got  %s\n             want %s", got, c.want)
		}
		cands := AssembleHunterV7CandidateCoins([]local.V7SignalOutput{sig}, "BOTH", defaultGeometry())
		if len(cands) != 1 {
			t.Fatalf("%s: assembled %d candidates", c.symbol, len(cands))
		}
		if got := hunterV7CandidateStateCode(cands[0], c.tier); got != c.want {
			t.Errorf("candidate: got  %s\n                want %s", got, c.want)
		}
	}
}

// Task-2 rows carry their lifecycle state and structure timeframes; empty
// values render as "-".
func TestS4StateCodeT2AndEmpty(t *testing.T) {
	sig := t2TagSignal("ignition_early_long", local.V7DirLong, "TRIGGERED")
	cfg := local.V7StructureConfigForSetup(string(sig.SetupType))
	got := hunterV7SignalStateCode(sig, "REVIEWABLE")
	wantSuffix := "|REVIEWABLE|TRIGGERED|tf=" + hunterV7V2TFLabel(cfg.TFSIntervalMs) + "/" + hunterV7V2TFLabel(cfg.TFHIntervalMs)
	if !strings.HasPrefix(got, "V7|"+sig.Symbol+"|LONG|ignition_early_long|") || !strings.HasSuffix(got, wantSuffix) {
		t.Fatalf("T2 state code %s (want suffix %s)", got, wantSuffix)
	}
	if got := hunterV7StateCode(hunterV7StateCodeFields{}); got != "V7|-|-|-|-|P0|S0/T0/F0/L0/R0|-|-" {
		t.Fatalf("empty: %s", got)
	}
}

// Persisted records carry the state code at the record's tier.
func TestS4StateCodePersisted(t *testing.T) {
	sigs := []local.V7SignalOutput{t2TagSignal("ignition_early_long", local.V7DirLong, "TRIGGERED")}
	cands := AssembleHunterV7CandidateCoins(sigs, "BOTH", defaultGeometry())
	recs := BuildHunterV7SignalRecords(sigs, cands)
	if recs[0].Tags.StateCode != hunterV7SignalStateCode(sigs[0], recs[0].Tier) || recs[0].Tags.StateCode == "" {
		t.Fatalf("record state code %q (tier %s)", recs[0].Tags.StateCode, recs[0].Tier)
	}
	db := BuildHunterV7SignalDBRecords(1, recs, time.Now(), "")
	if len(db) != 1 || db[0].StateCode != recs[0].Tags.StateCode || !strings.Contains(db[0].StateCode, "|"+db[0].ExecutionTier+"|") {
		t.Fatalf("db state code %q, execution tier %s", db[0].StateCode, db[0].ExecutionTier)
	}
}

// The state code is log and DB only: it never reaches a prompt.
func TestS4StateCodeNotInPrompt(t *testing.T) {
	for _, cfg := range []func(string) string{
		func(lang string) string { _, s, _, u := u11RenderHybrid(u11FixtureConfig(lang)); return s + u },
		func(lang string) string { _, s, _, u := u11RenderHybrid(u11HybridConfig(lang)); return s + u },
	} {
		for _, lang := range []string{"en", "zh"} {
			if p := cfg(lang); strings.Contains(p, "V7|") {
				t.Fatalf("a prompt (%s) carries a state code", lang)
			}
		}
	}
}
