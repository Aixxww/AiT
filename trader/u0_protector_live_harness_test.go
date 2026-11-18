package trader

// U0-H2 runner: arm A on the live trades through the unchanged U0-F model-A
// walk (u0fWalkA). Gated by U0H2_RUN=1; U0H2_IN is the per-trade input built
// by scripts/t2_u0/u0h2_inputs.py, U0H2_OUT the per-trade A rows. The test was
// pre-registered (docs/t2-u0-preregistrations.md) and has been run once; the
// runner is kept for audit, not for re-running the decision.

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/Aixxww/AiT/cmd/t2_replay/replay"
	"github.com/Aixxww/AiT/kernel"
)

type u0h2In struct {
	Pos    int         `json:"pos"`
	Symbol string      `json:"symbol"`
	Side   string      `json:"side"`
	Entry  float64     `json:"entry"`
	Stop   float64     `json:"stop"`
	TP     float64     `json:"tp"`
	Lev    int         `json:"dec_lev"`
	Bars   [][]float64 `json:"bars"`
}

func TestU0H2A(t *testing.T) {
	if os.Getenv("U0H2_RUN") != "1" {
		t.Skip("U0H2_RUN=1 to run")
	}
	raw, err := os.ReadFile(os.Getenv("U0H2_IN"))
	if err != nil {
		t.Fatal(err)
	}
	var in []u0h2In
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatal(err)
	}
	excluded := map[int]bool{9: true, 11: true} // addendum: owner ruling (a)
	tpCap := kernel.HunterV7EffectiveExecutionGeometry(3.0, 0, 0.5, 3.0, true).MaxTPMovePct
	type out struct {
		Pos   int      `json:"pos"`
		A     float64  `json:"a_r"`
		AExit string   `json:"a_exit"`
		Acts  []string `json:"a_actions"`
	}
	var outs []out
	for _, tr := range in {
		if excluded[tr.Pos] {
			continue
		}
		bars := make([]replay.Bar, 0, len(tr.Bars))
		for _, b := range tr.Bars {
			bars = append(bars, replay.Bar{OpenTime: int64(b[0]), Open: b[1], High: b[2], Low: b[3], Close: b[4]})
		}
		long := tr.Side == "LONG"
		sgn, side := 1.0, "long"
		if !long {
			sgn, side = -1, "short"
		}
		E, S := tr.Entry, tr.Stop
		d := math.Abs(E - S)
		u := func(p float64) float64 { return sgn * (p - E) / d }
		fav := func(b replay.Bar) float64 {
			if long {
				return b.High
			}
			return b.Low
		}
		adv := func(b replay.Bar) float64 {
			if long {
				return b.Low
			}
			return b.High
		}
		hit := func(px, lvl float64, favourable bool) bool {
			if favourable == long {
				return px >= lvl
			}
			return px <= lvl
		}
		tp := tr.TP
		if c := E * (1 + sgn*tpCap/100); sgn*(tp-c) > 0 {
			tp = c
		}
		var row u0fRow
		// T1 = T2 = 0: the live decisions carry no HunterV7 TP1/TP2, so the
		// production protector has no planned targets beyond take_profit.
		u0fWalkA(tr.Symbol, long, sgn, side, E, S, 0, 0, tp, tr.Lev, bars, 0, u, fav, adv, hit, d, &row)
		outs = append(outs, out{tr.Pos, row.A, row.AExit, row.AActions})
	}
	b, _ := json.Marshal(outs)
	if err := os.WriteFile(os.Getenv("U0H2_OUT"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("trades %d", len(outs))
}
