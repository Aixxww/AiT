package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TA-6 (docs/tier-a-improvement-plan-20261120.md, P5): record failed REST
// requests, with path and query, so the failing URL is visible in the
// harness output. Harness side only; the relay is not touched.
//
// Records go to <out-dir>/fetch-errors.jsonl, one JSON object per line.
// To bound the file on long runs, each (round, source) keeps its first
// fetchErrorKeepFirst failures in full, then every fetchErrorSampleEvery-th;
// the totals are always counted (fetchErrorLog.counts) and reported in the
// round report.

const (
	fetchErrorKeepFirst   = 50
	fetchErrorSampleEvery = 100

	fetchSourceRound    = "round_fetch"
	fetchSourceBackfill = "tracker_backfill"
)

type fetchErrorRecord struct {
	TS       string `json:"ts"`
	Round    int    `json:"round"` // 0 for tracker backfill (not tied to a round)
	Attempt  int    `json:"attempt,omitempty"`
	Source   string `json:"source"`
	Path     string `json:"path"` // request path including the query string
	NonASCII bool   `json:"non_ascii"`
	Error    string `json:"error"`
	Seq      int    `json:"seq"` // 1-based index of this failure within its (round, source)
}

type fetchErrorLog struct {
	mu     sync.Mutex
	path   string
	counts map[string]int // "round/source" -> failures seen
	now    func() time.Time
}

func newFetchErrorLog(outDir string) *fetchErrorLog {
	return &fetchErrorLog{path: filepath.Join(outDir, "fetch-errors.jsonl"), counts: map[string]int{}, now: time.Now}
}

// sink returns a datafetch ErrorSink bound to one round/attempt and source.
func (l *fetchErrorLog) sink(round, attempt int, source string) func(path string, err error) {
	if l == nil {
		return nil
	}
	return func(path string, err error) { l.record(round, attempt, source, path, err) }
}

func (l *fetchErrorLog) record(round, attempt int, source, path string, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := fmt.Sprintf("%d/%s", round, source)
	l.counts[key]++
	n := l.counts[key]
	if n > fetchErrorKeepFirst && n%fetchErrorSampleEvery != 0 {
		return
	}
	rec := fetchErrorRecord{
		TS: l.now().UTC().Format(time.RFC3339Nano), Round: round, Attempt: attempt, Source: source,
		Path: path, NonASCII: hasNonASCII(path), Error: err.Error(), Seq: n,
	}
	b, merr := json.Marshal(rec) // escapes any non-UTF-8 / control bytes safely
	if merr != nil {
		return
	}
	if dir := filepath.Dir(l.path); dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	f, oerr := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if oerr != nil {
		log.Printf("WARN fetch-errors log: %v", oerr)
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

// count returns the failures seen for one round and source.
func (l *fetchErrorLog) count(round int, source string) int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.counts[fmt.Sprintf("%d/%s", round, source)]
}

func hasNonASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return true
		}
	}
	return false
}
