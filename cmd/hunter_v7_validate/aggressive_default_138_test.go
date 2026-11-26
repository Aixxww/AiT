package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// #138 (R1-#1 residual): aggressive weighting is OFF by default in the CLI
// flag, the CLI's fallback strategy, and every shipped strategy template.
func TestAggressiveDefaultOff138(t *testing.T) {
	if defaultV7Aggressive {
		t.Fatal("CLI default for -aggressive must be false")
	}
	if cfg := promptStrategyConfig(5, nil); cfg.CoinSource.Hunter == nil || cfg.CoinSource.Hunter.V7Aggressive {
		t.Fatalf("CLI fallback strategy sets aggressive: %+v", cfg.CoinSource.Hunter)
	}
	files, err := filepath.Glob(filepath.Join("..", "..", "docs", "strategy-prompt-templates", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no templates found: %v", err)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var v interface{}
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		var walk func(x interface{})
		walk = func(x interface{}) {
			switch x := x.(type) {
			case map[string]interface{}:
				for k, val := range x {
					if strings.EqualFold(k, "v7_aggressive") && val == true {
						t.Errorf("%s ships v7_aggressive: true", filepath.Base(f))
					}
					walk(val)
				}
			case []interface{}:
				for _, e := range x {
					walk(e)
				}
			}
		}
		walk(v)
	}
}
