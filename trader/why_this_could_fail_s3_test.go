package trader

import (
	"testing"

	"github.com/Aixxww/AiT/kernel"
	"github.com/Aixxww/AiT/store"
)

// #111 S3: the decision record's why_this_could_fail column is the first
// open's non-empty value (a demoted open counts); nil otherwise.
func TestS3FirstOpenWhyThisCouldFail(t *testing.T) {
	ds := []kernel.Decision{
		{Symbol: "ALL", Action: "wait"},
		{Symbol: "AUSDT", Action: "open_long"},
		{Symbol: "BUSDT", Action: "wait", RejectedAction: "open_short", WhyThisCouldFail: "demoted short"},
		{Symbol: "CUSDT", Action: "open_long", WhyThisCouldFail: "later open"},
	}
	if got := firstOpenWhyThisCouldFail(ds); got == nil || *got != "demoted short" {
		t.Fatalf("got %v", got)
	}
	if got := firstOpenWhyThisCouldFail(ds[:2]); got != nil {
		t.Fatalf("no value: got %q", *got)
	}
	rec := &store.DecisionRecord{}
	applyAIDecisionToRecord(rec, &kernel.FullDecision{Decisions: ds[3:]})
	if rec.WhyThisCouldFail == nil || *rec.WhyThisCouldFail != "later open" {
		t.Fatalf("record: %v", rec.WhyThisCouldFail)
	}
}
