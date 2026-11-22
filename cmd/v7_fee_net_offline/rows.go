package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Aixxww/AiT/kernel"
	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openReadOnly opens a SQLite database for reading only: the file must
// exist, the connection is opened with mode=ro and query_only, so any write
// fails. store.New is not used because it creates tables and seeds data.
func openReadOnly(path string) (*gorm.DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("database %s: %w", abs, err)
	}
	dsn := (&url.URL{Scheme: "file", Path: abs, RawQuery: "mode=ro&_query_only=1"}).String()
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.Exec("PRAGMA query_only = ON").Error; err != nil {
		return nil, fmt.Errorf("set query_only: %w", err)
	}
	return db, nil
}

// rowFilter selects the stored rows to replay.
type rowFilter struct {
	From, To      time.Time // zero = unbounded; To is exclusive
	MinID, MaxID  int64     // 0 = unbounded
	Tiers         []string  // empty = all
	Setups        []string  // canonical setup names; empty = all
	IncludeNoTier bool      // include rows with an empty execution_tier (router-filtered)
}

func loadRecords(db *gorm.DB, f rowFilter) ([]store.HunterV7SignalRecord, error) {
	q := db.Model(&store.HunterV7SignalRecord{})
	if !f.From.IsZero() {
		q = q.Where("timestamp >= ?", f.From)
	}
	if !f.To.IsZero() {
		q = q.Where("timestamp < ?", f.To)
	}
	if f.MinID > 0 {
		q = q.Where("id >= ?", f.MinID)
	}
	if f.MaxID > 0 {
		q = q.Where("id <= ?", f.MaxID)
	}
	if len(f.Tiers) > 0 {
		q = q.Where("execution_tier IN ?", f.Tiers)
	} else if !f.IncludeNoTier {
		q = q.Where("execution_tier <> ''")
	}
	var recs []store.HunterV7SignalRecord
	if err := q.Order("timestamp ASC, id ASC").Find(&recs).Error; err != nil {
		return nil, err
	}
	out := recs[:0]
	for _, r := range recs {
		canon := store.V7CanonicalSetupType(r.SetupType)
		if len(f.Setups) > 0 && !containsFold(f.Setups, canon) {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func containsFold(list []string, v string) bool {
	for _, s := range list {
		if strings.EqualFold(strings.TrimSpace(s), v) {
			return true
		}
	}
	return false
}

// inputRow is one stored row with the values the live tracker registers
// (kernel BuildHunterV7SignalDBRecords + trader registration): stop =
// invalidation_price, TP1 = tp1_price, trigger = trigger_price, signal time
// = timestamp, entry (PnL path only) = entry-zone mid, else price_context.last.
type inputRow struct {
	ID            int64     `json:"id"`
	Time          time.Time `json:"timestamp"`
	Symbol        string    `json:"symbol"`
	Direction     string    `json:"direction"`
	Setup         string    `json:"setup_type"`
	CanonSetup    string    `json:"canonical_setup"`
	Tier          string    `json:"execution_tier"`
	TierReason    string    `json:"tier_reason"`
	Stack         string    `json:"stack"`
	Class         string    `json:"signal_class"`
	ClassSource   string    `json:"signal_class_source"` // "stored" or "registry"
	Admission     string    `json:"admission,omitempty"`
	RRBand        string    `json:"rr_band"`
	Regime        string    `json:"market_regime"`
	OutcomeSource string    `json:"outcome_source"`

	Entry   float64 `json:"entry"`
	Stop    float64 `json:"stop"`
	TP0     float64 `json:"tp0,omitempty"`
	TP1     float64 `json:"tp1"`
	TP1From string  `json:"tp1_from"` // tp1_price, target_1 (only with -tp1-fallback-target1) or ""
	TP2     float64 `json:"tp2,omitempty"`
	Trigger float64 `json:"trigger_price,omitempty"`

	StoredOutcome string   `json:"stored_fee_net_outcome,omitempty"`
	StoredR       *float64 `json:"stored_fee_net_r,omitempty"`
	StoredEntry   float64  `json:"stored_fee_net_entry,omitempty"`
}

func buildInputRow(r store.HunterV7SignalRecord, tp1FallbackTarget1 bool, classes map[string]store.V7SignalClassEntry) inputRow {
	in := inputRow{
		ID: r.ID, Time: r.Timestamp, Symbol: r.Symbol, Direction: strings.ToUpper(r.Direction),
		Setup: r.SetupType, CanonSetup: store.V7CanonicalSetupType(r.SetupType),
		Tier: r.ExecutionTier, TierReason: r.TierReason, Stack: r.Stack,
		Class: r.SignalClass, ClassSource: "stored", RRBand: r.RRBand, Regime: r.MarketRegime,
		OutcomeSource: r.OutcomeSource,
		Stop:          r.InvalidationPrice, TP0: r.TP0Price, TP1: r.TP1Price, TP2: r.TP2Price,
		StoredOutcome: r.FeeNetOutcome, StoredR: r.FeeNetR,
	}
	if in.TP1 > 0 {
		in.TP1From = "tp1_price"
	} else if tp1FallbackTarget1 && r.Target1 > 0 {
		in.TP1, in.TP1From = r.Target1, "target_1"
	}
	if r.TriggerPrice != nil {
		in.Trigger = *r.TriggerPrice
	}
	in.StoredEntry = r.FeeNetEntry
	if in.Stack == "" {
		in.Stack = store.V7StackNative
	}
	entry, ok := store.LookupV7Setup(classes, r.SetupType)
	if ok {
		in.Admission = entry.Admission
	}
	if in.Class == "" {
		if ok {
			in.Class, in.ClassSource = entry.Class, "registry"
		} else {
			in.Class, in.ClassSource = store.V7ClassUnassigned, "registry"
		}
	}
	if in.RRBand == "" {
		in.RRBand = store.V7BandUnknown
	}
	if in.Regime == "" {
		in.Regime = store.V7CellRegimeUnknown
	}
	in.Entry = rowEntryPrice(r)
	return in
}

// rowEntryPrice is the tracker's registered entry (PnL path only; the
// fee-net walk anchors on the next 1m close instead): the signal's
// HunterV7SignalEntryPrice from raw_json, else the stored entry-zone mid.
func rowEntryPrice(r store.HunterV7SignalRecord) float64 {
	if strings.TrimSpace(r.RawJSON) != "" {
		var sig local.V7SignalOutput
		if json.Unmarshal([]byte(r.RawJSON), &sig) == nil {
			if e := kernel.HunterV7SignalEntryPrice(sig); e > 0 {
				return e
			}
		}
	}
	if r.EntryZoneLower > 0 && r.EntryZoneUpper >= r.EntryZoneLower {
		return (r.EntryZoneLower + r.EntryZoneUpper) / 2
	}
	return 0
}
