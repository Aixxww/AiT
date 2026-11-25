package trader

import (
	"testing"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/store"
)

// #94 / AUD-B8: chosen_rank and the E+R count are recorded on every
// decision record (also a rejected reply) and survive the store round trip;
// the open's own rank is kept on its action.
func TestDecisionRecordCarriesChosenRankAndERCount(t *testing.T) {
	st, err := store.New(t.TempDir() + "/data.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ai := &kernel.FullDecision{HunterV7ERCount: 4, HunterV7ChosenRank: 2,
		Decisions: []kernel.Decision{{Symbol: "BBBUSDT", Action: "open_long", HunterV7ChosenRank: 2}}}
	rec := &store.DecisionRecord{TraderID: "t1", CycleNumber: 1}
	applyAIDecisionToRecord(rec, ai)
	rec.Decisions = []store.DecisionAction{{Action: "open_long", Symbol: "BBBUSDT", ChosenRank: ai.Decisions[0].HunterV7ChosenRank}}
	if err := st.Decision().LogDecision(rec); err != nil {
		t.Fatal(err)
	}

	// A rejected reply (no structured decision) still records the E+R count.
	failed := &store.DecisionRecord{TraderID: "t1", CycleNumber: 2, AIFailureReason: kernel.AIFailureNoStructuredDecision}
	applyAIDecisionToRecord(failed, &kernel.FullDecision{HunterV7ERCount: 3})
	if err := st.Decision().LogDecision(failed); err != nil {
		t.Fatal(err)
	}

	got, err := st.Decision().GetLatestRecords("t1", 5)
	if err != nil || len(got) != 2 {
		t.Fatalf("records: %v %d", err, len(got))
	}
	byCycle := map[int]*store.DecisionRecord{}
	for _, r := range got {
		byCycle[r.CycleNumber] = r
	}
	if r := byCycle[1]; r.ChosenRank != 2 || r.ERCount != 4 || len(r.Decisions) != 1 || r.Decisions[0].ChosenRank != 2 {
		t.Fatalf("open record: rank %d er %d actions %+v", r.ChosenRank, r.ERCount, r.Decisions)
	}
	if r := byCycle[2]; r.ChosenRank != 0 || r.ERCount != 3 || r.AIFailureReason != "no_structured_decision" {
		t.Fatalf("failed record: rank %d er %d reason %q", r.ChosenRank, r.ERCount, r.AIFailureReason)
	}
}
