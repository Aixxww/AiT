package local

import "testing"

// The 2026-07-26 live rounds tagged 97.2% of signals stale purely because the
// detail phase ran 52-93s against a fixed 45s cutoff. Freshness must be judged
// against what the pipeline can actually deliver.
func TestV7StaleThresholdTracksFetchCadence(t *testing.T) {
	if got := v7StaleThresholdMs(0); got != 45_000 {
		t.Fatalf("unknown fetch duration threshold = %d, want 45000", got)
	}
	if got := v7StaleThresholdMs(20_000); got != 45_000 {
		t.Fatalf("fast fetch threshold = %d, want 45000 floor", got)
	}
	if got := v7StaleThresholdMs(93_000); got != 108_000 {
		t.Fatalf("slow fetch threshold = %d, want 108000", got)
	}
	if got := v7StaleThresholdMs(600_000); got != 180_000 {
		t.Fatalf("pathological fetch threshold = %d, want 180000 ceiling", got)
	}
}

func TestV7SignalDataIsStale(t *testing.T) {
	cases := []struct {
		name  string
		fresh V7DataFreshness
		want  bool
	}{
		{
			// r01 2026-07-26: 75s fetch, 53.7s price age — previously stale.
			name:  "within_own_fetch_cadence_is_fresh",
			fresh: V7DataFreshness{PriceAgeMs: 53_700, SnapshotFetchMs: 75_000, StaleThresholdMs: 90_000},
			want:  false,
		},
		{
			// verify round: 93s fetch, 91.7s price age.
			name:  "slow_batch_still_fresh_after_rescale",
			fresh: V7DataFreshness{PriceAgeMs: 91_700, SnapshotFetchMs: 93_000, StaleThresholdMs: 108_000},
			want:  false,
		},
		{
			name:  "price_age_beyond_threshold_is_stale",
			fresh: V7DataFreshness{PriceAgeMs: 150_000, SnapshotFetchMs: 75_000, StaleThresholdMs: 90_000},
			want:  true,
		},
		{
			// Real market evidence overrides cadence: the feed stopped.
			name:  "closed_1m_candle_older_than_two_minutes_is_stale",
			fresh: V7DataFreshness{Kline1mAgeMs: 240_000, PriceAgeMs: 1_000, StaleThresholdMs: 108_000},
			want:  true,
		},
		{
			name:  "in_progress_candle_reports_zero_age",
			fresh: V7DataFreshness{Kline1mAgeMs: 0, PriceAgeMs: 30_000, StaleThresholdMs: 90_000},
			want:  false,
		},
		{
			name:  "missing_threshold_falls_back_to_45s",
			fresh: V7DataFreshness{PriceAgeMs: 60_000},
			want:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := v7SignalDataIsStale(tc.fresh); got != tc.want {
				t.Fatalf("v7SignalDataIsStale = %v, want %v", got, tc.want)
			}
		})
	}
}
