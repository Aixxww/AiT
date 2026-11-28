package market

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestU5cCalculateTimeframeSeriesFullHistoryVWAP20AndEMA20(t *testing.T) {
	// 200 raw klines sliced at default PrimaryCount=20 -> 20 retained klines,
	// full-history EMA20Values and VWAP20Values aligned with retained Klines.
	raw := make([]Kline, 200)
	for i := range raw {
		c := 100.0 + float64(i)*0.25 + 2*math.Sin(float64(i)/3)
		vol := 100.0 + float64((i%7)*50)
		raw[i] = Kline{
			OpenTime:  int64(i) * 900_000,
			CloseTime: int64(i+1)*900_000 - 1,
			Open:      c - 0.2,
			High:      c + 0.8,
			Low:       c - 0.7,
			Close:     c,
			Volume:    vol,
		}
	}

	s := calculateTimeframeSeries(raw, "15m", 20)
	if len(s.Klines) != 20 {
		t.Fatalf("len(Klines) = %d, want 20", len(s.Klines))
	}
	if len(s.EMA20Values) != 20 {
		t.Fatalf("len(EMA20Values) = %d, want 20", len(s.EMA20Values))
	}
	if len(s.VWAP20Values) != 20 {
		t.Fatalf("len(VWAP20Values) = %d, want 20", len(s.VWAP20Values))
	}

	// Bar index 18 in retained Klines is raw[198] (the 19th/last closed bar when raw[199] is forming).
	wantEMA := calculateEMA(raw[:199], 20)
	wantVWAP := calculateVWAP(raw[179:199])
	if math.Abs(s.EMA20Values[18]-wantEMA) > 1e-12 {
		t.Fatalf("EMA20Values[18] = %.12f, want %.12f", s.EMA20Values[18], wantEMA)
	}
	if math.Abs(s.VWAP20Values[18]-wantVWAP) > 1e-12 {
		t.Fatalf("VWAP20Values[18] = %.12f, want %.12f", s.VWAP20Values[18], wantVWAP)
	}

	// VWAP20Values carries json:"-" and must not appear in serialized JSON.
	rawJSON, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if strings.Contains(string(rawJSON), "vwap20") || strings.Contains(string(rawJSON), "VWAP20") {
		t.Fatalf("VWAP20Values must be omitted from JSON, got: %s", string(rawJSON))
	}

	// Warmup / suffix-aligned case: 38 raw klines with count=20 -> start=18,
	// bar i=18 lacks 20-bar history, bars i=19..37 produce 19 right-aligned values.
	shortRaw := raw[:38]
	sShort := calculateTimeframeSeries(shortRaw, "5m", 20)
	if len(sShort.Klines) != 20 || len(sShort.EMA20Values) != 19 || len(sShort.VWAP20Values) != 19 {
		t.Fatalf("suffix-aligned lengths = klines:%d ema:%d vwap:%d, want 20/19/19",
			len(sShort.Klines), len(sShort.EMA20Values), len(sShort.VWAP20Values))
	}
	wantShortEMA := calculateEMA(shortRaw[:37], 20)
	wantShortVWAP := calculateVWAP(shortRaw[17:37])
	// Retained Klines[18] is shortRaw[36]; right-aligned index is 19 - (20 - 18) = 17.
	if math.Abs(sShort.EMA20Values[17]-wantShortEMA) > 1e-12 {
		t.Fatalf("short EMA20Values[17] = %.12f, want %.12f", sShort.EMA20Values[17], wantShortEMA)
	}
	if math.Abs(sShort.VWAP20Values[17]-wantShortVWAP) > 1e-12 {
		t.Fatalf("short VWAP20Values[17] = %.12f, want %.12f", sShort.VWAP20Values[17], wantShortVWAP)
	}
}
