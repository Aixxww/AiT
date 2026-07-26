package local

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Tag families produced via runtime prefixes rather than fixed literals. The
// prompt layer explains them through their prefix semantics.
var catalogPrefixTemplates = []string{
	"live_confirmed_",       // kernel pre-prompt live confirmation stamps
	"sector_theme_",         // sector rotation appends the theme name
	"confirmation_missing_", // kernel tier-reason template over confirmation codes
}

// Literals the source scanner picks up that are not tags (JSON field names on
// emission lines, format verbs, etc).
var catalogScanFalsePositives = map[string]bool{
	"reason_codes":           true,
	"risk_tags":              true,
	"required_confirmations": true,
}

// TestHunterV7TagCatalogCoversEmittedTags enforces the vocabulary contract
// from the lean-core redesign: every tag a module can emit must carry catalog
// semantics, because anything uncatalogued reaches the LLM as
// "unknown_context_only" and cannot inform the open/wait decision. Before the
// 2026-07-26 reconciliation, 47% of the live vocabulary (186 tags) was in that
// state.
func TestHunterV7TagCatalogCoversEmittedTags(t *testing.T) {
	emitted := scanEmittedTags(t)
	if len(emitted) < 300 {
		t.Fatalf("scanner found only %d emitted tags; extraction likely broken", len(emitted))
	}

	var missing []string
	for tag := range emitted {
		if catalogScanFalsePositives[tag] {
			continue
		}
		if hasCatalogPrefix(tag) {
			continue
		}
		if _, ok := hunterV7TagCatalog[tag]; !ok {
			missing = append(missing, tag)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("%d emitted tag(s) missing catalog semantics (LLM would see unknown_context_only):\n%s",
			len(missing), strings.Join(sortedStrings(missing), "\n"))
	}
}

func scanEmittedTags(t *testing.T) map[string]bool {
	t.Helper()
	emitLine := regexp.MustCompile(`(ReasonCodes|RiskTags|RequiredConfirms|NextConfirm)\b[^\n]*`)
	literal := regexp.MustCompile(`"([a-z0-9_]{3,})"`)

	files, err := filepath.Glob("hunter_v7*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	emitted := make(map[string]bool)
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") || strings.Contains(file, "tag_catalog") {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			m := emitLine.FindString(line)
			if m == "" {
				continue
			}
			for _, lit := range literal.FindAllStringSubmatch(m, -1) {
				emitted[lit[1]] = true
			}
		}
	}
	return emitted
}

func hasCatalogPrefix(tag string) bool {
	for _, prefix := range catalogPrefixTemplates {
		if strings.HasPrefix(tag, prefix) || tag == strings.TrimSuffix(prefix, "_") {
			return true
		}
	}
	return false
}

func sortedStrings(in []string) []string {
	out := append([]string{}, in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// TestHunterV7GoldenTagsHaveKnownSemantics feeds every tag from the frozen
// live cycle through the describe path the prompt uses, and asserts the LLM
// never receives "unknown_context_only" for real routed signals.
func TestHunterV7GoldenTagsHaveKnownSemantics(t *testing.T) {
	fixture, err := LoadV7GoldenFixture(filepath.Join("testdata", "golden", "universe-20260726.json"))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	result := NewV7Router().RouteDetailed(fixture.Universe, fixture.Regime, fixture.Config)

	var unknown []string
	for _, sig := range result.RawSignals {
		for _, def := range DescribeHunterV7Tags(sig.ReasonCodes, sig.RiskTags, sig.RequiredConfirms) {
			if def.LLMAction == V7TagActionUnknown {
				unknown = append(unknown, sig.Symbol+"/"+string(sig.SetupType)+": "+def.Tag)
			}
		}
	}
	if len(unknown) > 0 {
		t.Fatalf("%d tag(s) reached the LLM with unknown semantics:\n%s",
			len(unknown), strings.Join(sortedStrings(unknown), "\n"))
	}
}

func TestDedupeV7TagsPreservesOrder(t *testing.T) {
	got := dedupeV7Tags([]string{"a", "b", "a", "", "c", "b"})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("dedupe = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dedupe = %v, want %v", got, want)
		}
	}
}
