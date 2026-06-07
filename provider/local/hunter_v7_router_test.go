package local

import (
	"strings"
	"testing"
)

func TestDefaultV7ConfirmationsDoNotTreatEntryZoneAsReclaim(t *testing.T) {
	tests := []struct {
		name      string
		direction V7Direction
		entryMode V7EntryMode
		want      string
	}{
		{
			name:      "long wait reclaim",
			direction: V7DirLong,
			entryMode: V7EntryWaitReclaim,
			want:      "15m_close_above_vwap_or_ema20_or_entry_zone_upper",
		},
		{
			name:      "short wait reclaim",
			direction: V7DirShort,
			entryMode: V7EntryWaitReclaim,
			want:      "15m_close_below_vwap_or_ema20_or_entry_zone_lower",
		},
		{
			name:      "long wait price reversal",
			direction: V7DirLong,
			entryMode: V7EntryWaitPriceReversal,
			want:      "15m_close_above_vwap_or_ema20_or_entry_zone_upper",
		},
		{
			name:      "short wait price reversal",
			direction: V7DirShort,
			entryMode: V7EntryWaitPriceReversal,
			want:      "15m_close_below_vwap_or_ema20_or_entry_zone_lower",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confirms := defaultV7Confirmations(&V7SignalOutput{
				Direction: tt.direction,
				EntryMode: tt.entryMode,
			})
			if len(confirms) == 0 || confirms[0] != tt.want {
				t.Fatalf("first confirmation = %q, want %q", confirms, tt.want)
			}
			for _, confirm := range confirms {
				if strings.Contains(confirm, "reclaim_vwap_or_entry_zone") {
					t.Fatalf("ambiguous confirmation remains: %q", confirm)
				}
			}
		})
	}
}

func TestFundingReversalDoesNotChaseShortAfterFastDropWithBuildingOI(t *testing.T) {
	mod := &fundingReversalModule{}
	ctx := &V7SymbolContext{
		Symbol:       "HUSDT",
		CurrentPrice: 0.63364,
		Change1h:     -5.77,
		Change4h:     7.28,
		Change24h:    -18.09,
		ATR15m:       0.0253,
		TakerBuy15m:  0.396,
		VWAP15m:      0.63576,
		High4h:       0.75672,
		ATR4h:        0.06255,
		Snapshot: &SymbolSnapshotData{
			FundingRate: 0.00005,
			LSR:         2.42,
			OIDelta1h:   9.86,
			OIDelta4h:   13.23,
		},
	}

	if sig := mod.Score(ctx, V7RegimeTrendDown); sig != nil {
		t.Fatalf("expected late short with building OI to be filtered before scoring, got %+v", sig)
	}
}

func TestRouterUsesGlobalMinPriorityForCandidateVisibility(t *testing.T) {
	signals := []V7SignalOutput{
		{
			Symbol:     "BREAKUSDT",
			Direction:  V7DirLong,
			SetupType:  V7SetupTrendBreakoutLong,
			Status:     V7StatusCandidate,
			AIPriority: 52,
		},
	}

	got := filterV7SignalsForLLM(signals, V7Config{
		MaxOutput:             10,
		MinOutput:             3,
		MinAIPriority:         50,
		FallbackMinAIPriority: 45,
		SetupThresholds:       DefaultSetupThresholds(),
	})

	if len(got) != 1 {
		t.Fatalf("signals = %d, want 1", len(got))
	}
	if got[0].Symbol != "BREAKUSDT" {
		t.Fatalf("symbol = %s, want BREAKUSDT", got[0].Symbol)
	}
}

func TestRouterBackfillsContextCandidatesWhenPrimaryPoolIsThin(t *testing.T) {
	signals := []V7SignalOutput{
		{Symbol: "AUSDT", Direction: V7DirLong, SetupType: V7SetupPanicReversalLong, Status: V7StatusCandidate, AIPriority: 57},
		{Symbol: "BUSDT", Direction: V7DirShort, SetupType: V7SetupDistributionShort, Status: V7StatusCandidate, AIPriority: 49},
		{Symbol: "CUSDT", Direction: V7DirLong, SetupType: V7SetupPullbackLong, Status: V7StatusCandidate, AIPriority: 46},
		{Symbol: "DUSDT", Direction: V7DirLong, SetupType: V7SetupLeaderMomentumLong, Status: V7StatusCandidate, AIPriority: 43},
		{Symbol: "EUSDT", Direction: V7DirShort, SetupType: V7SetupFundingReversal, Status: V7StatusFiltered, AIPriority: 80},
	}

	got := filterV7SignalsForLLM(signals, V7Config{
		MaxOutput:             10,
		MinOutput:             3,
		MinAIPriority:         50,
		FallbackMinAIPriority: 45,
	})

	if len(got) != 3 {
		t.Fatalf("signals = %d, want 3", len(got))
	}
	wantSymbols := []string{"AUSDT", "BUSDT", "CUSDT"}
	for i, want := range wantSymbols {
		if got[i].Symbol != want {
			t.Fatalf("signal[%d] = %s, want %s", i, got[i].Symbol, want)
		}
	}
	for _, sig := range got {
		if sig.Symbol == "EUSDT" {
			t.Fatal("hard-filtered signal was backfilled")
		}
		if sig.AIPriority < 50 && !containsString(sig.RiskTags, "context_only_low_priority") {
			t.Fatalf("backfill signal %s missing context risk tag: %+v", sig.Symbol, sig.RiskTags)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
