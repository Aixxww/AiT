package main

// U6 golden replay (plan §6.3 U6 acceptance: identical candidate set and
// tiers before and after). With -golden-out, every snapshot's T2 signal (at
// its created state and at TRIGGERED) is assembled together with the native
// final signals of the same symbol, under each strategy config returned by
// goldenConfigs (golden_geom.go), and the candidate set with tiers and
// stamped risk values is written. Running this file against the pre-U6
// commit and against U6 gives the before/after pair.

import (
	"encoding/json"
	"flag"
	"os"

	"github.com/Aixxww/AiT/kernel"
	local "github.com/Aixxww/AiT/provider/local"
)

var goldenOut = flag.String("golden-out", "", "U6 golden replay output (candidate set and tiers per config)")

type goldenCand struct {
	Key             string  `json:"key"` // stack|setup|direction
	Status          string  `json:"status"`
	Tier            string  `json:"tier"`
	Reason          string  `json:"reason"`
	Class           string  `json:"class"`
	Admission       string  `json:"admission"`
	RequiredRR      float64 `json:"required_rr"`
	RequiredMinStop float64 `json:"required_min_stop"`
	Extra           float64 `json:"required_max_tp,omitempty"` // U6 only (goldenExtra)
}

type goldenRow struct {
	EventID string       `json:"event_id"`
	Config  string       `json:"config"`
	State   string       `json:"t2_state"`
	Cands   []goldenCand `json:"candidates"`
}

var goldenRows []goldenRow

func goldenAdd(eventID string, sig *local.V7T2Signal, natives []local.V7SignalOutput) {
	if *goldenOut == "" {
		return
	}
	var states []string
	if sig != nil {
		states = []string{sig.EffectiveStatus(), "TRIGGERED"}
	} else {
		states = []string{""}
	}
	for _, gc := range goldenConfigs() {
		geom := goldenGeometry(gc.cfg)
		for _, state := range states {
			sigs := append([]local.V7SignalOutput(nil), natives...)
			if sig != nil {
				sigs = append(sigs, sig.ToV7SignalOutputWithLifecycle("", state))
			}
			row := goldenRow{EventID: eventID, Config: gc.name, State: state}
			for _, c := range kernel.AssembleHunterV7CandidateCoins(sigs, "BOTH", geom) {
				row.Cands = append(row.Cands, goldenCand{
					Key: c.V7Stack + "|" + c.V7SetupType + "|" + c.Direction, Status: c.V7Status,
					Tier: c.V7ExecutionTier, Reason: c.V7TierReason, Class: c.V7SignalClass, Admission: c.V7ClassAdmission,
					RequiredRR: c.V7RequiredRR, RequiredMinStop: c.V7RequiredMinStop, Extra: goldenExtra(c),
				})
			}
			goldenRows = append(goldenRows, row)
		}
	}
}

func goldenWrite() {
	if *goldenOut == "" {
		return
	}
	b, _ := json.Marshal(goldenRows)
	_ = os.WriteFile(*goldenOut, b, 0o644)
}
