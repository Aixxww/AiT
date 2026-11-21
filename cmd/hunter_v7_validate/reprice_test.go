package main

import (
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/datafetch"
	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/provider/local"
)

// Fix 04: pre-prompt repricing.

func repriceFixture(now time.Time) *datafetch.Snapshot {
	bars := func(n int, intervalMs int64) []datafetch.Kline {
		out := make([]datafetch.Kline, n)
		for i := range out {
			ct := now.UnixMilli() - int64(n-1-i)*intervalMs
			out[i] = datafetch.Kline{OpenTime: ct - intervalMs + 1, CloseTime: ct, Open: 100, High: 101, Low: 99, Close: 100, Volume: 10}
		}
		return out
	}
	old := now.Add(-195 * time.Second) // the Tier A fetch-length staleness
	return &datafetch.Snapshot{CreatedAt: old, Symbols: map[string]*datafetch.SymbolSnapshot{
		"KUSDT": {Symbol: "KUSDT", Price: 100, Timestamp: old, FundingRate: 0.0005,
			Klines: map[string][]datafetch.Kline{"5m": bars(80, 300000), "15m": bars(80, 900000), "1h": bars(80, 3600000)}},
		"SUSDT": {Symbol: "SUSDT", Price: 5, Timestamp: old},
		"XUSDT": {Symbol: "XUSDT", Price: 7, Timestamp: old},
	}}
}

func TestRepriceForPromptCopiesAndReprices(t *testing.T) {
	now := time.Now()
	snap := repriceFixture(now)
	fetched := now.Add(-800 * time.Millisecond)
	prices := map[string]float64{"KUSDT": 123.4567, "SUSDT": 5.5, "OTHERUSDT": 1}
	got, sum := repriceForPrompt(snap, []string{"KUSDT", "MISSINGUSDT", "SUSDT"}, prices, fetched, now, time.Minute)
	if got.Symbols["KUSDT"].Price != 123.4567 || !got.Symbols["KUSDT"].Timestamp.Equal(fetched) || got.Symbols["SUSDT"].Price != 5.5 {
		t.Fatalf("not repriced: KUSDT price %v ts %v, SUSDT price %v", got.Symbols["KUSDT"].Price, got.Symbols["KUSDT"].Timestamp, got.Symbols["SUSDT"].Price)
	}
	if snap.Symbols["KUSDT"].Price != 100 || snap.Symbols["SUSDT"].Price != 5 {
		t.Fatal("the scoring snapshot must not be modified")
	}
	if got.Symbols["XUSDT"] != snap.Symbols["XUSDT"] || len(got.Symbols) != len(snap.Symbols) {
		t.Fatal("non-prompt symbols must be carried unchanged")
	}
	if len(got.Symbols["KUSDT"].Klines["5m"]) != 80 {
		t.Fatal("bars must be carried")
	}
	if sum.Symbols != 3 || sum.Repriced != 2 || len(sum.Missing) != 1 || sum.Missing[0] != "MISSINGUSDT" ||
		sum.TickerAgeMs != 800 || sum.MaxAgeMs != 60000 || len(sum.Stale) != 1 {
		t.Fatalf("summary %+v", sum)
	}
	iss := repriceIssues(sum)
	if len(iss) != 1 || iss[0].Code != "prompt_price_stale" || iss[0].Severity != "medium" || !strings.Contains(iss[0].Detail, "MISSINGUSDT") {
		t.Fatalf("issues %+v", iss)
	}
	// A ticker older than the cap makes every prompt price stale.
	_, sum = repriceForPrompt(snap, []string{"KUSDT"}, prices, now.Add(-61*time.Second), now, time.Minute)
	if len(sum.Stale) != 1 || sum.Stale[0] != "KUSDT" {
		t.Fatalf("cap exceeded: %+v", sum)
	}
	// All fresh: no issue.
	_, sum = repriceForPrompt(snap, []string{"KUSDT"}, prices, fetched, now, time.Minute)
	if len(repriceIssues(sum)) != 0 {
		t.Fatal("no issue when every prompt price is fresh")
	}
}

// The prompt's market data uses the repriced price on both converter paths
// (full klines and sparse), and funding is unchanged.
func TestPromptMarketDataUsesRepricedPrice(t *testing.T) {
	now := time.Now()
	snap := repriceFixture(now)
	got, _ := repriceForPrompt(snap, []string{"KUSDT", "SUSDT"}, map[string]float64{"KUSDT": 123.4567, "SUSDT": 5.5}, now, now, time.Minute)
	signals := []local.V7SignalOutput{{Symbol: "KUSDT"}, {Symbol: "SUSDT", PriceCtx: &local.V7PriceContext{Last: 5}}}
	before := promptMarketData(signals, snap)
	after := promptMarketData(signals, got)
	if before["KUSDT"].CurrentPrice != 100 || after["KUSDT"].CurrentPrice != 123.4567 || after["KUSDT"].FundingRate != 0.0005 {
		t.Fatalf("klines path: before %v after %v funding %v", before["KUSDT"].CurrentPrice, after["KUSDT"].CurrentPrice, after["KUSDT"].FundingRate)
	}
	// Sparse path (no klines): uses the signal's PriceCtx.Last, which the
	// kernel prompt pass then replaces with market data; the snapshot price
	// is what the full path reads, so check it there.
	if got.Symbols["SUSDT"].Price != 5.5 {
		t.Fatal("sparse symbol not repriced in the prompt snapshot")
	}
}

// Freshness: repriced candidates show the ticker age; every candidate gets
// the fixed cap; the assembled candidates (persisted with their tiers) are
// not modified when the caller works on a copy, as runValidation does.
func TestApplyPromptFreshnessOnCopy(t *testing.T) {
	now := time.Now()
	snap := repriceFixture(now)
	fetched := now.Add(-2 * time.Second)
	got, _ := repriceForPrompt(snap, []string{"KUSDT"}, map[string]float64{"KUSDT": 120}, fetched, now, time.Minute)
	assembled := []kernel.CandidateCoin{
		{Symbol: "KUSDT", V7ExecutionTier: "REVIEWABLE", V7TierReason: "x", V7DataFreshness: local.V7DataFreshness{PriceAgeMs: 195000, StaleThresholdMs: 180000, Kline1mAgeMs: 150000}},
		{Symbol: "SUSDT", V7ExecutionTier: "WATCH", V7DataFreshness: local.V7DataFreshness{PriceAgeMs: 190000, StaleThresholdMs: 180000}},
	}
	prompt := append([]kernel.CandidateCoin(nil), assembled...)
	applyPromptFreshness(prompt, got, snap, now, time.Minute)
	if prompt[0].V7DataFreshness.PriceAgeMs != 2000 || prompt[0].V7DataFreshness.StaleThresholdMs != 60000 || prompt[0].V7DataFreshness.Kline1mAgeMs != 150000 {
		t.Fatalf("repriced freshness %+v", prompt[0].V7DataFreshness)
	}
	if prompt[1].V7DataFreshness.PriceAgeMs != 190000 || prompt[1].V7DataFreshness.StaleThresholdMs != 60000 {
		t.Fatalf("not-repriced freshness %+v (age kept, cap applied)", prompt[1].V7DataFreshness)
	}
	if assembled[0].V7DataFreshness.PriceAgeMs != 195000 || assembled[0].V7DataFreshness.StaleThresholdMs != 180000 ||
		assembled[0].V7ExecutionTier != "REVIEWABLE" || assembled[1].V7ExecutionTier != "WATCH" {
		t.Fatal("assembled candidates must be unchanged")
	}
}

func TestPromptSymbolsUnion(t *testing.T) {
	got := promptSymbols([]local.V7SignalOutput{{Symbol: "BUSDT"}, {Symbol: "AUSDT"}, {Symbol: ""}},
		[]kernel.CandidateCoin{{Symbol: "AUSDT"}, {Symbol: "CUSDT"}})
	if strings.Join(got, ",") != "AUSDT,BUSDT,CUSDT" {
		t.Fatalf("symbols %v", got)
	}
}

// End to end through buildPrompt: the prompt text carries the repriced
// price, not the ~3-minute-old scoring price; the assembled tier is the same.
func TestBuildPromptShowsRepricedPrice(t *testing.T) {
	now := time.Now()
	snap := repriceFixture(now)
	sig := (&local.V7T2Signal{Symbol: "KUSDT", Direction: "LONG", SetupType: "momentum_continuation_long", Status: "ARMED",
		Score: 90, Trigger: 100, Entry: 100, Stop: 98, Target1: 103.6, Target2: 106, Reasons: []string{"t2_test"}}).ToV7SignalOutputWithLifecycle("", "ARMED")
	local.ApplyV7T2ContractFields(&sig, snap)
	signals := []local.V7SignalOutput{sig}
	cands := kernel.AssembleHunterV7CandidateCoins(signals, "BOTH", executionGeometryFromStrategy(nil))
	if len(cands) != 1 {
		t.Fatalf("candidates %d", len(cands))
	}
	tierBefore := cands[0].V7ExecutionTier
	oldPrompt := buildPrompt(cands, signals, snap, nil)
	got, _ := repriceForPrompt(snap, promptSymbols(signals, cands), map[string]float64{"KUSDT": 101.2345}, now, now, time.Minute)
	pc := append([]kernel.CandidateCoin(nil), cands...)
	applyPromptFreshness(pc, got, snap, now, time.Minute)
	newPrompt := buildPrompt(pc, signals, got, nil)
	if !strings.Contains(newPrompt, "101.2345") {
		t.Fatal("repriced price not in the prompt")
	}
	if strings.Contains(oldPrompt, "101.2345") {
		t.Fatal("fixture leak")
	}
	if cands[0].V7ExecutionTier != tierBefore {
		t.Fatal("assembled tier changed")
	}
}
