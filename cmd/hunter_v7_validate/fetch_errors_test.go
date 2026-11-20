package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Aixxww/AiT/datafetch"
)

func readFetchErrors(t *testing.T, path string) []fetchErrorRecord {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []fetchErrorRecord
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r fetchErrorRecord
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("bad jsonl line %q: %v", sc.Text(), err)
		}
		out = append(out, r)
	}
	return out
}

// TA-6 end to end: a failed request from the round fetcher and from the
// tracker backfill fetcher lands in <out-dir>/fetch-errors.jsonl with its
// path and query.
func TestFailedRequestsRecordedWithPathAndQuery(t *testing.T) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Skipf("local TCP listener unavailable: %v", err)
	}
	ln.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, `{"code":-1,"msg":"relay upstream error"}`)
	}))
	defer srv.Close()
	dir := t.TempDir()
	opts := validationOptions{outDir: dir, binanceURL: srv.URL, topDetail: 5, maxWorkers: 1, attempt: 2, fetchErrors: newFetchErrorLog(dir)}

	rc := roundFetcherConfig(opts, false, false)
	rc.ErrorSink = opts.fetchErrors.sink(7, opts.attempt, fetchSourceRound)
	if _, err := datafetch.NewDataFetcher(rc).FetchKlines(context.Background(), "CRVUSDT", "15m", 120); err == nil {
		t.Fatal("expected failure")
	}
	if _, err := datafetch.NewDataFetcher(backfillFetcherConfig(opts)).FetchKlines(context.Background(), "IDOLUSDT", "1m", 120); err == nil {
		t.Fatal("expected failure")
	}
	recs := readFetchErrors(t, filepath.Join(dir, "fetch-errors.jsonl"))
	if len(recs) != 2 {
		t.Fatalf("records %d, want 2: %+v", len(recs), recs)
	}
	r0, r1 := recs[0], recs[1]
	if r0.Source != fetchSourceRound || r0.Round != 7 || r0.Attempt != 2 || r0.Path != "/fapi/v1/klines?symbol=CRVUSDT&interval=15m&limit=120" || r0.NonASCII || r0.Error == "" || r0.Seq != 1 {
		t.Fatalf("round record %+v", r0)
	}
	if r1.Source != fetchSourceBackfill || r1.Round != 0 || r1.Path != "/fapi/v1/klines?symbol=IDOLUSDT&interval=1m&limit=120" {
		t.Fatalf("backfill record %+v", r1)
	}
	if opts.fetchErrors.count(7, fetchSourceRound) != 1 || opts.fetchErrors.count(0, fetchSourceBackfill) != 1 {
		t.Fatal("counts")
	}
}

// Non-ASCII bytes in a path (the open P5 question) are flagged, and the
// JSON line stays valid.
func TestFetchErrorLogFlagsNonASCIIAndSamples(t *testing.T) {
	dir := t.TempDir()
	l := newFetchErrorLog(dir)
	sink := l.sink(3, 1, fetchSourceRound)
	sink("/fapi/v1/klines?symbol=币安USDT&interval=1m&limit=1", errors.New("EOF"))
	for i := 0; i < 250; i++ {
		sink(fmt.Sprintf("/fapi/v1/openInterest?symbol=S%dUSDT", i), errors.New("EOF"))
	}
	recs := readFetchErrors(t, filepath.Join(dir, "fetch-errors.jsonl"))
	if !recs[0].NonASCII || recs[0].Path != "/fapi/v1/klines?symbol=币安USDT&interval=1m&limit=1" {
		t.Fatalf("non-ASCII record %+v", recs[0])
	}
	// 251 failures: first 50 kept, then seq 100, 200 sampled.
	if len(recs) != 52 || recs[50].Seq != 100 || recs[51].Seq != 200 {
		t.Fatalf("kept %d records (last seqs %d,%d), want 52 with 100/200 sampled", len(recs), recs[len(recs)-2].Seq, recs[len(recs)-1].Seq)
	}
	if l.count(3, fetchSourceRound) != 251 {
		t.Fatalf("count %d, want 251 (all failures counted)", l.count(3, fetchSourceRound))
	}
	var nilLog *fetchErrorLog
	if nilLog.sink(1, 1, fetchSourceRound) != nil || nilLog.count(1, fetchSourceRound) != 0 {
		t.Fatal("nil log must be inert")
	}
}

func TestReportShowsFailedRequestLog(t *testing.T) {
	md := formatMarkdown(validationReport{Snapshot: snapshotSummary{FailedRequests: 3, FetchErrorLog: "out/fetch-errors.jsonl"}}, "raw.json")
	if !strings.Contains(md, "| Failed request log (path+query) | `out/fetch-errors.jsonl` |") {
		t.Fatal("markdown lacks the failed-request log row")
	}
	if md2 := formatMarkdown(validationReport{}, "raw.json"); strings.Contains(md2, "Failed request log") {
		t.Fatal("row must be absent without failures")
	}
}
