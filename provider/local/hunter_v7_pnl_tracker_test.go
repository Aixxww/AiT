package local

import (
	"testing"
	"time"
)

func TestSignalOutcomeTrackerEmitsTP0Outcome(t *testing.T) {
	price := 100.0
	candle := &TrackedCandle{T: time.Now(), High: 100.5, Low: 99.8, Close: 100.5}
	var outcomes []TrackedOutcome
	tracker := NewSignalOutcomeTracker(&TrackerConfig{
		PollInterval:      time.Hour,
		TimeoutDuration:   time.Hour,
		MaxTracked:        10,
		SnapshotLimit:     10,
		EnableDynamicStop: false,
	}, func(string) float64 {
		return price
	})
	tracker.SetCandleSource(func(string) *TrackedCandle {
		return candle
	})
	tracker.SetOutcomeCallback(func(outcome TrackedOutcome) {
		outcomes = append(outcomes, outcome)
	})

	tracker.Register(1, "TESTUSDT", string(V7DirLong), string(V7SetupLeaderMomentumLong), "EXECUTABLE", 100, 95, 101, 103, 106, time.Now())
	candle = &TrackedCandle{T: time.Now(), High: 101.2, Low: 100.2, Close: 100.6}
	tracker.TickNow()

	if len(outcomes) != 1 {
		t.Fatalf("outcomes len = %d, want 1", len(outcomes))
	}
	if outcomes[0].Status != TrackedWinTP0 {
		t.Fatalf("status = %s, want %s", outcomes[0].Status, TrackedWinTP0)
	}
	if outcomes[0].PnLPct <= 0 || outcomes[0].MaxFavorable <= 0 {
		t.Fatalf("expected positive pnl/mfe, got pnl=%f mfe=%f", outcomes[0].PnLPct, outcomes[0].MaxFavorable)
	}
	if got := len(tracker.GetActive()); got != 0 {
		t.Fatalf("active len = %d, want 0 after terminal outcome", got)
	}
}

func TestSignalOutcomeTrackerDynamicStopUsesCandleLow(t *testing.T) {
	candle := &TrackedCandle{T: time.Now(), High: 104, Low: 103.5, Close: 103.8}
	var outcomes []TrackedOutcome
	tracker := NewSignalOutcomeTracker(&TrackerConfig{
		PollInterval:      time.Hour,
		TimeoutDuration:   time.Hour,
		MaxTracked:        10,
		SnapshotLimit:     10,
		EnableDynamicStop: true,
		ATRPercentile:     50,
	}, func(string) float64 {
		return candle.Close
	})
	tracker.SetCandleSource(func(string) *TrackedCandle {
		return candle
	})
	tracker.SetOutcomeCallback(func(outcome TrackedOutcome) {
		outcomes = append(outcomes, outcome)
	})

	tracker.Register(3, "TESTUSDT", string(V7DirLong), string(V7SetupPullbackLong), "EXECUTABLE", 100, 95, 110, 115, 120, time.Now().Add(-40*time.Minute))
	tracker.TickNow()
	if len(outcomes) != 1 || outcomes[0].Status != TrackedActive {
		t.Fatalf("first outcome = %+v, want active", outcomes)
	}

	active := tracker.GetActive()
	if len(active) != 1 || active[0].DynamicStop <= 100 {
		t.Fatalf("dynamic stop not raised enough: %+v", active)
	}

	candle = &TrackedCandle{T: time.Now(), High: 103.2, Low: active[0].DynamicStop - 0.01, Close: 103}
	tracker.TickNow()
	if outcomes[len(outcomes)-1].Status != TrackedStop {
		t.Fatalf("last status = %s, want %s", outcomes[len(outcomes)-1].Status, TrackedStop)
	}
}

func TestSignalOutcomeTrackerReplaysCandleHistorySinceSignal(t *testing.T) {
	signalTime := time.Now().Add(-3 * time.Minute).Truncate(time.Second)
	tracker := NewSignalOutcomeTracker(&TrackerConfig{
		PollInterval:      time.Hour,
		TimeoutDuration:   time.Hour,
		MaxTracked:        10,
		EnableDynamicStop: false,
	}, nil)

	tracker.SetCandleHistorySource(func(symbol string, since time.Time) []TrackedCandle {
		return []TrackedCandle{
			{T: signalTime.Add(1 * time.Minute), Open: 100, High: 100.4, Low: 99.8, Close: 100.2},
			{T: signalTime.Add(2 * time.Minute), Open: 100.2, High: 101.2, Low: 100.1, Close: 100.5},
			{T: signalTime.Add(3 * time.Minute), Open: 100.5, High: 100.6, Low: 100.0, Close: 100.1},
		}
	})

	var outcome TrackedOutcome
	tracker.SetOutcomeCallback(func(o TrackedOutcome) {
		outcome = o
	})

	tracker.Register(4, "TESTUSDT", string(V7DirLong), string(V7SetupPullbackLong), "EXECUTABLE", 100, 95, 101, 103, 106, signalTime)
	tracker.TickNow()

	if outcome.Status != TrackedWinTP0 {
		t.Fatalf("status = %s, want %s", outcome.Status, TrackedWinTP0)
	}
	if outcome.ExitPrice != 101 {
		t.Fatalf("exit price = %v, want TP0 price 101", outcome.ExitPrice)
	}
	if outcome.ExitTime == nil || !outcome.ExitTime.Equal(signalTime.Add(2*time.Minute)) {
		t.Fatalf("exit time = %v, want second candle close time", outcome.ExitTime)
	}
	if got := len(tracker.GetActive()); got != 0 {
		t.Fatalf("active len = %d, want 0 after terminal replay", got)
	}
}

func TestSignalOutcomeTrackerBackfillsWhenHistoryMissesSignalWindow(t *testing.T) {
	signalTime := time.Now().Add(-5 * time.Minute).Truncate(time.Second)
	tracker := NewSignalOutcomeTracker(&TrackerConfig{
		PollInterval:      time.Hour,
		TimeoutDuration:   time.Hour,
		MaxTracked:        10,
		EnableDynamicStop: false,
	}, nil)

	tracker.SetCandleHistorySource(func(symbol string, since time.Time) []TrackedCandle {
		return []TrackedCandle{
			{T: signalTime.Add(4 * time.Minute), Open: 100.2, High: 100.3, Low: 100, Close: 100.1},
		}
	})
	backfillCalls := 0
	tracker.SetCandleBackfillSource(func(symbol string, from, to time.Time) []TrackedCandle {
		backfillCalls++
		return []TrackedCandle{
			{T: signalTime.Add(1 * time.Minute), Open: 100, High: 101.4, Low: 99.9, Close: 100.6},
		}
	})

	var outcome TrackedOutcome
	tracker.SetOutcomeCallback(func(o TrackedOutcome) {
		outcome = o
	})

	tracker.Register(5, "TESTUSDT", string(V7DirLong), string(V7SetupPullbackLong), "EXECUTABLE", 100, 95, 101, 103, 106, signalTime)
	tracker.TickNow()

	if backfillCalls != 1 {
		t.Fatalf("backfill calls = %d, want 1", backfillCalls)
	}
	if outcome.Status != TrackedWinTP0 {
		t.Fatalf("status = %s, want %s", outcome.Status, TrackedWinTP0)
	}
	if outcome.ExitTime == nil || !outcome.ExitTime.Equal(signalTime.Add(time.Minute)) {
		t.Fatalf("exit time = %v, want backfilled candle time", outcome.ExitTime)
	}
}

func TestSignalOutcomeTrackerSkipsBackfillWhenHistoryCoversSignalWindow(t *testing.T) {
	signalTime := time.Now().Add(-5 * time.Minute).Truncate(time.Second)
	tracker := NewSignalOutcomeTracker(&TrackerConfig{
		PollInterval:      time.Hour,
		TimeoutDuration:   time.Hour,
		MaxTracked:        10,
		EnableDynamicStop: false,
	}, nil)

	tracker.SetCandleHistorySource(func(symbol string, since time.Time) []TrackedCandle {
		return []TrackedCandle{
			{T: signalTime.Add(30 * time.Second), Open: 100, High: 100.2, Low: 99.8, Close: 100.1},
		}
	})
	backfillCalls := 0
	tracker.SetCandleBackfillSource(func(symbol string, from, to time.Time) []TrackedCandle {
		backfillCalls++
		return nil
	})

	tracker.Register(6, "TESTUSDT", string(V7DirLong), string(V7SetupPullbackLong), "EXECUTABLE", 100, 95, 101, 103, 106, signalTime)
	tracker.TickNow()

	if backfillCalls != 0 {
		t.Fatalf("backfill calls = %d, want 0 when history covers signal window", backfillCalls)
	}
}

func TestSignalOutcomeTrackerThrottlesActiveOutcomeWrites(t *testing.T) {
	signalTime := time.Now().Add(-10 * time.Minute).Truncate(time.Second)
	candleTime := signalTime
	high := 100.4
	tracker := NewSignalOutcomeTracker(&TrackerConfig{
		PollInterval:          time.Hour,
		TimeoutDuration:       time.Hour,
		MaxTracked:            10,
		SnapshotLimit:         10,
		ActiveOutcomeInterval: 10 * time.Minute,
		EnableDynamicStop:     false,
	}, nil)
	tracker.SetCandleSource(func(string) *TrackedCandle {
		candleTime = candleTime.Add(time.Minute)
		return &TrackedCandle{T: candleTime, Open: 100, High: high, Low: 99.8, Close: 100.2}
	})

	var outcomes []TrackedOutcome
	tracker.SetOutcomeCallback(func(o TrackedOutcome) {
		outcomes = append(outcomes, o)
	})

	tracker.Register(7, "TESTUSDT", string(V7DirLong), string(V7SetupPullbackLong), "EXECUTABLE", 100, 95, 101, 103, 106, signalTime)
	tracker.TickNow()
	tracker.TickNow()

	if len(outcomes) != 1 {
		t.Fatalf("outcomes len = %d, want 1 active write within throttle interval", len(outcomes))
	}
	if outcomes[0].Status != TrackedActive {
		t.Fatalf("status = %s, want %s", outcomes[0].Status, TrackedActive)
	}

	high = 101.2
	tracker.TickNow()
	if len(outcomes) != 2 {
		t.Fatalf("outcomes len = %d, want terminal write despite throttle", len(outcomes))
	}
	if outcomes[1].Status != TrackedWinTP0 {
		t.Fatalf("terminal status = %s, want %s", outcomes[1].Status, TrackedWinTP0)
	}
}

func TestSignalOutcomeTrackerTimeoutOutcome(t *testing.T) {
	var outcome TrackedOutcome
	tracker := NewSignalOutcomeTracker(&TrackerConfig{
		PollInterval:    time.Hour,
		TimeoutDuration: time.Minute,
		MaxTracked:      10,
		SnapshotLimit:   10,
	}, func(string) float64 {
		return 100
	})
	tracker.SetOutcomeCallback(func(o TrackedOutcome) {
		outcome = o
	})

	tracker.Register(2, "TESTUSDT", string(V7DirLong), string(V7SetupPullbackLong), "REVIEWABLE", 100, 95, 105, 110, 115, time.Now().Add(-2*time.Minute))
	tracker.TickNow()

	if outcome.Status != TrackedTimeout {
		t.Fatalf("status = %s, want %s", outcome.Status, TrackedTimeout)
	}
}
