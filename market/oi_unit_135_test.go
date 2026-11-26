package market

import (
	"strings"
	"testing"
)

// #135: OIData holds contracts; NotionalUSDT is the single conversion and
// Format labels the printed value as USDT notional.
func TestOIUnit135NotionalUSDT(t *testing.T) {
	oi := &OIData{Latest: 40_000, Average: 38_000}
	latest, avg, ok := oi.NotionalUSDT(500)
	if !ok || latest != 20_000_000 || avg != 19_000_000 {
		t.Fatalf("NotionalUSDT = %.0f/%.0f/%v, want 20000000/19000000/true", latest, avg, ok)
	}
	if _, _, ok := oi.NotionalUSDT(0); ok {
		t.Fatal("NotionalUSDT must refuse a non-positive price")
	}
	var nilOI *OIData
	if _, _, ok := nilOI.NotionalUSDT(500); ok {
		t.Fatal("NotionalUSDT must refuse a nil receiver")
	}
}

func TestOIUnit135FormatLabelsNotional(t *testing.T) {
	out := Format(&Data{Symbol: "OIUSDT", CurrentPrice: 500, OpenInterest: &OIData{Latest: 40_000, Average: 38_000}})
	if !strings.Contains(out, "Open Interest (USDT notional): Latest: 20000000 Average: 19000000") {
		t.Fatalf("Format does not print labelled USDT notional:\n%s", out)
	}
	if strings.Contains(out, "40000") {
		t.Fatalf("Format still prints the raw contract count:\n%s", out)
	}
}
