package local

import (
	"testing"
)

// mmsTrendRideTestContext builds a context that clears every mmsTrendRideLongModule
// Match precondition, so a test only varies the continuation inputs it cares about.
func mmsTrendRideTestContext(change1h, change4h, oi1h, oi4h float64) *V7SymbolContext {
	return &V7SymbolContext{
		Symbol:         "TESTUSDT",
		CurrentPrice:   100,
		Change1h:       change1h,
		Change4h:       change4h,
		EMA7_15m:       101,
		EMA25_15m:      100,
		EMA99_15m:      98,
		Last15mLow:     100.2,
		Last15mClose:   100.5,
		VolumeBurst15m: 0.5,
		RSI1h:          60,
		TakerBuy15m:    0.55,
		ATR15m:         1,
		Snapshot: &SymbolSnapshotData{
			QuoteVolume24h: 30_000_000,
			OI:             5_000_000,
			TradeCount24h:  50_000,
			OIDelta1h:      oi1h,
			OIDelta4h:      oi4h,
		},
	}
}

// The 2026-07-26 live rounds showed a single-frame OI or price dip is normal
// noise inside a live trend. Demoting on either frame alone also killed
// SIRENUSDT, the only TP0 winner in the tracked MMS sample, so both frames must
// agree before the signal is marked review-only.
func TestMMSTrendRideWeakContinuationTagUsesBothFrames(t *testing.T) {
	cases := []struct {
		name                    string
		change1h, change4h      float64
		oi1h, oi4h              float64
		wantWeakContinuationTag bool
	}{
		// Losers from the tracked sample: both price frames dead.
		{"wif_both_price_frames_negative", -1.20, -0.45, 0.42, 3.98, true},
		// Loser: OI leaving on both frames.
		{"etc_oi_negative_on_both_frames", 0.27, 0.27, -0.36, -0.66, true},
		// Winner: 1h dipped but 4h ran and 1h OI is positive.
		{"siren_winner_must_stay_open", -0.25, 1.00, 0.40, -0.29, false},
		// Live 17:24 round: healthy trend with a trivial 1h OI wiggle.
		{"la_minor_1h_oi_wiggle", 0.97, 0.54, -0.32, 0.74, false},
		{"beat_strong_4h_oi_inflow", 0.39, 0.11, -0.72, 3.15, false},
		{"chillguy_1h_dip_with_oi_inflow", -1.35, 1.95, 4.87, 1.24, false},
	}

	module := &mmsTrendRideLongModule{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := mmsTrendRideTestContext(tc.change1h, tc.change4h, tc.oi1h, tc.oi4h)
			sig := module.Score(ctx, V7RegimeTrendUp)
			if sig == nil {
				t.Fatal("expected mms_trend_ride_long signal from fixture")
			}
			got := containsTagValue(sig.RiskTags, "mms_weak_continuation_review_only")
			if got != tc.wantWeakContinuationTag {
				t.Fatalf("mms_weak_continuation_review_only = %v, want %v (tags=%v)",
					got, tc.wantWeakContinuationTag, sig.RiskTags)
			}
		})
	}
}

func containsTagValue(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
