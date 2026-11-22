package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// Fix 11: t2_event_id is a new nullable column. An existing table (created
// before this change) gains it on start through the usual AutoMigrate,
// keeps its rows (the new column reads as empty), and new rows store it.
func TestT2EventIDColumnMigratesAndRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	// A pre-fix table: today's schema without t2_event_id, with one row.
	pre, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := pre.HunterV7Signal().CreateBatch([]HunterV7SignalRecord{{Timestamp: time.Now().UTC(), Symbol: "OLDUSDT", Direction: "LONG", SetupType: "x"}}); err != nil {
		t.Fatal(err)
	}
	pre.Close()
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`ALTER TABLE hunter_v7_signal_records DROP COLUMN t2_event_id`); err != nil {
		t.Fatal(err)
	}
	var cols int
	if err := raw.QueryRow(`SELECT count(*) FROM pragma_table_info('hunter_v7_signal_records') WHERE name = 't2_event_id'`).Scan(&cols); err != nil || cols != 0 {
		t.Fatalf("fixture still has the column (%d, %v)", cols, err)
	}
	raw.Close()

	st, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.DB().QueryRow(`SELECT count(*) FROM pragma_table_info('hunter_v7_signal_records') WHERE name = 't2_event_id'`).Scan(&cols); err != nil || cols != 1 {
		t.Fatalf("column not added on start (%d, %v)", cols, err)
	}
	var old HunterV7SignalRecord
	if err := st.GormDB().Where("symbol = ?", "OLDUSDT").First(&old).Error; err != nil || old.T2EventID != "" {
		t.Fatalf("pre-fix row after migration: %+v (%v)", old, err)
	}
	recs := []HunterV7SignalRecord{{Timestamp: time.Now().UTC(), Symbol: "AUSDT", Direction: "LONG", SetupType: "ignition_early_long",
		T2EventID: "AUSDT:ignition_early_long:LONG:200"}}
	if err := st.HunterV7Signal().CreateBatch(recs); err != nil {
		t.Fatal(err)
	}
	var got HunterV7SignalRecord
	if err := st.GormDB().First(&got, recs[0].ID).Error; err != nil || got.T2EventID != "AUSDT:ignition_early_long:LONG:200" {
		t.Fatalf("round trip %q (%v)", got.T2EventID, err)
	}
	// Grouping by event works on the column.
	var n int64
	st.GormDB().Model(&HunterV7SignalRecord{}).Where("t2_event_id = ?", got.T2EventID).Count(&n)
	if n != 1 {
		t.Fatalf("rows for the event: %d", n)
	}
}
