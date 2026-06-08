package trader

import (
	"testing"

	"github.com/Aixxww/AiT/kernel"
)

func TestShouldSkipCandidateForRepeatedWaitKeepsOnlyReadyExecutableHunterV7Signals(t *testing.T) {
	tests := []struct {
		name string
		coin kernel.CandidateCoin
		want bool
	}{
		{
			name: "legacy repeated wait still skips",
			coin: kernel.CandidateCoin{Symbol: "OLDUSDT"},
			want: true,
		},
		{
			name: "low priority watch skips",
			coin: kernel.CandidateCoin{
				Symbol:             "WATCHUSDT",
				V7SetupType:        "pre_breakout_watch",
				V7Status:           "wait_confirm",
				V7ExecutionQuality: "watch_only",
				V7AIPriority:       42,
			},
			want: true,
		},
		{
			name: "low priority near confirm now skips",
			coin: kernel.CandidateCoin{
				Symbol:             "READYUSDT",
				V7SetupType:        "trend_breakout_long",
				V7Status:           "candidate",
				V7ExecutionQuality: "near_confirm",
				V7AIPriority:       47,
			},
			want: true,
		},
		{
			name: "high priority wait now skips",
			coin: kernel.CandidateCoin{
				Symbol:             "STRONGUSDT",
				V7SetupType:        "distribution_short",
				V7Status:           "wait_confirm",
				V7ExecutionQuality: "watch_only",
				V7AIPriority:       64,
			},
			want: true,
		},
		{
			name: "ready executable is kept",
			coin: kernel.CandidateCoin{
				Symbol:             "PRIMEUSDT",
				Direction:          "LONG",
				V7SetupType:        "panic_reversal_long",
				V7Status:           "candidate",
				V7ExecutionQuality: "ready",
				V7ExecutionTier:    "EXECUTABLE",
				V7AIPriority:       62,
			},
			want: false,
		},
		{
			name: "reviewable with timing is kept",
			coin: kernel.CandidateCoin{
				Symbol:             "FUNDUSDT",
				Direction:          "SHORT",
				V7SetupType:        "funding_reversal",
				V7Status:           "wait_confirm",
				V7ExecutionQuality: "watch_only",
				V7ExecutionTier:    "REVIEWABLE",
				V7AIPriority:       59,
				V7TimingScore:      62,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSkipCandidateForRepeatedWait(tt.coin, 3)
			if got != tt.want {
				t.Fatalf("skip = %v, want %v", got, tt.want)
			}
		})
	}
}
