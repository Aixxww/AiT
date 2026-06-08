package local

import "testing"

func TestLeaderMomentumTimingUpgradesHealthyContinuation(t *testing.T) {
	mod := &leaderMomentumLongModule{}
	ctx := &V7SymbolContext{
		Symbol:       "LEADERUSDT",
		CurrentPrice: 1.0,
		Change1h:     2.2,
		Change4h:     10.5,
		Change24h:    24.0,
		ATR15m:       0.008,
		ATR1h:        0.012,
		ATR4h:        0.04,
		Low1h:        0.982,
		TakerBuy15m:  0.57,
		Snapshot: &SymbolSnapshotData{
			OIDelta1h: 4.2,
			OIDelta4h: 14.5,
		},
	}

	sig := mod.Score(ctx, V7RegimeRotation)
	if sig == nil {
		t.Fatal("expected leader momentum signal")
	}
	if sig.TimingScore < 60 {
		t.Fatalf("timing = %.1f, want >= 60", sig.TimingScore)
	}

	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())
	if sig.ExecutionQuality != V7ExecReady {
		t.Fatalf("execution quality = %s, want ready (timing %.1f tags %+v)", sig.ExecutionQuality, sig.TimingScore, sig.RiskTags)
	}
}

func TestLeaderMomentumTimingCapsLateWeakChase(t *testing.T) {
	mod := &leaderMomentumLongModule{}
	ctx := &V7SymbolContext{
		Symbol:       "CHASEUSDT",
		CurrentPrice: 1.0,
		Change1h:     7.2,
		Change4h:     18.0,
		Change24h:    50.0,
		ATR15m:       0.01,
		ATR1h:        0.012,
		ATR4h:        0.04,
		Low1h:        0.982,
		TakerBuy15m:  0.54,
		Snapshot: &SymbolSnapshotData{
			OIDelta1h: 9.0,
			OIDelta4h: 70.0,
		},
	}

	sig := mod.Score(ctx, V7RegimeRotation)
	if sig == nil {
		t.Fatal("expected leader momentum signal")
	}
	if sig.TimingScore >= 60 {
		t.Fatalf("timing = %.1f, want < 60 for late weak chase", sig.TimingScore)
	}

	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())
	if sig.ExecutionQuality != V7ExecChaseRisk {
		t.Fatalf("execution quality = %s, want chase_risk (timing %.1f tags %+v)", sig.ExecutionQuality, sig.TimingScore, sig.RiskTags)
	}
}

func TestLeaderMomentumExtremeFundingStaysWatch(t *testing.T) {
	mod := &leaderMomentumLongModule{}
	ctx := &V7SymbolContext{
		Symbol:       "CROWDEDUSDT",
		CurrentPrice: 1.0,
		Change1h:     2.0,
		Change4h:     11.0,
		Change24h:    25.0,
		ATR15m:       0.008,
		ATR1h:        0.012,
		ATR4h:        0.04,
		Low1h:        0.982,
		TakerBuy15m:  0.62,
		RSI1h:        65,
		Snapshot: &SymbolSnapshotData{
			FundingRate: 0.00124,
			OIDelta1h:   4.2,
			OIDelta4h:   14.5,
		},
	}

	sig := mod.Score(ctx, V7RegimeRotation)
	if sig == nil {
		t.Fatal("expected leader momentum signal")
	}
	sig.RiskScore = AssessV7Risk(sig, ctx)
	sig.RiskLevel = ClassifyV7RiskLevel(sig.RiskScore)

	finalizeV7SignalForExecution(sig, ctx, DefaultV7Config())
	if sig.ExecutionQuality != V7ExecChaseRisk {
		t.Fatalf("execution quality = %s, want chase_risk (risk %.1f tags %+v reasons %+v)", sig.ExecutionQuality, sig.RiskScore, sig.RiskTags, sig.ReasonCodes)
	}
	if !hasLeaderMomentumReason(sig, "momentum_extreme_funding_wait") {
		t.Fatalf("missing funding wait reason: %+v", sig.ReasonCodes)
	}
}
