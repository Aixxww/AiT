package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

// Fix 10 (P1-9): the validation strategy enables every transplanted setup
// type except bb_oi, in dispatcher order, and writes it as v7_t2_setups.
func TestTierAStrategyDisablesBBOIOnly(t *testing.T) {
	var want []string
	for _, s := range local.V7T2SetupTypes {
		if !strings.HasPrefix(s, "bb_oi_") {
			want = append(want, s)
		}
	}
	if len(want) != len(local.V7T2SetupTypes)-2 {
		t.Fatalf("expected exactly two bb_oi setup types in %v", local.V7T2SetupTypes)
	}
	cfg := tierAStrategyConfig()
	h := cfg.CoinSource.Hunter
	if !reflect.DeepEqual(h.V7T2Setups, want) {
		t.Fatalf("v7_t2_setups %v\nwant %v", h.V7T2Setups, want)
	}
	if !h.V7T2Enabled || h.V7MinAIPriority != 55 || cfg.CoinSource.SourceType != "hunter_v7" {
		t.Fatalf("other fields changed: %+v", h)
	}
	// Written to the strategy row as v7_t2_setups and read back unchanged.
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var back store.StrategyConfig
	if err := json.Unmarshal(raw, &back); err != nil || !reflect.DeepEqual(back.CoinSource.Hunter.V7T2Setups, want) {
		t.Fatalf("round trip %v (%v)", back.CoinSource.Hunter, err)
	}
	if strings.Contains(string(raw), "bb_oi") || !strings.Contains(string(raw), `"v7_t2_setups":[`) {
		t.Fatalf("config JSON: %s", raw)
	}
	// The returned list is a copy.
	h.V7T2Setups[0] = "x"
	if tierAV7T2Setups[0] == "x" {
		t.Fatal("tierAStrategyConfig must copy the list")
	}
}
