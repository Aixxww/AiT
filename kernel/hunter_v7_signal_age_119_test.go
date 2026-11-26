package kernel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// #119 (naming F-04): neither payload carries signal_age_ms, so no prompt
// surface (legacy or hybrid system prompt, doctrine, output format) and no
// shipped template asks the model for it.
func TestSignalAge119NoPromptRequestsSignalAgeMs(t *testing.T) {
	for _, lang := range []string{"en", "zh"} {
		if out := u11RenderPrompts(u11FixtureConfig(lang)); strings.Contains(out, "signal_age_ms") {
			t.Errorf("legacy %s prompt still asks for signal_age_ms", lang)
		}
		_, system, doctrine, user := u11RenderHybrid(u11HybridConfig(lang))
		for name, text := range map[string]string{"system": system, "doctrine": doctrine, "user": user} {
			if strings.Contains(text, "signal_age_ms") {
				t.Errorf("hybrid %s %s prompt still asks for signal_age_ms", lang, name)
			}
		}
	}
	files, err := filepath.Glob(filepath.Join("..", "docs", "strategy-prompt-templates", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("templates: %v", err)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "signal_age_ms") {
			t.Errorf("%s still references signal_age_ms", filepath.Base(f))
		}
	}
}
