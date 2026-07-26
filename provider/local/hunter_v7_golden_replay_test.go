package local

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var updateGolden = flag.Bool("update-golden", false, "re-record the golden replay output")

// TestHunterV7GoldenReplay is the refactor safety net: a frozen live routing
// cycle (2026-07-26, trend_up, 208 symbols) replayed through RouteDetailed and
// compared field-by-field against the recorded output. Any diff is a behavior
// change. Intended changes are re-recorded with:
//
//	go test ./provider/local/ -run TestHunterV7GoldenReplay -update-golden
//
// and the resulting fixture diff is reviewed like code.
func TestHunterV7GoldenReplay(t *testing.T) {
	fixturePath := filepath.Join("testdata", "golden", "universe-20260726.json")
	goldenPath := filepath.Join("testdata", "golden", "route-output-20260726.json")

	fixture, err := LoadV7GoldenFixture(fixturePath)
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	if len(fixture.Universe) == 0 {
		t.Fatal("fixture universe is empty")
	}

	router := NewV7Router()
	result := router.RouteDetailed(fixture.Universe, fixture.Regime, fixture.Config)
	signals := ensureV7SignalIDs(result.OutputSignals, fixture.Config.CycleNumber)
	raw := ensureV7SignalIDs(result.RawSignals, fixture.Config.CycleNumber)

	replay := struct {
		Regime        V7MarketRegime         `json:"regime"`
		OutputSignals []V7SignalOutput       `json:"output_signals"`
		RawSignals    []V7SignalOutput       `json:"raw_signals"`
		PotentialPool []V7PotentialCandidate `json:"potential_pool"`
	}{fixture.Regime, signals, raw, result.PotentialPool}

	got, err := json.MarshalIndent(replay, "", " ")
	if err != nil {
		t.Fatalf("marshal replay: %v", err)
	}

	if *updateGolden {
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("golden re-recorded: %s (%d output signals, %d raw)", goldenPath, len(signals), len(raw))
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with -update-golden to record): %v", err)
	}
	if string(got) == string(want) {
		return
	}

	// Locate the first divergence precisely instead of dumping two megabytes.
	var gotV, wantV map[string]any
	if err := json.Unmarshal(got, &gotV); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}
	if err := json.Unmarshal(want, &wantV); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	t.Fatalf("golden replay diverged: %s", firstJSONDiff("", wantV, gotV))
}

// firstJSONDiff walks two decoded JSON values and returns a path-qualified
// description of the first difference found.
func firstJSONDiff(path string, want, got any) string {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return path + ": type mismatch"
		}
		for k, wv := range w {
			gv, present := g[k]
			if !present {
				return path + "." + k + ": missing in replay"
			}
			if d := firstJSONDiff(path+"."+k, wv, gv); d != "" {
				return d
			}
		}
		for k := range g {
			if _, present := w[k]; !present {
				return path + "." + k + ": unexpected in replay"
			}
		}
	case []any:
		g, ok := got.([]any)
		if !ok {
			return path + ": type mismatch"
		}
		if len(w) != len(g) {
			return path + ": length " + itoa(len(w)) + " -> " + itoa(len(g))
		}
		for i := range w {
			if d := firstJSONDiff(path+"["+itoa(i)+"]", w[i], g[i]); d != "" {
				return d
			}
		}
	default:
		if want != got {
			return path + ": " + jsonScalar(want) + " -> " + jsonScalar(got)
		}
	}
	return ""
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func jsonScalar(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
