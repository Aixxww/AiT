package trader

import (
	"testing"

	"github.com/Aixxww/AiT/kernel"
)

// U11: the signal contract accepts the hybrid_v2 presentation (t2_ setup
// name, tier_ enum prefix) and still rejects a real mismatch.
func TestU11SignalContractAcceptsHybridNames(t *testing.T) {
	cand := &kernel.CandidateCoin{Symbol: "AUSDT", Direction: "LONG", V7SignalID: "s1", V7SetupType: "ignition_early_long", V7ExecutionTier: "REVIEWABLE"}
	ok := []kernel.Decision{
		{Symbol: "AUSDT", Action: "open_long", SelectedHunterV7SignalID: "s1", SelectedHunterV7Setup: "t2_ignition_early_long", SelectedHunterV7Tier: "tier_REVIEWABLE"},
		{Symbol: "AUSDT", Action: "open_long", SelectedHunterV7SignalID: "s1", SelectedHunterV7Setup: "ignition_early_long", SelectedHunterV7Tier: "REVIEWABLE"},
	}
	for _, d := range ok {
		d := d
		if err := validateHunterV7DecisionSignalContract(cand, &d); err != nil {
			t.Errorf("%+v: %v", d, err)
		}
	}
	bad := []kernel.Decision{
		{Symbol: "AUSDT", Action: "open_long", SelectedHunterV7SignalID: "s1", SelectedHunterV7Setup: "t2_ignition_early_short"},
		{Symbol: "AUSDT", Action: "open_long", SelectedHunterV7SignalID: "s1", SelectedHunterV7Tier: "tier_EXECUTABLE"},
	}
	for _, d := range bad {
		d := d
		if err := validateHunterV7DecisionSignalContract(cand, &d); err == nil {
			t.Errorf("%+v: mismatch accepted", d)
		}
	}
	// Prefixed emission: the candidate carries the t2_ name, the decision
	// either name.
	cand.V7SetupType = "t2_ignition_early_long"
	d := kernel.Decision{Symbol: "AUSDT", Action: "open_long", SelectedHunterV7SignalID: "s1", SelectedHunterV7Setup: "ignition_early_long"}
	if err := validateHunterV7DecisionSignalContract(cand, &d); err != nil {
		t.Errorf("canonical name vs prefixed candidate: %v", err)
	}
}
