// Package replay implements the M1 replay harness for the task-2 entry
// study: it ports the Python reference semantics of shadow_exec.py
// (decide / limit_fill_scan) and sim_live_replay.py (Arm A: walk_live
// settlement) to Go, and provides a point-in-time multi-timeframe
// snapshot builder over 1-minute bars.
//
// The Python files are the specification. Every function here cites the
// Python source it ports; behaviour differences are bugs in the port,
// not in the reference.
package replay

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// MinuteMs is the length of the source bars in milliseconds.
const MinuteMs int64 = 60000

// Bar is one OHLC bar. OpenTime is the bar open time in milliseconds
// since the Unix epoch; the bar closes at OpenTime+interval.
type Bar struct {
	OpenTime int64
	Open     float64
	High     float64
	Low      float64
	Close    float64
}

// CloseTime returns the time at which a bar of the given interval
// starting at OpenTime is fully closed.
func (b Bar) CloseTime(intervalMs int64) int64 { return b.OpenTime + intervalMs }

// Event is one triggered task-2 signal from the entry-study cache
// (events.json). Field names mirror the Python dict keys.
type Event struct {
	EventID     string  `json:"event_id"`
	Symbol      string  `json:"symbol"`
	Direction   string  `json:"direction"` // "LONG" or "SHORT"
	Setup       string  `json:"setup"`
	TriggeredAt string  `json:"triggered_at"`
	TrigMs      int64   `json:"trig_ms"`
	Entry       float64 `json:"entry"`
	Stop        float64 `json:"stop"`
	TP1         float64 `json:"tp1"`
	TP2         float64 `json:"tp2"`
	Score       float64 `json:"score"`
}

// eventJSON is the on-disk shape of events.json; trig_ms is decoded via
// float64 so exponent/fractional literals cannot break the load.
type eventJSON struct {
	EventID     string  `json:"event_id"`
	Symbol      string  `json:"symbol"`
	Direction   string  `json:"direction"`
	Setup       string  `json:"setup"`
	TriggeredAt string  `json:"triggered_at"`
	TrigMs      float64 `json:"trig_ms"`
	Entry       float64 `json:"entry"`
	Stop        float64 `json:"stop"`
	TP1         float64 `json:"tp1"`
	TP2         float64 `json:"tp2"`
	Score       float64 `json:"score"`
}

// LoadEvents reads events.json from the study directory.
func LoadEvents(dir string) ([]Event, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "events.json"))
	if err != nil {
		return nil, err
	}
	var rows []eventJSON
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("parse events.json: %w", err)
	}
	events := make([]Event, len(rows))
	for i, r := range rows {
		events[i] = Event{
			EventID: r.EventID, Symbol: r.Symbol, Direction: r.Direction,
			Setup: r.Setup, TriggeredAt: r.TriggeredAt,
			TrigMs: int64(math.Round(r.TrigMs)),
			Entry:  r.Entry, Stop: r.Stop, TP1: r.TP1, TP2: r.TP2, Score: r.Score,
		}
	}
	return events, nil
}

// LoadBars reads klines/<index>.json for the event at position index in
// events.json. On disk each bar is a JSON array
// [openTimeMs, open, high, low, close]. Bars are returned sorted by
// OpenTime (the cache is already sorted; the sort is defensive).
func LoadBars(dir string, index int) ([]Bar, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "klines", fmt.Sprintf("%d.json", index)))
	if err != nil {
		return nil, err
	}
	var rows [][]float64
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("parse klines/%d.json: %w", index, err)
	}
	bars := make([]Bar, 0, len(rows))
	for _, r := range rows {
		if len(r) < 5 {
			return nil, fmt.Errorf("klines/%d.json: bar row has %d fields, want >= 5", index, len(r))
		}
		bars = append(bars, Bar{
			OpenTime: int64(r[0]),
			Open:     r[1], High: r[2], Low: r[3], Close: r[4],
		})
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].OpenTime < bars[j].OpenTime })
	return bars, nil
}

// round4 mirrors Python round(x, 4) (round-half-even at 4 decimals),
// used for per-event gross/net R in the reference records.
func round4(x float64) float64 { return math.RoundToEven(x*10000) / 10000 }

// round2 mirrors Python round(x, 2) for aggregate display values.
func round2(x float64) float64 { return math.RoundToEven(x*100) / 100 }

// round1 mirrors Python round(x, 1) for win-rate display values.
func round1(x float64) float64 { return math.RoundToEven(x*10) / 10 }

// math3 mirrors Python round(x, 3) for per-fill average R.
func math3(x float64) float64 { return math.RoundToEven(x*1000) / 1000 }

// ftoa formats a band bound compactly ("0", "0.5", "1").
func ftoa(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
