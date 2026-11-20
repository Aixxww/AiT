package local

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

// TA-3: the T2 event store round-trips through MarshalState / LoadState
// and behaves identically afterwards (status, triggered_at, cooldown,
// FreshlyTriggered, outcomes).
func TestT2StoreStateRoundTrip(t *testing.T) {
	st := NewV7T2EventStore()
	watch := u1Sig("ignition_early_long", "LONG", 70)
	st.Upsert(watch, 100, u1T0)
	short := u1Sig("ignition_early_short", "SHORT", 70)
	short.Symbol = "BUSDT"
	st.Upsert(short, 100, u1T0)
	st.UpdateOutcomes(map[string]float64{"AUSDT": 100}, at(3)) // AUSDT -> TRIGGERED

	b, err := st.MarshalState()
	if err != nil {
		t.Fatal(err)
	}
	loaded := NewV7T2EventStore()
	if err := loaded.LoadState(b); err != nil {
		t.Fatal(err)
	}
	if loaded.Len() != 2 {
		t.Fatalf("events %d, want 2", loaded.Len())
	}
	id := V7T2EventID(watch, 100)
	ev := loaded.Get(id)
	if ev == nil || ev.Status != "TRIGGERED" || ev.TriggeredAt == nil || !ev.TriggeredAt.Equal(at(3)) {
		t.Fatalf("triggered event not restored: %+v", ev)
	}
	// Same behaviour on both stores for the next scans.
	other := u1Sig("ignition_early_long", "LONG", 70)
	other.Trigger = 110 // new zone, same symbol/direction -> cooldown
	for _, s := range []*V7T2EventStore{st, loaded} {
		if r := s.Upsert(other, 100, at(9)); !r.Dropped || r.DropReason != "cooldown" {
			t.Fatalf("cooldown index not restored: %+v", r)
		}
		if got := s.FreshlyTriggered(at(10)); len(got) != 1 || got[0].EventID != id {
			t.Fatalf("FreshlyTriggered %v", got)
		}
	}
	trA := st.UpdateOutcomes(map[string]float64{"AUSDT": 99, "BUSDT": 101.5}, at(12))
	trB := loaded.UpdateOutcomes(map[string]float64{"AUSDT": 99, "BUSDT": 101.5}, at(12))
	key := func(trs []V7T2Transition) []string {
		out := make([]string, 0, len(trs))
		for _, tr := range trs {
			out = append(out, tr.Event.EventID+":"+tr.From+">"+tr.To+":"+tr.Outcome)
		}
		sort.Strings(out)
		return out
	}
	if ka, kb := key(trA), key(trB); !reflect.DeepEqual(ka, kb) || len(ka) != 2 {
		t.Fatalf("transitions differ or missing: %v vs %v", ka, kb)
	}
	a, _ := st.MarshalState()
	c, _ := loaded.MarshalState()
	if string(a) != string(c) {
		t.Fatal("stores diverged after identical scans")
	}
}

func TestT2StoreCloneIsIsolated(t *testing.T) {
	st, ev := u1Store(t, u1Sig("ignition_early_long", "LONG", 70))
	st.Configure(V7T2LifecycleParams{TriggeredSurfaceMinutes: 30})
	c := st.Clone()
	c.UpdateOutcomes(map[string]float64{"AUSDT": 100}, at(3))
	if ev.Status != "WATCH" {
		t.Fatalf("clone mutated the original: %s", ev.Status)
	}
	if got := c.FreshlyTriggered(at(25)); len(got) != 1 {
		t.Fatalf("clone lost params (30 min surface): %v", got)
	}
	st.ReplaceFrom(c)
	if got := st.Get(ev.EventID); got == nil || got.Status != "TRIGGERED" {
		t.Fatalf("ReplaceFrom did not commit: %+v", got)
	}
	var nilStore *V7T2EventStore
	if nilStore.Clone().Len() != 0 {
		t.Fatal("nil clone must be empty")
	}
}

func TestT2StoreLoadStateRejectsBadInput(t *testing.T) {
	for _, b := range []string{`{`, `{"version":2}`, `{"version":1,"events":[{"EventID":""}]}`,
		`{"version":1,"events":[],"latest_by_sym_dir":{"A|LONG":"missing"}}`} {
		if err := NewV7T2EventStore().LoadState([]byte(b)); err == nil {
			t.Fatalf("accepted %s", b)
		}
	}
}

// Native watch state: export -> import gives a manager that processes the
// next cycle exactly like the original.
func TestWatchStateExportImport(t *testing.T) {
	sig := V7SignalOutput{Symbol: "AUSDT", Direction: V7DirLong, SetupType: V7SetupPreBreakoutWatch, Status: V7StatusWaitConfirm,
		SetupScore: 70, AIPriority: 70, EntryZone: V7PriceZone{Lower: 99, Upper: 101}, Invalidation: V7InvalidationRule{Price: 97},
		Targets: []V7Target{{Price: 105}}, ReasonCodes: []string{"x"}}
	m := NewV7SignalStateManager()
	m.Process([]V7SignalOutput{sig}, 1)
	m.Process([]V7SignalOutput{sig}, 2)
	b, err := m.ExportState()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.entries)+len(m.triggerEntries) == 0 {
		t.Fatal("fixture tracked nothing; the test would be vacuous")
	}
	n := NewV7SignalStateManager()
	if err := n.ImportState(b); err != nil {
		t.Fatal(err)
	}
	b2, _ := n.ExportState()
	if string(b) != string(b2) {
		t.Fatalf("export -> import -> export differs:\n%s\n%s", b, b2)
	}
	gotA := m.Process([]V7SignalOutput{sig}, 3)
	gotB := n.Process([]V7SignalOutput{sig}, 3)
	ja, _ := json.Marshal(gotA)
	jb, _ := json.Marshal(gotB)
	if string(ja) != string(jb) {
		t.Fatalf("cycle 3 differs after import:\n%s\n%s", ja, jb)
	}
	ea, _ := m.ExportState()
	eb, _ := n.ExportState()
	if !reflect.DeepEqual(ea, eb) {
		t.Fatal("states diverged after cycle 3")
	}
	if err := NewV7SignalStateManager().ImportState([]byte(`{"version":9}`)); err == nil {
		t.Fatal("wrong schema accepted")
	}
}
