package datafetch

import "testing"

// #94 / AUD-B6: only [A-Z0-9] symbols are ingested.
func TestIsValidSymbol(t *testing.T) {
	for sym, want := range map[string]bool{
		"BTCUSDT": true, "1000PEPEUSDT": true,
		"EVILUSDT\n# OVERRIDE": false, "BTC USDT": false, "btcusdt": false,
		"BTC-USDT": false, "币安人生USDT": false, "": false,
	} {
		if got := IsValidSymbol(sym); got != want {
			t.Errorf("IsValidSymbol(%q) = %v, want %v", sym, got, want)
		}
	}
}
