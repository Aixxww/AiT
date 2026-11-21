package main

import (
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/datafetch"
)

// Fix 05: two-tier fetch wiring in the harness.

func TestTwoTierDefaults(t *testing.T) {
	if defaultSlowEvery != 4 || defaultWeightLimitPerMin != 1800 || defaultFuturesDataLimitPer5Min != 1000 {
		t.Fatalf("defaults %d / %v / %d, want 4 / 1800 / 1000", defaultSlowEvery, defaultWeightLimitPerMin, defaultFuturesDataLimitPer5Min)
	}
}

func TestRoundFetcherConfigTwoTier(t *testing.T) {
	legacy := roundFetcherConfig(validationOptions{topDetail: 220}, true, false)
	if legacy.Tiered != nil || legacy.WeightLimitPerMin != 0 || legacy.FuturesDataLimitPer5Min != 0 {
		t.Fatalf("--two-tier=false must keep the legacy fetch: %+v", legacy)
	}
	opts := validationOptions{topDetail: 220, twoTier: true, slowEvery: 4, weightLimitPerMin: 1800, futuresDataLimitPer5Min: 1000}
	c := roundFetcherConfig(opts, true, false)
	if c.Tiered == nil || c.Tiered.SlowEvery != 4 || c.Tiered.T2InputsTopN != 220 {
		t.Fatalf("tiered config %+v", c.Tiered)
	}
	if c.WeightLimitPerMin != 1800 || c.FuturesDataLimitPer5Min != 1000 || !c.IncludeT2Inputs || c.TopNForDetail != 220 {
		t.Fatalf("config %+v", c)
	}
}

// The two-tier fetcher is reused across rounds and attempts (attempt copies
// of opts share the cache) so slow data carries; a config change starts a
// new one; legacy mode builds one per round.
func TestRoundFetcherReuse(t *testing.T) {
	opts := validationOptions{topDetail: 220, twoTier: true, slowEvery: 4, roundFetcher: &roundFetcherCache{}}
	cfg := func(o validationOptions, t2 bool) datafetch.FetcherConfig {
		c := roundFetcherConfig(o, t2, false)
		c.ErrorSink = func(string, error) {}
		return c
	}
	a := roundFetcher(opts, cfg(opts, true))
	attemptCopy := opts
	attemptCopy.attempt = 2
	if b := roundFetcher(attemptCopy, cfg(attemptCopy, true)); b != a {
		t.Fatal("a new round or attempt must reuse the two-tier fetcher")
	}
	if c := roundFetcher(opts, cfg(opts, false)); c == a {
		t.Fatal("a changed config (T2 toggled) must start a new fetcher")
	}
	legacy := validationOptions{topDetail: 220, roundFetcher: &roundFetcherCache{}}
	if roundFetcher(legacy, cfg(legacy, true)) == roundFetcher(legacy, cfg(legacy, true)) {
		t.Fatal("legacy mode builds a fetcher per round")
	}
}

func TestSnapshotSummaryTwoTier(t *testing.T) {
	snap := &datafetch.Snapshot{Symbols: map[string]*datafetch.SymbolSnapshot{}, Meta: datafetch.SnapshotMeta{
		SymbolCount: 515, DetailRequested: 515, DetailCompleted: 515,
		Tiered: true, SlowEvery: 4, FetchIndex: 1, T2InputsTopN: 220,
		SlowRefreshed: 129, SlowCarried: 130, SlowMissing: 256, SlowMaxAge: 390 * time.Second,
	}}
	s := buildSnapshotSummary(snap, 300, "neutral", time.Minute)
	if !s.TwoTier || s.SlowEvery != 4 || s.FetchIndex != 1 || s.T2InputsTopN != 220 ||
		s.SlowRefreshed != 129 || s.SlowCarried != 130 || s.SlowMissing != 256 || s.SlowMaxAgeSec != 390 {
		t.Fatalf("summary %+v", s)
	}
	s = annotateSnapshotQuality(s)
	found := false
	for _, is := range snapshotIssues(s) {
		if is.Code == "slow_tier_missing" {
			found = true
			if !strings.Contains(is.Detail, "256 of 515") {
				t.Fatalf("detail %q", is.Detail)
			}
		}
	}
	if !found {
		t.Fatal("slow_tier_missing issue not raised")
	}
	// Observability only: acceptance and the contract signature are unaffected.
	if snapshotAcceptanceError(s) != nil || contractSignature(validationReport{FormatCheck: formatCheck{JSONMarshalOK: true, JSONUnmarshalOK: true}, Issues: snapshotIssues(s)}) != "contract:unspecified" {
		t.Fatal("slow_tier_missing must not affect acceptance or the contract signature")
	}
	md := formatMarkdown(validationReport{Snapshot: s}, "raw.json")
	for _, want := range []string{
		"| Two-tier fetch: slow every N / fetch index / T2-input top N | 4 / 1 / 220 |",
		"| Slow tier refreshed / carried / missing | 129 / 130 / 256 |",
		"| Slow tier max age s | 390 |",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown lacks %q", want)
		}
	}
	// Legacy rounds report nothing new.
	legacy := annotateSnapshotQuality(buildSnapshotSummary(&datafetch.Snapshot{Meta: datafetch.SnapshotMeta{SymbolCount: 515, DetailRequested: 220, DetailCompleted: 220}}, 140, "", 0))
	for _, is := range snapshotIssues(legacy) {
		if is.Code == "slow_tier_missing" {
			t.Fatal("no slow-tier issue in legacy mode")
		}
	}
	if strings.Contains(formatMarkdown(validationReport{Snapshot: legacy}, "raw.json"), "Two-tier") {
		t.Fatal("no two-tier rows in legacy mode")
	}
}
