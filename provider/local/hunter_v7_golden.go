package local

import (
	"encoding/json"
	"fmt"
	"os"
)

// ============================================================================
// Hunter v7 — Golden replay fixture
// ============================================================================
// The lean-core refactor (docs/hunter-v7-lean-core-redesign-20260726.md) moves
// scoring rules between representations without changing behavior. Its safety
// net is a frozen real routing cycle: the exact universe, regime, and config
// of one live round, replayed through RouteDetailed on every test run and
// compared field-by-field against the recorded output. The route path is
// deterministic (no clock, no randomness, stable sorts), so any diff is a
// behavior change — intended ones are re-recorded explicitly via -update.

// V7GoldenFixture freezes the inputs of one live routing cycle.
type V7GoldenFixture struct {
	CapturedAt string            `json:"captured_at"`
	Regime     V7MarketRegime    `json:"regime"`
	Config     V7Config          `json:"config"`
	Universe   []V7SymbolContext `json:"universe"`
}

// DumpV7GoldenFixture writes a replay fixture. Called from
// cmd/hunter_v7_validate with -dump-universe.
func DumpV7GoldenFixture(path string, universe []V7SymbolContext, regime V7MarketRegime, cfg V7Config) error {
	fixture := V7GoldenFixture{
		Regime:   regime,
		Config:   cfg,
		Universe: universe,
	}
	data, err := json.MarshalIndent(fixture, "", " ")
	if err != nil {
		return fmt.Errorf("marshal golden fixture: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadV7GoldenFixture reads a replay fixture.
func LoadV7GoldenFixture(path string) (*V7GoldenFixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var fixture V7GoldenFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return nil, fmt.Errorf("unmarshal golden fixture: %w", err)
	}
	return &fixture, nil
}
