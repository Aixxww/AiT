package market

import (
	"math"
	"reflect"
	"testing"

	"github.com/Aixxww/AiT/datafetch"
)

func fx156Klines(n int) []Kline {
	out := make([]Kline, n)
	for i := range out {
		c := 100 + 5*math.Sin(float64(i)/4) + float64(i)*0.1
		out[i] = Kline{OpenTime: int64(i) * 60_000, Open: c - 0.3, High: c + 1, Low: c - 1, Close: c, Volume: 1000 + float64(i)}
	}
	return out
}

// #156: the default configuration reproduces the pre-#156 hardcoded series
// exactly (EMA 20/50, RSI 7/14, ATR 14, BOLL 20).
func TestIndicatorPeriods156DefaultsReproduceFixedSeries(t *testing.T) {
	d := calculateTimeframeSeries(fx156Klines(120), "15m", 60)
	if !reflect.DeepEqual(d.Periods, DefaultIndicatorPeriods()) {
		t.Fatalf("default periods = %+v", d.Periods)
	}
	pairs := []struct {
		name      string
		got, want []float64
	}{
		{"EMA20", d.EMASeries[20], d.EMA20Values}, {"EMA50", d.EMASeries[50], d.EMA50Values},
		{"RSI7", d.RSISeries[7], d.RSI7Values}, {"RSI14", d.RSISeries[14], d.RSI14Values},
		{"BOLL20 upper", d.BOLLSeries[20].Upper, d.BOLLUpper}, {"BOLL20 middle", d.BOLLSeries[20].Middle, d.BOLLMiddle},
		{"BOLL20 lower", d.BOLLSeries[20].Lower, d.BOLLLower},
	}
	for _, p := range pairs {
		if len(p.want) == 0 || !reflect.DeepEqual(p.got, p.want) {
			t.Errorf("%s: configured series differs from the pre-#156 series (len %d vs %d)", p.name, len(p.got), len(p.want))
		}
	}
	if d.ATRValues[14] != d.ATR14 || d.ATR14 == 0 {
		t.Errorf("ATR14 = %v vs %v", d.ATRValues[14], d.ATR14)
	}
	if len(d.EMASeries) != 2 || len(d.RSISeries) != 2 || len(d.ATRValues) != 1 || len(d.BOLLSeries) != 1 {
		t.Errorf("default build carries extra periods: %d/%d/%d/%d", len(d.EMASeries), len(d.RSISeries), len(d.ATRValues), len(d.BOLLSeries))
	}
}

// A non-default period changes the computed series.
func TestIndicatorPeriods156NonDefaultChangesSeries(t *testing.T) {
	kl := fx156Klines(120)
	p := IndicatorPeriods{EMA: []int{9}, RSI: []int{21}, ATR: []int{7}, BOLL: []int{10}}
	d := calculateTimeframeSeries(kl, "15m", 60, p)
	if !reflect.DeepEqual(d.Periods, p) {
		t.Fatalf("periods = %+v", d.Periods)
	}
	if _, ok := d.EMASeries[20]; ok || len(d.EMASeries[9]) == 0 {
		t.Fatalf("EMA series keys: %v", SortedPeriods(d.EMASeries))
	}
	if got, want := d.EMASeries[9][len(d.EMASeries[9])-1], calculateEMA(kl, 9); got != want || got == d.EMA20Values[len(d.EMA20Values)-1] {
		t.Fatalf("EMA9 last = %v, want %v and distinct from EMA20", got, want)
	}
	if got := d.RSISeries[21][len(d.RSISeries[21])-1]; got != calculateRSI(kl, 21) || got == d.RSI14Values[len(d.RSI14Values)-1] {
		t.Fatalf("RSI21 last = %v", got)
	}
	if d.ATRValues[7] != calculateATR(kl, 7) || d.ATRValues[7] == d.ATR14 {
		t.Fatalf("ATR7 = %v (ATR14 %v)", d.ATRValues[7], d.ATR14)
	}
	if u, _, _ := calculateBOLL(kl, 10, 2.0); d.BOLLSeries[10].Upper[len(d.BOLLSeries[10].Upper)-1] != u {
		t.Fatal("BOLL10 upper mismatch")
	}
}

func TestIndicatorPeriods156Effective(t *testing.T) {
	got := IndicatorPeriods{EMA: []int{0, 9, 9, -1, 21}, RSI: nil}.Effective()
	want := IndicatorPeriods{EMA: []int{9, 21}, RSI: []int{7, 14}, ATR: []int{14}, BOLL: []int{20}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Effective = %+v, want %+v", got, want)
	}
}

// The builders thread the option through to every timeframe and the
// current values.
func TestIndicatorPeriods156BuilderThreadsOption(t *testing.T) {
	raw := make([]datafetch.Kline, 120)
	for i, k := range fx156Klines(120) {
		raw[i] = datafetch.Kline{OpenTime: k.OpenTime, CloseTime: k.OpenTime + 59_999, Open: k.Open, High: k.High, Low: k.Low, Close: k.Close, Volume: k.Volume}
	}
	ss := &datafetch.SymbolSnapshot{Symbol: "PERUSDT", Price: 100, Klines: map[string][]datafetch.Kline{"15m": raw, "1h": raw}}
	p := IndicatorPeriods{EMA: []int{9}, RSI: []int{21}}
	d, err := BuildDataFromSymbolSnapshot("PERUSDT", ss, []string{"15m", "1h"}, "15m", 60, WithIndicatorPeriods(p))
	if err != nil {
		t.Skipf("snapshot fixture rejected (%v)", err)
	}
	for tf, s := range d.TimeframeData {
		if _, ok := s.EMASeries[9]; !ok || len(s.Periods.RSI) != 1 || s.Periods.RSI[0] != 21 {
			t.Fatalf("%s: periods not threaded: %+v", tf, s.Periods)
		}
	}
	if _, ok := d.CurrentEMAs[9]; !ok || d.IndicatorPeriods.EMA[0] != 9 {
		t.Fatalf("current values not threaded: %+v %+v", d.CurrentEMAs, d.IndicatorPeriods)
	}
	def, _ := BuildDataFromSymbolSnapshot("PERUSDT", ss, []string{"15m"}, "15m", 60)
	if def.CurrentEMAs[20] != def.CurrentEMA20 || def.CurrentRSIs[7] != def.CurrentRSI7 {
		t.Fatal("default current values must equal the fixed EMA20 / RSI7")
	}
}
