package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/Aixxww/AiT/kernel"
	local "github.com/Aixxww/AiT/provider/local"
	"github.com/Aixxww/AiT/store"
	aittrader "github.com/Aixxww/AiT/trader"
)

// Fix 09 (P1-8): the outcome tracker across a resume.
//
// TA-3 saved the watch state and the T2 store but not the outcome tracker,
// so the rows it was tracking when the process stopped stayed ACTIVE in the
// DB for good (17 rows in the rerun). Now the state file also lists the
// record ids the tracker had ACTIVE after each round, and --resume settles
// each of them:
//
//   - re-registered: the row is still ACTIVE in the DB and every 1m bar
//     since its signal time can still be fetched (age <= resumeReregisterMaxAge,
//     inside the 120-bar backfill). It is registered again with the values
//     persistValidationSignals used and walked over those bars, so its
//     outcome (and fee-net walk) is the one an uninterrupted run would give.
//   - RESTART_ORPHANED: the row is still ACTIVE but its bars since the signal
//     can no longer be fetched in full (older than the limit, or the
//     backfill failed / starts late). Only track_status and track_exit_time
//     are written; the other track_* values stay as they were. Its fee-net
//     outcome can be computed afterwards with cmd/v7_fee_net_offline.
//   - already final: the row is no longer ACTIVE (it finished before the
//     stop): left alone.
//   - missing: the id is not in the DB: counted, nothing written.

// trackRestartOrphaned is the DB track_status of a row the harness stopped
// tracking at a restart and could not re-register.
const trackRestartOrphaned = "RESTART_ORPHANED"

// validationBackfillMaxMinutes is the most 1m bars validationBackfillCandles
// fetches (datafetch FetchKlines takes a limit, not a start time).
const validationBackfillMaxMinutes = 120

// resumeReregisterMaxAge is the oldest signal that can be re-registered:
// the backfill fetches age + 3 minutes, capped at 120 bars, so a signal up
// to 117 minutes old is fully covered; 5 minutes of margin for the time
// between this check and the fetch.
const resumeReregisterMaxAge = (validationBackfillMaxMinutes - 5) * time.Minute

// resumeTrackingSummary is reported in the run summary.
type resumeTrackingSummary struct {
	SavedActive   int            `json:"saved_active"`
	Reregistered  []int64        `json:"reregistered,omitempty"`
	Orphaned      []int64        `json:"restart_orphaned,omitempty"`
	AlreadyFinal  int            `json:"already_final"`
	Missing       int            `json:"missing"`
	OrphanReasons map[string]int `json:"orphan_reasons,omitempty"`
	Note          string         `json:"note,omitempty"`
}

// trackedActiveIDs lists the record ids the tracker has ACTIVE, sorted.
func trackedActiveIDs(tr *aittrader.SignalOutcomeTracker) []int64 {
	if tr == nil {
		return nil
	}
	var ids []int64
	for _, sig := range tr.GetActive() {
		if sig != nil && sig.RecordID > 0 {
			ids = append(ids, sig.RecordID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// restoreTrackedOutcomes settles the ids saved as ACTIVE in the prior state
// (see the header). It runs once, after the state is loaded and before the
// first resumed round. backfill fetches 1m bars for [from, to].
func restoreTrackedOutcomes(opts validationOptions, prior harnessState, now time.Time,
	backfill func(symbol string, from, to time.Time) []aittrader.TrackedCandle) (*resumeTrackingSummary, error) {
	sum := &resumeTrackingSummary{OrphanReasons: map[string]int{}}
	defer sum.sortIDs()
	if prior.TrackedActiveIDs == nil {
		sum.Note = "state file has no tracked_active_ids (saved before fix 09): ACTIVE rows from before the restart cannot be identified"
		return sum, nil
	}
	sum.SavedActive = len(prior.TrackedActiveIDs)
	if len(prior.TrackedActiveIDs) == 0 || opts.store == nil {
		return sum, nil
	}
	db := opts.store.GormDB()
	var rows []store.HunterV7SignalRecord
	if err := db.Where("id IN ?", prior.TrackedActiveIDs).Order("timestamp ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load saved ACTIVE rows: %w", err)
	}
	sum.Missing = len(prior.TrackedActiveIDs) - len(rows)

	orphan := func(r store.HunterV7SignalRecord, reason string) error {
		exit := now.UTC()
		// Only rows still ACTIVE; only the two columns.
		res := db.Model(&store.HunterV7SignalRecord{}).Where("id = ? AND track_status = ?", r.ID, string(aittrader.TrackedActive)).
			Updates(map[string]interface{}{"track_status": trackRestartOrphaned, "track_exit_time": &exit})
		if res.Error != nil {
			return fmt.Errorf("mark row %d %s: %w", r.ID, trackRestartOrphaned, res.Error)
		}
		if res.RowsAffected == 1 {
			sum.Orphaned = append(sum.Orphaned, r.ID)
			sum.OrphanReasons[reason]++
		}
		return nil
	}

	bars := map[int64][]aittrader.TrackedCandle{}
	symbolOf := map[int64]string{}
	var toRegister []store.HunterV7SignalRecord
	for _, r := range rows {
		if r.TrackStatus != string(aittrader.TrackedActive) {
			sum.AlreadyFinal++
			continue
		}
		if opts.outcomeTracker == nil {
			if err := orphan(r, "tracking_disabled"); err != nil {
				return nil, err
			}
			continue
		}
		if now.Sub(r.Timestamp) > resumeReregisterMaxAge {
			if err := orphan(r, "older_than_backfill"); err != nil {
				return nil, err
			}
			continue
		}
		cs := backfill(r.Symbol, r.Timestamp, now)
		if !candlesCoverFrom(cs, r.Timestamp) {
			if err := orphan(r, "backfill_incomplete"); err != nil {
				return nil, err
			}
			continue
		}
		bars[r.ID], symbolOf[r.ID] = cs, r.Symbol
		toRegister = append(toRegister, r)
	}

	if len(toRegister) == 0 {
		return sum, nil
	}
	// Serve each re-registered row its own prefetched bars for this one
	// tick, then drop the source (the next round sets its snapshot source).
	bySymbol := map[string][]aittrader.TrackedCandle{}
	for id, cs := range bars {
		s := symbolOf[id]
		if len(cs) > len(bySymbol[s]) {
			bySymbol[s] = cs // rows of one symbol: the longest window covers the others
		}
	}
	opts.outcomeTracker.SetCandleHistorySource(func(symbol string, since time.Time) []aittrader.TrackedCandle {
		var out []aittrader.TrackedCandle
		for _, c := range bySymbol[symbol] {
			if c.T.After(since) {
				out = append(out, c)
			}
		}
		return out
	})
	for _, r := range toRegister {
		ok, replacedID := opts.outcomeTracker.RegisterWithMeta(
			aittrader.TrackedSignalMeta{OutcomeSource: r.OutcomeSource, TriggerPrice: floatOrZero(r.TriggerPrice)},
			r.ID, r.Symbol, r.Direction, r.SetupType, r.ExecutionTier,
			resumeEntryPrice(r), r.InvalidationPrice, r.TP0Price, r.TP1Price, r.TP2Price,
			r.Timestamp,
		)
		if !ok {
			if err := orphan(r, "tracker_refused"); err != nil {
				return nil, err
			}
			continue
		}
		if replacedID > 0 {
			exit := now.UTC()
			if err := opts.store.HunterV7Signal().UpdateTrackOutcome(replacedID, store.HunterV7SignalTrackUpdate{Status: "DUPLICATE_CONTEXT", ExitTime: &exit}); err != nil {
				log.Printf("WARN mark validation V7 duplicate context failed: %v", err)
			}
			sum.Reregistered = removeID(sum.Reregistered, replacedID)
		}
		sum.Reregistered = append(sum.Reregistered, r.ID)
	}
	opts.outcomeTracker.TickNow()
	opts.outcomeTracker.SetCandleHistorySource(nil)
	return sum, nil
}

// candlesCoverFrom reports whether the bars start no later than the first
// bar the tracker reads after the signal (the one closing at the end of the
// minute after the signal minute), i.e. none of the walk is missing.
func candlesCoverFrom(cs []aittrader.TrackedCandle, signal time.Time) bool {
	if len(cs) == 0 {
		return false
	}
	first := cs[0].T
	for _, c := range cs[1:] {
		if c.T.Before(first) {
			first = c.T
		}
	}
	return !first.After(signal.Truncate(time.Minute).Add(2 * time.Minute))
}

// resumeEntryPrice is the entry persistValidationSignals registered:
// kernel.HunterV7SignalEntryPrice of the stored signal (raw_json), else the
// stored entry-zone mid.
func resumeEntryPrice(r store.HunterV7SignalRecord) float64 {
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

func floatOrZero(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func removeID(ids []int64, id int64) []int64 {
	out := ids[:0]
	for _, x := range ids {
		if x != id {
			out = append(out, x)
		}
	}
	return out
}

func (s *resumeTrackingSummary) sortIDs() {
	sort.Slice(s.Reregistered, func(i, j int) bool { return s.Reregistered[i] < s.Reregistered[j] })
	sort.Slice(s.Orphaned, func(i, j int) bool { return s.Orphaned[i] < s.Orphaned[j] })
}

func (s *resumeTrackingSummary) logLine() string {
	if s == nil {
		return ""
	}
	if s.Note != "" {
		return "resume tracking: " + s.Note
	}
	return fmt.Sprintf("resume tracking: %d saved ACTIVE: %d re-registered, %d %s %v, %d already final, %d missing",
		s.SavedActive, len(s.Reregistered), len(s.Orphaned), trackRestartOrphaned, s.OrphanReasons, s.AlreadyFinal, s.Missing)
}
