package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/Aixxww/AiT/market"
)

// bar is one public 1m kline: open time (ms) and OHLC.
type bar struct {
	OpenMs int64   `json:"t"`
	Open   float64 `json:"o"`
	High   float64 `json:"h"`
	Low    float64 `json:"l"`
	Close  float64 `json:"c"`
}

// rangeFetcher fetches public 1m klines for [start, end). The default is
// market.GetKlinesRange (fapi /fapi/v1/klines with startTime/endTime, with
// the data.binance.vision archive as fallback). Tests inject a fake.
type rangeFetcher func(symbol string, start, end time.Time) ([]bar, error)

func publicRangeFetcher(symbol string, start, end time.Time) ([]bar, error) {
	ks, err := market.GetKlinesRange(symbol, "1m", start, end)
	if err != nil {
		return nil, err
	}
	out := make([]bar, 0, len(ks))
	for _, k := range ks {
		out = append(out, bar{OpenMs: k.OpenTime, Open: k.Open, High: k.High, Low: k.Low, Close: k.Close})
	}
	return out, nil
}

// klineStore serves 1m bars per symbol from a per-UTC-day file cache
// (<dir>/<SYMBOL>/<YYYY-MM-DD>.json), fetching missing days unless offline.
// Only complete days (ended before now - 10 min) are written to the cache.
type klineStore struct {
	dir     string
	offline bool
	fetch   rangeFetcher
	now     func() time.Time
	mem     map[string][]bar // symbol|day -> bars
	Fetched int              `json:"-"`
	Cached  int              `json:"-"`
	Errors  map[string]string
}

func newKlineStore(dir string, offline bool, fetch rangeFetcher) *klineStore {
	return &klineStore{dir: dir, offline: offline, fetch: fetch, now: time.Now, mem: map[string][]bar{}, Errors: map[string]string{}}
}

func (s *klineStore) dayPath(symbol string, day time.Time) string {
	return filepath.Join(s.dir, symbol, day.Format("2006-01-02")+".json")
}

func (s *klineStore) day(symbol string, day time.Time) ([]bar, error) {
	key := symbol + "|" + day.Format("2006-01-02")
	if b, ok := s.mem[key]; ok {
		return b, nil
	}
	path := s.dayPath(symbol, day)
	if raw, err := os.ReadFile(path); err == nil {
		var bars []bar
		if err := json.Unmarshal(raw, &bars); err != nil {
			return nil, fmt.Errorf("cache %s: %w", path, err)
		}
		s.mem[key] = bars
		s.Cached++
		return bars, nil
	}
	if s.offline {
		return nil, fmt.Errorf("no cached klines for %s on %s (offline)", symbol, day.Format("2006-01-02"))
	}
	end := day.Add(24 * time.Hour)
	bars, err := s.fetch(symbol, day, end)
	if err != nil {
		return nil, err
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].OpenMs < bars[j].OpenMs })
	s.Fetched++
	s.mem[key] = bars
	if end.Before(s.now().Add(-10*time.Minute)) && len(bars) > 0 {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
			if data, err := json.Marshal(bars); err == nil {
				_ = os.WriteFile(path, data, 0o644)
			}
		}
	}
	return bars, nil
}

// window returns the bars opening in [from, to], from the day files that
// cover it.
func (s *klineStore) window(symbol string, from, to time.Time) ([]bar, error) {
	var out []bar
	for d := from.UTC().Truncate(24 * time.Hour); !d.After(to); d = d.Add(24 * time.Hour) {
		bars, err := s.day(symbol, d)
		if err != nil {
			s.Errors[symbol+" "+d.Format("2006-01-02")] = err.Error()
			return nil, err
		}
		for _, b := range bars {
			t := time.UnixMilli(b.OpenMs)
			if !t.Before(from) && !t.After(to) {
				out = append(out, b)
			}
		}
	}
	return out, nil
}
