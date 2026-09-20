package kernel

import (
	"testing"
	"time"

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

func TestBuildHunterV7SignalDBRecordsOnlyMarksOpenReviewActive(t *testing.T) {
	records := []local.V7SignalRecord{
		{
			Signal: local.V7SignalOutput{
				Symbol:    "OPENUSDT",
				Direction: local.V7DirLong,
				SetupType: local.V7SetupAltLadderLong,
				Status:    local.V7StatusCandidate,
			},
			Tier: string(local.V7ReadinessReviewable),
		},
		{
			Signal: local.V7SignalOutput{
				Symbol:    "WATCHUSDT",
				Direction: local.V7DirShort,
				SetupType: local.V7SetupAltLadderShort,
				Status:    local.V7StatusCandidate,
			},
			Tier: string(local.V7ReadinessWatch),
		},
		{
			Signal: local.V7SignalOutput{
				Symbol:    "REJECTUSDT",
				Direction: local.V7DirLong,
				SetupType: local.V7SetupModuleNoMatch,
				Status:    local.V7StatusFiltered,
			},
			Tier: string(local.V7ReadinessRejected),
		},
	}

	dbRecords := BuildHunterV7SignalDBRecords(1, records, time.Now(), "ACTIVE")
	if len(dbRecords) != len(records) {
		t.Fatalf("db records len = %d, want %d", len(dbRecords), len(records))
	}
	if dbRecords[0].TrackStatus != "ACTIVE" {
		t.Fatalf("reviewable track status = %q, want ACTIVE", dbRecords[0].TrackStatus)
	}
	if dbRecords[1].TrackStatus != "" {
		t.Fatalf("watch track status = %q, want empty", dbRecords[1].TrackStatus)
	}
	if dbRecords[2].TrackStatus != "" {
		t.Fatalf("rejected track status = %q, want empty", dbRecords[2].TrackStatus)
	}
}
