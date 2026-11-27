package kernel

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/store"
)

// #157 hunter_sniffer.strict_squeeze: opt-in rigid bb_squeeze_15m gate.
func TestStrictSqueeze157(t *testing.T) {
	cands := []CandidateCoin{
		{Symbol: "SQZUSDT", Direction: "LONG", LongScore: 30, LongTags: []string{"bb_squeeze_15m"}},
		{Symbol: "NOSQZUSDT", Direction: "LONG", LongScore: 30, LongTags: []string{"oi_spike_1h"}},
		{Symbol: "SHRTUSDT", Direction: "SHORT", ShortScore: 30, ShortTags: []string{"bb_squeeze_15m"}},
	}
	e := &StrategyEngine{}
	loose := e.filterSniffCandidates(cands, &store.SnifferConfig{MinLongScore: 20, MinShortScore: 20})
	strict := e.filterSniffCandidates(cands, &store.SnifferConfig{MinLongScore: 20, MinShortScore: 20, StrictSqueeze: true})
	if len(loose) != 3 {
		t.Fatalf("default (off) must keep all 3, got %d", len(loose))
	}
	if len(strict) != 2 || strict[0].Symbol != "SQZUSDT" || strict[1].Symbol != "SHRTUSDT" {
		t.Fatalf("strict_squeeze kept %+v", strict)
	}
}

// #157 indicators.external_data_sources: fetched per cycle, re-fetched only
// after refresh_secs, and written into the user prompt as data.
func TestExternalDataSources157(t *testing.T) {
	externalDataMu.Lock()
	externalDataCache = map[string]externalDataEntry{}
	externalDataMu.Unlock()
	calls := 0
	orig := externalSourceFetcher
	defer func() { externalSourceFetcher = orig }()
	fail := false
	externalSourceFetcher = func(_ *StrategyEngine, s store.ExternalDataSource) (interface{}, error) {
		calls++
		if fail {
			return nil, errors.New("down")
		}
		return map[string]interface{}{"value": calls}, nil
	}
	cfg := store.GetDefaultStrategyConfig("en")
	e := NewStrategyEngine(&cfg)
	if e.ExternalDataForCycle(time.Now()) != nil || calls != 0 {
		t.Fatal("no sources configured: no fetch, no data")
	}
	cfg.Indicators.ExternalDataSources = []store.ExternalDataSource{
		{Name: "fear_greed", Type: "api", URL: "https://example.com/fg", Method: "GET", RefreshSecs: 60},
	}
	t0 := time.Unix(1_800_000_000, 0)
	d1 := e.ExternalDataForCycle(t0)
	d2 := e.ExternalDataForCycle(t0.Add(30 * time.Second)) // within refresh_secs
	d3 := e.ExternalDataForCycle(t0.Add(61 * time.Second)) // expired
	if calls != 2 || d1["fear_greed"].(map[string]interface{})["value"] != 1 || d2["fear_greed"].(map[string]interface{})["value"] != 1 || d3["fear_greed"].(map[string]interface{})["value"] != 2 {
		t.Fatalf("refresh_secs not respected: calls=%d %v %v %v", calls, d1, d2, d3)
	}
	fail = true
	if d4 := e.ExternalDataForCycle(t0.Add(200 * time.Second)); d4["fear_greed"].(map[string]interface{})["value"] != 2 {
		t.Fatalf("a failed fetch must reuse the last good value: %v", d4)
	}
	ctx := &Context{ExternalData: d3}
	user := e.BuildUserPrompt(ctx)
	if !strings.Contains(user, "## External Data") || !strings.Contains(user, "### fear_greed") || !strings.Contains(user, `{"value":2}`) {
		t.Fatalf("user prompt lacks the external data section:\n%s", user)
	}
	if strings.Contains(e.BuildUserPrompt(&Context{}), "## External Data") {
		t.Fatal("no external data: no section")
	}
}
