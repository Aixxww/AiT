package local

import "testing"

func TestComputeBBWidthPercentileTreatsTightCurrentBandAsLowPercentile(t *testing.T) {
	var bars []klineBar
	for i := 0; i < 40; i++ {
		close := 100.0
		if i%2 == 0 {
			close = 92
		} else {
			close = 108
		}
		bars = append(bars, klineBar{Open: close, High: close + 2, Low: close - 2, Close: close, Volume: 1000})
	}
	for i := 0; i < 25; i++ {
		close := 100.0 + float64(i%3-1)*0.15
		bars = append(bars, klineBar{Open: close, High: close + 0.2, Low: close - 0.2, Close: close, Volume: 1000})
	}

	got := computeBBWidthPercentile(bars)
	if got >= 25 {
		t.Fatalf("tight current BB width percentile = %.2f, want < 25", got)
	}
}
