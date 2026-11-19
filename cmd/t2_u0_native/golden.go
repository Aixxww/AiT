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
	"github.com/Aixxww/AiT/store"
)

var goldenOut = flag.String("golden-out", "", "U6 golden replay output (candidate set and tiers per config)")

// U11: emit the T2 signal as t2_<setup> before assembly. The golden key
// carries the upstream name, so with the prefix the output must be
// byte-identical to the run without it.
var goldenT2Prefix = flag.Bool("golden-t2-prefix", false, "U11: assemble T2 signals under their t2_ setup name")

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
				t2 := sig.ToV7SignalOutputWithLifecycle("", state)
				if *goldenT2Prefix {
					t2.SetupType = local.V7SetupType(store.V7T2SetupPrefix + string(t2.SetupType))
				}
				sigs = append(sigs, t2)
			}
			row := goldenRow{EventID: eventID, Config: gc.name, State: state}
			for _, c := range kernel.AssembleHunterV7CandidateCoins(sigs, "BOTH", geom) {
				row.Cands = append(row.Cands, goldenCand{
					// U11: the upstream (unprefixed) setup name.
					Key: c.V7Stack + "|" + store.V7CanonicalSetupType(c.V7SetupType) + "|" + c.Direction, Status: c.V7Status,
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
