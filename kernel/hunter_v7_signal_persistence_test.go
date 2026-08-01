package kernel

import (
	"testing"

	local "github.com/Aixxww/AiT/provider/local"
)

func TestHunterV7ShouldTrackSignalUsesFinalRecordTierOnly(t *testing.T) {
	rec := local.V7SignalRecord{
		Signal: local.V7SignalOutput{
			ExecutionReadiness: &local.V7ExecutionReadiness{
				Tier: local.V7ReadinessReviewable,
			},
		},
	}
	if HunterV7ShouldTrackSignal(rec) {
		t.Fatalf("signal readiness tier without final DB tier must not be tracked")
	}

	rec.Tier = string(local.V7ReadinessReviewable)
	if !HunterV7ShouldTrackSignal(rec) {
		t.Fatalf("final REVIEWABLE tier should be tracked")
	}

	rec.Tier = string(local.V7ReadinessExecutable)
	if !HunterV7ShouldTrackSignal(rec) {
		t.Fatalf("final EXECUTABLE tier should be tracked")
	}

	rec.Tier = string(local.V7ReadinessWatch)
	if HunterV7ShouldTrackSignal(rec) {
		t.Fatalf("final WATCH tier must not be tracked")
	}
}
