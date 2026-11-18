package local

import (
	"testing"
	"time"
)

// U1/P1: spec section 5.3 trigger stage and outcome tracking.

var u1T0 = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

func u1Sig(setup, dir string, score float64) *V7T2Signal {
	s := &V7T2Signal{Symbol: "AUSDT", Direction: dir, SetupType: setup, Status: "WATCH", Score: score, Trigger: 100}
	if dir == "LONG" {
		s.Entry, s.Stop, s.Target1, s.Target2 = 100, 99, 101.8, 103
	} else {
		s.Entry, s.Stop, s.Target1, s.Target2 = 100, 101, 98.2, 97
	}
	return s
}

func u1Store(t *testing.T, sig *V7T2Signal) (*V7T2EventStore, *V7T2Event) {
	t.Helper()
	st := NewV7T2EventStore()
	res := st.Upsert(sig, 100, u1T0)
	if !res.Created {
		t.Fatalf("event not created: %+v", res)
	}
	return st, res.Event
}

func at(m int) time.Time { return u1T0.Add(time.Duration(m) * time.Minute) }

func TestU1WatchEventTriggersOnEntry(t *testing.T) {
	st, ev := u1Store(t, u1Sig("ignition_early_long", "LONG", 70)) // WATCH by construction
	if ev.Status != "WATCH" {
		t.Fatalf("status %s", ev.Status)
	}
	if tr := st.UpdateOutcomes(map[string]float64{"AUSDT": 99.9}, at(3)); len(tr) != 0 || ev.Status != "WATCH" {
		t.Fatalf("below entry must not trigger: %v %s", tr, ev.Status)
	}
	tr := st.UpdateOutcomes(map[string]float64{"AUSDT": 100.0}, at(6))
	if len(tr) != 1 || tr[0].To != "TRIGGERED" || ev.Status != "TRIGGERED" || ev.TriggeredAt == nil || !ev.TriggeredAt.Equal(at(6)) {
		t.Fatalf("WATCH ignition must trigger at entry: %+v status=%s", tr, ev.Status)
	}
}

func TestU1ShortTriggersAtOrBelowEntry(t *testing.T) {
	st, ev := u1Store(t, u1Sig("ignition_early_short", "SHORT", 70))
	st.UpdateOutcomes(map[string]float64{"AUSDT": 99.95}, at(3))
	if ev.Status != "TRIGGERED" {
		t.Fatalf("short must trigger at price <= entry, got %s", ev.Status)
	}
}

func TestU1PreEntryStopInvalidates(t *testing.T) {
	sig := u1Sig("ignition_early_short", "SHORT", 70)
	sig.Entry = 99 // entry below the scan price: not yet reached
	st, ev := u1Store(t, sig)
	st.UpdateOutcomes(map[string]float64{"AUSDT": 101.2}, at(3))
	if ev.Status != "INVALIDATED" || ev.Outcome != "NO_ENTRY_INVALIDATED" || ev.RealizedR != nil || ev.TriggeredAt != nil {
		t.Fatalf("pre-entry stop: status=%s outcome=%s r=%v", ev.Status, ev.Outcome, ev.RealizedR)
	}
	// Terminal: the same event id is never revived.
	if res := st.Upsert(sig, 100, at(70)); !res.Dropped || res.DropReason != "terminal_event" {
		t.Fatalf("terminal event revived: %+v", res)
	}
}

func TestU1PostEntryOutcomes(t *testing.T) {
	cases := []struct {
		name    string
		path    []float64
		outcome string
		r       float64
	}{
		{"stop", []float64{100, 99.5, 99}, "STOP", -1},
		{"tp1_then_stop", []float64{100, 101.9, 99}, "TP1_THEN_STOP", 0.4},
		{"tp2", []float64{100, 101.9, 103}, "TP2", 2.4},
	}
	for _, c := range cases {
		st, ev := u1Store(t, u1Sig("momentum_continuation_long", "LONG", 90))
		for i, p := range c.path {
			st.UpdateOutcomes(map[string]float64{"AUSDT": p}, at(3*(i+1)))
		}
		if ev.Status != "INVALIDATED" || ev.Outcome != c.outcome || ev.RealizedR == nil || *ev.RealizedR != c.r {
			t.Errorf("%s: status=%s outcome=%s r=%v", c.name, ev.Status, ev.Outcome, ev.RealizedR)
		}
	}
}

func TestU1ExpiryRealizedR(t *testing.T) {
	st, ev := u1Store(t, u1Sig("ignition_early_long", "LONG", 70)) // WATCH, 2h
	st.UpdateOutcomes(map[string]float64{"AUSDT": 100.2}, at(10))
	st.UpdateOutcomes(map[string]float64{"AUSDT": 100.5}, at(120))
	if ev.Status != "EXPIRED" || ev.Outcome != "EXPIRED" || ev.RealizedR == nil || *ev.RealizedR != 0.5 {
		t.Fatalf("triggered expiry: status=%s r=%v", ev.Status, ev.RealizedR)
	}
	st2, ev2 := u1Store(t, u1Sig("ignition_early_long", "LONG", 70))
	st2.UpdateOutcomes(map[string]float64{"AUSDT": 99.5}, at(120))
	if ev2.Status != "EXPIRED" || ev2.RealizedR == nil || *ev2.RealizedR != 0 {
		t.Fatalf("untriggered expiry: status=%s r=%v", ev2.Status, ev2.RealizedR)
	}
}

func TestU1DisableTrigger(t *testing.T) {
	st, ev := u1Store(t, u1Sig("ignition_early_long", "LONG", 70))
	st.Configure(V7T2LifecycleParams{DisableTrigger: true})
	st.UpdateOutcomes(map[string]float64{"AUSDT": 100.5}, at(3))
	if ev.Status != "WATCH" || ev.TriggeredAt != nil {
		t.Fatalf("disabled trigger must keep WATCH, got %s", ev.Status)
	}
}

func TestU1ScoreUpgradeKeepsTriggered(t *testing.T) {
	sig := u1Sig("momentum_continuation_long", "LONG", 70)
	st, ev := u1Store(t, sig)
	st.UpdateOutcomes(map[string]float64{"AUSDT": 100}, at(3))
	up := u1Sig("momentum_continuation_long", "LONG", 90)
	up.Status = "ARMED"
	st.Upsert(up, 100, at(6))
	if ev.Status != "TRIGGERED" || ev.Score != 90 {
		t.Fatalf("upgrade on a triggered event: status=%s score=%v", ev.Status, ev.Score)
	}
}

func TestU1FreshlyTriggeredWindow(t *testing.T) {
	st, ev := u1Store(t, u1Sig("ignition_early_long", "LONG", 70))
	st.UpdateOutcomes(map[string]float64{"AUSDT": 100}, at(3))
	if got := st.FreshlyTriggered(at(18)); len(got) != 1 || got[0] != ev {
		t.Fatalf("within the 15 min default window: %v", got)
	}
	if got := st.FreshlyTriggered(at(19)); len(got) != 0 {
		t.Fatalf("outside the window: %v", got)
	}
	st.Configure(V7T2LifecycleParams{TriggeredSurfaceMinutes: 30})
	if got := st.FreshlyTriggered(at(30)); len(got) != 1 {
		t.Fatalf("configured window: %v", got)
	}
}

func TestU1ConfiguredLifecycleParams(t *testing.T) {
	st := NewV7T2EventStore()
	st.Configure(V7T2LifecycleParams{WatchExpiryHours: 1, CooldownMinutes: 10})
	res := st.Upsert(u1Sig("ignition_early_long", "LONG", 70), 100, u1T0)
	if !res.Event.ExpiresAt.Equal(u1T0.Add(time.Hour)) {
		t.Fatalf("watch expiry %v", res.Event.ExpiresAt)
	}
	other := u1Sig("ignition_early_long", "LONG", 70)
	other.Trigger = 110 // different zone, same symbol and direction
	if r := st.Upsert(other, 100, at(9)); !r.Dropped || r.DropReason != "cooldown" {
		t.Fatalf("within the 10 min cooldown: %+v", r)
	}
	if r := st.Upsert(other, 100, at(11)); !r.Created {
		t.Fatalf("after the 10 min cooldown: %+v", r)
	}
}

func TestU1DefaultsEqualScannerConstants(t *testing.T) {
	p := V7T2LifecycleParams{}
	if p.alertScore() != 82 || p.watchScore() != 72 || p.armedExpiry() != 4*time.Hour || p.watchExpiry() != 2*time.Hour ||
		p.cooldown() != time.Hour || p.triggeredSurface() != 15*time.Minute {
		t.Fatalf("defaults changed")
	}
	for setup := range v7T2ObserveOnlySetups {
		if !p.observeOnly(setup) {
			t.Fatalf("%s must stay observe-only by default (OD-5)", setup)
		}
	}
	o := V7T2LifecycleParams{ObserveOnlyOverride: true}
	if o.observeOnly("trend_pullback_long") {
		t.Fatalf("an explicitly empty override list must clear the list")
	}
}

func TestU1TriggeredOutputIsCandidate(t *testing.T) {
	sig := u1Sig("ignition_early_long", "LONG", 70)
	out := sig.ToV7SignalOutputWithLifecycle("", "TRIGGERED")
	if out.Status != V7StatusCandidate || out.LifecycleState != "TRIGGERED" || !v7HasTag(out.RiskTags, "t2_triggered") {
		t.Fatalf("triggered output: %+v", out)
	}
	if w := sig.ToV7SignalOutputWithLifecycle("", "WATCH"); w.Status != V7StatusWaitConfirm || w.LifecycleState != "WATCH" {
		t.Fatalf("watch output: %+v", w)
	}
	obs := u1Sig("trend_pullback_long", "LONG", 94)
	obs.ObserveOnly = true
	if o := obs.ToV7SignalOutputWithLifecycle("", "TRIGGERED"); o.Status != V7StatusWaitConfirm {
		t.Fatalf("observe-only must stay wait_confirm when triggered (OD-5): %s", o.Status)
	}
}

func v7HasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}
