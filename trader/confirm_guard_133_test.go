package trader

import (
	"strings"
	"testing"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/provider/local"
)

// #133 / naming F-02: the trader guard enforces only machine-verified
// required confirmations; a checklist or never-registered code can never
// block an open on its own.
func TestConfirmGuard133(t *testing.T) {
	d := &kernel.Decision{Symbol: "RUSDT", Action: "open_long"}
	mk := func(codes []string, s *local.V7ConfirmationSummary) *kernel.CandidateCoin {
		return &kernel.CandidateCoin{Symbol: "RUSDT", V7SetupType: "range_reversion", V7ExecutionTier: "REVIEWABLE", V7RequiredConfirms: codes, V7ConfirmSummary: s}
	}
	checklistOnly := []string{"enter_only_near_range_bottom", "stop_below_range_low", "unregistered_future_code"}
	// Checklist only: no summary needed, never blocked.
	if err := validateHunterV7RequiredConfirmations(mk(checklistOnly, nil), d); err != nil {
		t.Fatalf("checklist codes must not block: %v", err)
	}
	// The pre-#133 shape: the checklist code as a failed context check.
	legacy := &local.V7ConfirmationSummary{ContextChecks: []local.V7ConfirmationCheck{{Code: "enter_only_near_range_bottom", Passed: false, Severity: local.V7ConfirmContext}}}
	if err := validateHunterV7RequiredConfirmations(mk(checklistOnly, legacy), d); err != nil {
		t.Fatalf("a checklist context check must not block: %v", err)
	}
	// A failed machine code still blocks.
	failed := &local.V7ConfirmationSummary{MissingReview: []local.V7ConfirmationCheck{{Code: "live_price_in_entry_zone", Severity: local.V7ConfirmReviewWait}}}
	err := validateHunterV7RequiredConfirmations(mk(append([]string{"live_price_in_entry_zone"}, checklistOnly...), failed), d)
	if err == nil || !strings.Contains(err.Error(), "live_price_in_entry_zone") {
		t.Fatalf("a failed machine code must block: %v", err)
	}
	// A machine code without a summary still needs the refresh.
	if err := validateHunterV7RequiredConfirmations(mk([]string{"live_price_in_entry_zone"}, nil), d); err == nil {
		t.Fatal("a machine code without a summary must block")
	}
}
