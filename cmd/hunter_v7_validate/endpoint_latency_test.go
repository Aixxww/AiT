package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Aixxww/AiT/datafetch"
)

// Fix 02: the round report carries the per-endpoint latency table.
func TestReportCarriesEndpointLatency(t *testing.T) {
	snap := &datafetch.Snapshot{Meta: datafetch.SnapshotMeta{SymbolCount: 515, EndpointLatency: map[string]datafetch.EndpointLatency{
		"/fapi/v1/klines":       {Calls: 1320, Attempts: 1331, Failed: 2, NetP50Ms: 310, NetP95Ms: 920, NetMaxMs: 4100, LimiterWaitP50Ms: 35, LimiterWaitP95Ms: 140, CallP50Ms: 360, CallP95Ms: 1200},
		"/fapi/v1/openInterest": {Calls: 220, Attempts: 220, NetP50Ms: 280, NetP95Ms: 700},
	}}}
	s := buildSnapshotSummary(snap, 280, "trend_down", 0)
	b, _ := json.Marshal(s)
	if !strings.Contains(string(b), `"endpoint_latency":{"/fapi/v1/klines":{"calls":1320,"attempts":1331,"failed":2,"net_p50_ms":310,"net_p95_ms":920`) {
		t.Fatalf("report JSON: %s", b)
	}
	md := formatMarkdown(validationReport{Snapshot: s}, "raw.json")
	if !strings.Contains(md, "| `/fapi/v1/klines` | 1320 | 1331 | 2 | 310 / 920 / 4100 | 35 / 140 | 360 / 1200 |") ||
		!strings.Contains(md, "| Endpoint | Calls | Attempts | Failed |") {
		t.Fatal("markdown lacks the endpoint latency table")
	}
	if strings.Index(md, "`/fapi/v1/klines`") > strings.Index(md, "`/fapi/v1/openInterest`") {
		t.Fatal("endpoints must be sorted")
	}
	if md2 := formatMarkdown(validationReport{}, "raw.json"); strings.Contains(md2, "| Endpoint |") {
		t.Fatal("no table without data")
	}
}
