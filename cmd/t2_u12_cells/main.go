// Command t2_u12_cells runs the U12 dashboard tables over the U5 replay
// records (cmd/t2_u5_tags output): every event's last-state record, with
// the tracker's fee-net outcome, is written to an in-memory store, read back
// through the dashboard query and tabled with the dashboard code. The
// output is the acceptance input for "per-class tables match U5 records".
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Aixxww/AiT/store"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type u5Row struct {
	EventID string `json:"event_id"`
	States  []struct {
		Record store.HunterV7SignalRecord `json:"record"`
	} `json:"states"`
	TrackedSource string   `json:"tracked_source"`
	FeeNetOutcome string   `json:"fee_net_outcome"`
	FeeNetR       *float64 `json:"fee_net_r"`
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}

func main() {
	u5Path := flag.String("u5", "", "cmd/t2_u5_tags output")
	outPath := flag.String("out", "", "result JSON")
	source := flag.String("source", store.V7CellSourceDefault, "outcome source filter: all, prompt_open_review, watch_audit, shadow")
	flag.Parse()
	if *u5Path == "" || *outPath == "" || !store.ValidV7CellSource(*source) {
		fatal("usage: t2_u12_cells -u5 u5.json -out out.json [-source prompt_open_review]")
	}
	raw, err := os.ReadFile(*u5Path)
	if err != nil {
		fatal("read: %v", err)
	}
	var rows []u5Row
	if err := json.Unmarshal(raw, &rows); err != nil {
		fatal("decode: %v", err)
	}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		fatal("sqlite: %v", err)
	}
	if err := db.AutoMigrate(&store.HunterV7SignalRecord{}); err != nil {
		fatal("migrate: %v", err)
	}
	st := store.NewHunterV7SignalStore(db)
	var recs []store.HunterV7SignalRecord
	from, to := time.Time{}, time.Time{}
	for _, r := range rows {
		if len(r.States) == 0 {
			continue
		}
		rec := r.States[len(r.States)-1].Record
		rec.ID = 0
		rec.OutcomeSource, rec.FeeNetOutcome, rec.FeeNetR = r.TrackedSource, r.FeeNetOutcome, r.FeeNetR
		if from.IsZero() || rec.Timestamp.Before(from) {
			from = rec.Timestamp
		}
		if rec.Timestamp.After(to) {
			to = rec.Timestamp
		}
		recs = append(recs, rec)
	}
	if err := st.CreateBatch(recs); err != nil {
		fatal("insert: %v", err)
	}
	src, err := st.CellSource(from, to.Add(time.Second))
	if err != nil {
		fatal("query: %v", err)
	}
	rule := store.EffectiveV7CellRule(nil)
	out := map[string]any{
		"source":        *source,
		"rule":          rule,
		"hysteresis":    "none",
		"records":       len(recs),
		"queried":       len(src),
		"tables":        store.BuildV7CellTables(src, *source, rule),
		"excluded_no_r": store.V7CellExclusions(src, *source),
	}
	data, _ := json.MarshalIndent(out, "", "  ")
	if err := os.WriteFile(*outPath, data, 0o644); err != nil {
		fatal("write: %v", err)
	}
	fmt.Printf("records %d queried %d\n", len(recs), len(src))
}
