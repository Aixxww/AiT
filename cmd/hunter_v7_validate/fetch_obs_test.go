package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/datafetch"
)

// TA-5: the round report carries the true fetch time, the round time and
// the fetch counters, and a deadline truncation is reported.
func TestSnapshotSummaryCarriesFetchObservability(t *testing.T) {
	snap := &datafetch.Snapshot{Meta: datafetch.SnapshotMeta{
		SymbolCount: 515, FetchDuration: 150 * time.Second, RestErrors: 2,
		DetailRequested: 220, DetailCompleted: 180, DetailSkippedDeadline: 40, FailedRequests: 5,
	}}
	s := annotateSnapshotQuality(buildSnapshotSummary(snap, 140, "rotation", 198*time.Second))
	if s.FetchMs != 150000 || s.RoundMs != 198000 {
		t.Fatalf("fetch_ms %d round_ms %d, want 150000/198000", s.FetchMs, s.RoundMs)
	}
	if s.DetailRequested != 220 || s.DetailCompleted != 180 || s.DetailSkippedDeadline != 40 || s.FailedRequests != 5 {
		t.Fatalf("counters %+v", s)
	}
	if s.DetailRequested != s.DetailCompleted+s.DetailSkippedDeadline {
		t.Fatal("requested must equal completed + skipped")
	}
	b, _ := json.Marshal(s)
	for _, k := range []string{`"fetch_ms":150000`, `"round_ms":198000`, `"detail_requested":220`, `"detail_completed":180`, `"detail_skipped_deadline":40`, `"failed_requests":5`} {
		if !strings.Contains(string(b), k) {
			t.Fatalf("report JSON lacks %s: %s", k, b)
		}
	}
	found := false
	for _, is := range snapshotIssues(s) {
		if is.Code == "detail_truncated_by_deadline" {
			found = is.Severity == "medium" && strings.Contains(is.Detail, "40 of 220")
		}
	}
	if !found {
		t.Fatal("deadline truncation must be reported as a medium issue")
	}
	// Observability only: the contract and the snapshot gate are unaffected.
	if snapshotAcceptanceError(s) != nil || contractSignature(validationReport{FormatCheck: formatCheck{JSONMarshalOK: true, JSONUnmarshalOK: true}, Issues: snapshotIssues(s)}) != "contract:unspecified" {
		t.Fatal("the new issue must not affect acceptance or the contract signature")
	}
	clean := annotateSnapshotQuality(buildSnapshotSummary(&datafetch.Snapshot{Meta: datafetch.SnapshotMeta{SymbolCount: 515, DetailRequested: 220, DetailCompleted: 220}}, 140, "", 0))
	for _, is := range snapshotIssues(clean) {
		if is.Code == "detail_truncated_by_deadline" {
			t.Fatal("no truncation issue without skips")
		}
	}
	md := formatMarkdown(validationReport{Snapshot: s}, "raw.json")
	if !strings.Contains(md, "| Round ms | 198000 |") || !strings.Contains(md, "| Detail requested / completed / skipped (deadline) | 220 / 180 / 40 |") || !strings.Contains(md, "| Failed requests | 5 |") {
		t.Fatal("markdown report lacks the new rows")
	}
}
