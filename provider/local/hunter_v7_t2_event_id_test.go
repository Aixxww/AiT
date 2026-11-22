package local

import (
	"testing"

	"github.com/Aixxww/AiT/datafetch"
)

// Fix 11 (P2-11): every T2 output from RunV7T2Module with an event store
// carries the id of its lifecycle event, so stored rows can be grouped by
// event (the N3 oscillation re-test).

func TestT2OutputCarriesEventID(t *testing.T) {
	ss := t2AdapterMomentumSnapshot(true)
	snap := ta1Snapshot(ss)
	cfg := &V7T2Config{Enabled: true}
	ts := BuildV7T2SnapshotFromDatafetch(ss)
	sig := EvaluateV7T2Snapshot(ts, cfg, nil)
	if sig == nil {
		t.Fatal("fixture must fire")
	}
	st := NewV7T2EventStore()
	out := RunV7T2Module(snap, cfg, st, nil, "", u1T0)
	want := V7T2EventID(sig, ts.Price())
	if len(out) != 1 || out[0].T2EventID != want || st.Get(want) == nil {
		t.Fatalf("event id %q, want %q (stored: %v)", out[0].T2EventID, want, st.Get(want) != nil)
	}
	// Without an event store there is no event: empty.
	if out := RunV7T2Module(snap, cfg, nil, nil, "", u1T0); len(out) != 1 || out[0].T2EventID != "" {
		t.Fatalf("no store: %+v", out)
	}
	// The setup prefix (U11) renames the setup type, not the event id.
	cfg.SetupPrefix = true
	if out := RunV7T2Module(snap, cfg, NewV7T2EventStore(), nil, "", u1T0); out[0].T2EventID != want {
		t.Fatalf("prefixed output event id %q, want %q", out[0].T2EventID, want)
	}
}

// A FreshlyTriggered re-emission (the detector is silent on that scan)
// carries the event's id too.
func TestT2ReemissionCarriesEventID(t *testing.T) {
	st, ev := u1Store(t, u1Sig("ignition_early_long", "LONG", 70))
	present := ta1Snapshot(&datafetch.SymbolSnapshot{Symbol: "AUSDT", Price: 100})
	out := RunV7T2Module(present, &V7T2Config{Enabled: true}, st, nil, "", at(3))
	if len(out) != 1 || out[0].LifecycleState != "TRIGGERED" || out[0].T2EventID != ev.EventID || ev.EventID == "" {
		t.Fatalf("re-emission %+v, want event id %q", out, ev.EventID)
	}
}

// What the re-test relies on: a TRIGGERED event never returns to WATCH
// (re-hits keep it TRIGGERED), and a WATCH row after it for the same
// symbol and direction is a different event, created no sooner than the
// 60-minute cooldown after the previous event's creation. With the event id
// stored, each TRIGGERED -> WATCH change can be checked against that.
func TestT2TriggeredThenWatchIsANewEvent(t *testing.T) {
	st, a := u1Store(t, u1Sig("ignition_early_long", "LONG", 70)) // event A at +0
	st.UpdateOutcomes(map[string]float64{"AUSDT": 100}, at(3))
	if a.Status != "TRIGGERED" {
		t.Fatalf("A status %s", a.Status)
	}
	// Re-hit of A: stays TRIGGERED, same id.
	if res := st.Upsert(u1Sig("ignition_early_long", "LONG", 71), 100, at(10)); res.Event != a || a.Status != "TRIGGERED" {
		t.Fatalf("re-hit: %+v status %s", res, a.Status)
	}
	// A different zone (new event id) for the same symbol and direction
	// inside the cooldown is dropped.
	b := u1Sig("ignition_early_long", "LONG", 70)
	b.Trigger, b.Entry = 102, 102
	if res := st.Upsert(b, 100, at(59)); !res.Dropped || res.DropReason != "cooldown" {
		t.Fatalf("inside cooldown: %+v", res)
	}
	res := st.Upsert(b, 100, at(60))
	if !res.Created || res.Event.Status != "WATCH" || res.Event.EventID == a.EventID {
		t.Fatalf("after cooldown: %+v", res)
	}
	if gap := res.Event.CreatedAt.Sub(a.CreatedAt); gap.Minutes() < 60 {
		t.Fatalf("new event %v after the previous one, want >= 60 min", gap)
	}
	if a.Status != "TRIGGERED" {
		t.Fatalf("A changed to %s", a.Status)
	}
}
