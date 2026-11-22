package kernel

import (
	"testing"
	"time"

	local "github.com/Aixxww/AiT/provider/local"
)

// Fix 11: the T2 event id travels from the output to the U5 tag and the
// persisted row (column t2_event_id and raw_json); native rows have none.
func TestT2EventIDPersisted(t *testing.T) {
	t2 := local.V7SignalOutput{Symbol: "AUSDT", Direction: local.V7DirLong, SetupType: "ignition_early_long", Stack: local.V7StackT2,
		LifecycleState: "WATCH", Status: local.V7StatusWaitConfirm, T2EventID: "AUSDT:ignition_early_long:LONG:200"}
	native := local.V7SignalOutput{Symbol: "BUSDT", Direction: local.V7DirShort, SetupType: "trend_breakout_short", Status: local.V7StatusCandidate}
	recs := BuildHunterV7SignalRecordsTagged([]local.V7SignalOutput{t2, native}, nil, DefaultHunterV7RecordTagOptions())
	if len(recs) != 2 || recs[0].Tags.T2EventID != t2.T2EventID || recs[1].Tags.T2EventID != "" {
		t.Fatalf("tags %+v / %+v", recs[0].Tags, recs[1].Tags)
	}
	db := BuildHunterV7SignalDBRecords(7, recs, time.Now().UTC(), "")
	if db[0].T2EventID != t2.T2EventID || db[1].T2EventID != "" {
		t.Fatalf("db rows %q / %q", db[0].T2EventID, db[1].T2EventID)
	}
	if !containsSub(db[0].RawJSON, `"t2_event_id":"AUSDT:ignition_early_long:LONG:200"`) || containsSub(db[1].RawJSON, "t2_event_id") {
		t.Fatalf("raw_json: %s | %s", db[0].RawJSON, db[1].RawJSON)
	}
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
