package store

import (
	"testing"
	"time"
)

// #111 S3 (owner decision Q3): why_this_could_fail has its own
// decision_records column; a missing or unusable value is stored as NULL
// and never fails the insert.
func TestS3WhyThisCouldFailColumn(t *testing.T) {
	st, err := New(t.TempDir() + "/data.db")
	if err != nil {
		t.Fatalf("store init failed: %v", err)
	}
	if !st.gdb.Migrator().HasColumn(&DecisionRecordDB{}, "why_this_could_fail") {
		t.Fatal("why_this_could_fail column missing")
	}
	str := func(s string) *string { return &s }
	cases := []struct {
		name string
		in   *string
		want *string
	}{
		{"value", str("  a failed breakout back under 1.05  "), str("a failed breakout back under 1.05")},
		{"absent", nil, nil},
		{"blank", str("   "), nil},
		{"invalid utf8", str("bad \xff byte"), nil},
	}
	for i, c := range cases {
		rec := &DecisionRecord{TraderID: "s3", CycleNumber: i + 1, Timestamp: time.Now().UTC(), Success: true, WhyThisCouldFail: c.in,
			Decisions: []DecisionAction{{Action: "open_long", Symbol: "XUSDT", WhyThisCouldFail: "per-action value"}}}
		if err := st.Decision().LogDecision(rec); err != nil {
			t.Fatalf("%s: insert must not fail: %v", c.name, err)
		}
		var db DecisionRecordDB
		if err := st.gdb.First(&db, rec.ID).Error; err != nil {
			t.Fatalf("%s: load: %v", c.name, err)
		}
		got := db.toRecord()
		switch {
		case c.want == nil && got.WhyThisCouldFail != nil:
			t.Errorf("%s: got %q, want NULL", c.name, *got.WhyThisCouldFail)
		case c.want != nil && (got.WhyThisCouldFail == nil || *got.WhyThisCouldFail != *c.want):
			t.Errorf("%s: got %v, want %q", c.name, got.WhyThisCouldFail, *c.want)
		}
		if len(got.Decisions) != 1 || got.Decisions[0].WhyThisCouldFail != "per-action value" {
			t.Errorf("%s: per-action value lost: %+v", c.name, got.Decisions)
		}
	}
	var nulls int64
	st.gdb.Model(&DecisionRecordDB{}).Where("why_this_could_fail IS NULL").Count(&nulls)
	if nulls != 3 {
		t.Fatalf("NULL rows %d, want 3", nulls)
	}
}
