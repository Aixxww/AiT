package trader

// Unit tests for P0-2: adaptive hard-loss threshold.
import (
	"testing"
	"time"
)

func TestAdaptiveHardLossDefersToWiderPlan(t *testing.T) {
	old := protectionNow
	protectionNow = time.Now
	defer func() { protectionNow = old }()

	newState := func(planROE float64) *positionProtectionState {
		return &positionProtectionState{
			InitialQuantity:       1,
			PlannedStopLossROEPct: planROE,
			OpenedAt:              time.Now().Add(-time.Hour), // avoid early-loss rule
		}
	}

	// Plan at -20% ROE (2% price move @10x): fixed -12% must NOT preempt it.
	action, _ := choosePositionProtectionAction(newState(-20), -13, -1.3)
	if action == protectionHardLossClose {
		t.Errorf("hard loss fired at -13%% despite wider plan (-20%%): plan preempted (P0-2 regression)")
	}

	// Beyond plan + slack (-20*1.1 = -22): backstop must fire.
	action, _ = choosePositionProtectionAction(newState(-20), -23, -2.3)
	if action != protectionHardLossClose {
		t.Errorf("hard loss did NOT fire at -23%% with plan -20%% (slack -22%%), got %s", action)
	}

	// No plan recorded: old fixed behavior preserved.
	action, _ = choosePositionProtectionAction(newState(0), -13, -1.3)
	if action != protectionHardLossClose {
		t.Errorf("without plan, hard loss should fire at -13%% (fixed -12%%), got %s", action)
	}

	// Tighter plan (-8%): fixed -12% backstop still applies.
	action, _ = choosePositionProtectionAction(newState(-8), -13, -1.3)
	if action != protectionHardLossClose {
		t.Errorf("with tighter plan -8%%, hard loss should still fire at -13%%, got %s", action)
	}
}
