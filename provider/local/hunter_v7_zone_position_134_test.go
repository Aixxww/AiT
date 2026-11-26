package local

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// #134 (F-01): scoring-time zone_position (confirmation_summary and
// execution_readiness) is the canonical from-lower-edge, clamped measure for
// both directions.
func TestZonePosition134ScoringTimeCanonical(t *testing.T) {
	for _, dir := range []V7Direction{V7DirLong, V7DirShort} {
		for _, tc := range []struct {
			price, want float64
		}{{108.5, 85}, {101.5, 15}, {112, 100}, {98, 0}} {
			sig := &V7SignalOutput{Direction: dir, EntryZone: V7PriceZone{Lower: 100, Upper: 110}}
			ctx := &V7SymbolContext{CurrentPrice: tc.price}
			summary := EvaluateV7Confirmations(sig, ctx, DefaultV7Config())
			if math.Abs(summary.EntryZonePosition-tc.want) > 1e-6 {
				t.Errorf("%s @%.1f confirmation_summary.entry_zone_position = %.3f, want %.1f", dir, tc.price, summary.EntryZonePosition, tc.want)
			}
			got, ok := v7EntryZonePositionPct(sig, tc.price)
			if !ok || math.Abs(got-tc.want) > 1e-6 {
				t.Errorf("%s @%.1f readiness helper = %.3f/%v, want %.1f", dir, tc.price, got, ok, tc.want)
			}
		}
	}
}

// TimingBooster compares percent against percent and applies the chase
// penalty to the chase end of the zone for the signal's direction only.
func TestZonePosition134TimingBoosterDirectionAware(t *testing.T) {
	cases := []struct {
		name     string
		dir      V7Direction
		price    float64
		taker    float64
		wantCode string
	}{
		{"long_top_weak_buy_penalised", V7DirLong, 108.5, 0.55, "chase_high_protection"},
		{"long_top_strong_buy_ok", V7DirLong, 108.5, 0.65, ""},
		{"long_bottom_ok", V7DirLong, 101.5, 0.30, ""},
		{"short_top_is_retest_edge_ok", V7DirShort, 108.5, 0.55, ""},
		{"short_bottom_weak_sell_penalised", V7DirShort, 101.5, 0.45, "chase_low_protection"},
		{"short_bottom_strong_sell_ok", V7DirShort, 101.5, 0.35, ""},
		{"long_at_70_not_extended", V7DirLong, 107, 0.50, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sig := &V7SignalOutput{Direction: tc.dir, EntryZone: V7PriceZone{Lower: 100, Upper: 110}, TimingScore: 70}
			ctx := &V7SymbolContext{CurrentPrice: tc.price, TakerBuy15m: tc.taker}
			res := DefaultTimingBooster().EnhanceTiming(sig, ctx)
			hasHigh := containsString(sig.ReasonCodes, "chase_high_protection")
			hasLow := containsString(sig.ReasonCodes, "chase_low_protection")
			switch tc.wantCode {
			case "":
				if hasHigh || hasLow || res.Delta != 0 {
					t.Fatalf("unexpected chase penalty: reasons=%v delta=%.1f", sig.ReasonCodes, res.Delta)
				}
			default:
				if !containsString(sig.ReasonCodes, tc.wantCode) || res.Delta != DefaultTimingBooster().config.TimingPenalty {
					t.Fatalf("want %s with delta %.0f, got reasons=%v delta=%.1f", tc.wantCode, DefaultTimingBooster().config.TimingPenalty, sig.ReasonCodes, res.Delta)
				}
				if hasHigh && hasLow {
					t.Fatalf("both chase codes set: %v", sig.ReasonCodes)
				}
			}
		})
	}
	if def, ok := HunterV7TagLLMAction("chase_low_protection"); !ok || def != V7TagActionWaitOnly {
		t.Fatalf("chase_low_protection catalog action = %q/%v, want wait_only", def, ok)
	}
}

// Acceptance: no production file outside hunter_v7_geom.go reimplements the
// zone position formula (price-lower)/(upper-lower).
func TestZonePosition134NoIndependentFormula(t *testing.T) {
	formula := regexp.MustCompile(`\(\s*[A-Za-z0-9_.]+\s*-\s*[A-Za-z0-9_.]*[Ll]ower\s*\)\s*/\s*\(?\s*[A-Za-z0-9_.]*([Uu]pper\s*-\s*[A-Za-z0-9_.]*[Ll]ower|[Ww]idth|[Ss]pan)`)
	inverted := regexp.MustCompile(`\(\s*[A-Za-z0-9_.]*[Uu]pper\s*-\s*[A-Za-z0-9_.]*price\s*\)\s*/\s*\(?\s*[A-Za-z0-9_.]*([Uu]pper\s*-\s*[A-Za-z0-9_.]*[Ll]ower|[Ww]idth|[Ss]pan)`)
	root := filepath.Join("..", "..")
	var hits []string
	for _, dir := range []string{"kernel", "trader", "provider", "backtest", "api", "store"} {
		_ = filepath.Walk(filepath.Join(root, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			if filepath.Base(path) == "hunter_v7_geom.go" {
				return nil
			}
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			for i, line := range strings.Split(string(body), "\n") {
				if formula.MatchString(line) || inverted.MatchString(line) {
					hits = append(hits, path+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
				}
			}
			return nil
		})
	}
	if len(hits) > 0 {
		t.Fatalf("independent zone position formula outside V7ZonePositionPct (#134):\n%s", strings.Join(hits, "\n"))
	}
}
