package kernel

import (
	"testing"

	"github.com/Aixxww/AiT/store"
)

// The kernel builds the final pass from config and the U6a registry.
func TestHunterV7FinalConflictConfigFromRegistry(t *testing.T) {
	cfg := hunterV7FinalConflictConfig(&store.HunterConfig{V7T2Enabled: true})
	if !cfg.Enabled || cfg.MinAIPriority != 60 || cfg.TieWinner != "SHORT" || !cfg.ContextNonVoting {
		t.Fatalf("cfg = %+v", cfg)
	}
	for _, s := range []string{"funding_reversal", "pre_breakout_watch", "pre_squeeze_watch", "pre_distribution_watch", "accumulation_watch"} {
		if !cfg.ContextSetups[s] {
			t.Errorf("%s must be a CONTEXT setup", s)
		}
	}
	if cfg.ContextSetups["distribution_short"] {
		t.Errorf("distribution_short is REVERSAL, it votes")
	}
	over := hunterV7FinalConflictConfig(&store.HunterConfig{V7SignalClasses: map[string]store.V7SignalClassEntry{
		"funding_reversal": {Class: store.V7ClassReversal},
	}})
	if over.ContextSetups["funding_reversal"] {
		t.Errorf("registry overrides must reach the final pass")
	}
}
