package kernel

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Aixxww/AiT/market"
	"github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
)

var updatePromptGolden = flag.Bool("update-prompt-golden", false, "re-record the golden prompt output")

// TestHunterV7GoldenPrompt extends the golden-replay safety net to the kernel
// layer: the frozen live cycle is routed, converted to candidate coins,
// tiered, and rendered through BuildUserPrompt; the full prompt text is
// compared byte-for-byte against the recording. This is what protects tier
// classification, candidate expansion, and the three signal serializers while
// they are being restructured.
func TestHunterV7GoldenPrompt(t *testing.T) {
	fixturePath := filepath.Join("..", "provider", "local", "testdata", "golden", "universe-20260726.json")
	goldenPath := filepath.Join("testdata", "golden-prompt-20260726.txt")

	fixture, err := local.LoadV7GoldenFixture(fixturePath)
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	result := local.NewV7Router().RouteDetailed(fixture.Universe, fixture.Regime, fixture.Config)

	cfg := store.GetDefaultStrategyConfig("zh")
	cfg.CoinSource.SourceType = "hunter_v7"
	cfg.CoinSource.HunterLimit = len(result.OutputSignals)
	engine := NewStrategyEngine(&cfg)

	coins := engine.hunterV7SignalsToCandidateCoins(result.OutputSignals, "BOTH")

	// Market data from the same frozen universe, via the production converter,
	// so the pre-prompt live-confirmation pass exercises real TimeframeData.
	marketData := make(map[string]*market.Data, len(coins))
	for i := range fixture.Universe {
		ctx := &fixture.Universe[i]
		if ctx.Snapshot == nil {
			continue
		}
		marketData[ctx.Symbol] = &market.Data{
			Symbol:        ctx.Symbol,
			CurrentPrice:  ctx.CurrentPrice,
			PriceChange1h: ctx.Change1h,
			PriceChange4h: ctx.Change4h,
		}
	}

	ctx := &Context{
		CurrentTime:    "2026-07-26 20:07:24",
		CallCount:      1,
		RuntimeMinutes: 0,
		Account:        AccountInfo{TotalEquity: 1000, AvailableBalance: 1000},
		CandidateCoins: coins,
		MarketDataMap:  marketData,
	}
	prompt := engine.BuildUserPrompt(ctx)

	if *updatePromptGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(prompt), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("prompt golden re-recorded: %s (%d bytes, %d candidates)", goldenPath, len(prompt), len(coins))
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with -update-prompt-golden to record): %v", err)
	}
	expected := injectHunterV7ExecutionContractGolden(string(want))
	if prompt == expected {
		return
	}
	gotLines := strings.Split(prompt, "\n")
	wantLines := strings.Split(expected, "\n")
	for i := 0; i < len(gotLines) && i < len(wantLines); i++ {
		if gotLines[i] != wantLines[i] {
			t.Fatalf("prompt diverged at line %d:\n want: %s\n got:  %s", i+1, wantLines[i], gotLines[i])
		}
	}
	t.Fatalf("prompt diverged in length: %d -> %d lines", len(wantLines), len(gotLines))
}

func injectHunterV7ExecutionContractGolden(prompt string) string {
	// Frozen contract for universe-20260726: both replayed MMS candidates are
	// REVIEWABLE, share the same required confirmations, and carry the backend
	// TP0 runner policy. Contract drift now fails the complete prompt replay.
	const contract = `,"execution_contract":{"entry_permission":"live_confirmation_required","must_confirm":["5m_price_holds_ema20_or_trailing_support","taker_flow_not_flipping_against_direction","live_price_in_entry_zone"],"size_policy":"normal_if_backend_rr_and_confirmations_pass","exit_policy":"partial_tp0_then_breakeven_runner_to_tp1_tp2"}`
	const anchor = `,"tag_semantics":`
	lines := strings.Split(prompt, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "hunter_v7_signal_json: ") || strings.Contains(line, `"execution_contract":`) {
			continue
		}
		if at := strings.Index(line, anchor); at >= 0 {
			lines[i] = line[:at] + contract + line[at:]
		}
	}
	return strings.Join(lines, "\n")
}

func TestInjectHunterV7ExecutionContractGolden(t *testing.T) {
	in := `hunter_v7_signal_json: {"symbol":"X","tag_semantics":[]}`
	if got := injectHunterV7ExecutionContractGolden(in); !strings.Contains(got, `"execution_contract":`) {
		t.Fatalf("golden contract was not injected: %q", got)
	}
}
