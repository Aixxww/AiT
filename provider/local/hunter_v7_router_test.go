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
