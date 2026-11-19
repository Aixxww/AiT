package trader

import (
	"errors"
	"time"

	"github.com/Aixxww/AiT/store"
)

// U14: the protector replay walker (plan §7.3 "reuse ... as shared code, not
// as a copy"). It walks one position over 1-minute bars with a simulated
// venue: resting stop and take-profit orders fill intrabar at their price
// (stop checked first), and the protector polls once per bar at the close
// through protectPosition, the same step the live monitor runs, with the
// bar close time as the clock.

// protectorWalkBar is one 1-minute bar.
type protectorWalkBar struct {
	OpenMs                 int64
	Open, High, Low, Close float64
}

// protectorWalkInput is one position to walk.
type protectorWalkInput struct {
	Symbol, Side  string
	Entry         float64
	Stop          float64 // planned stop, resting on the venue at the open
	TakeProfit    float64 // the open's take-profit, resting on the venue (0 = none)
	TP0, TP1, TP2 float64 // planned targets (0 = none)
	Leverage      int
	Qty           float64
	Bars          []protectorWalkBar
	FillIdx       int
	Config        *store.ProtectorConfig // nil = defaults
}

// protectorWalkLeg is one exit fill.
type protectorWalkLeg struct {
	Qty, Price float64
	Why        string
}

// protectorWalkResult is the walk's outcome.
type protectorWalkResult struct {
	Legs      []protectorWalkLeg
	Exit      string
	Actions   []string
	StopMoves int
	// Orders is the venue's order log (stop / take-profit placements and
	// cancels, closes), for comparing the walker with the live adapter.
	Orders []string
}

func walkBarMs(b protectorWalkBar) int64 { return b.OpenMs + 60_000 }

// protectorWalk walks the position and returns its exits.
func protectorWalk(in protectorWalkInput) protectorWalkResult {
	long := in.Side == "long"
	v := newProtectorSimVenue(in.Symbol, in.Side, in.Entry, in.Qty, in.Leverage, in.Stop, in.TakeProfit)
	at := newProtectorWalkTrader(v, in.Config)
	e := at.protectorEngine()
	simOpen := time.UnixMilli(in.Bars[in.FillIdx].OpenMs)
	plan := plannedPositionRisk{stopLoss: in.Stop, takeProfit: in.TakeProfit, tp0Price: in.TP0, tp1Price: in.TP1, tp2Price: in.TP2}
	res := protectorWalkResult{Exit: "timeout"}
	for j := in.FillIdx; j < len(in.Bars) && v.qty > 0; j++ {
		b := in.Bars[j]
		if why, ok := v.fillResting(b, long); ok {
			res.Exit = why
			break
		}
		v.mark, v.why = b.Close, "protector"
		nLegs := len(v.legs)
		out := at.protectPosition(e, protectorObs{
			Symbol: in.Symbol, Side: in.Side, Entry: in.Entry, Mark: b.Close, Qty: v.qty, Leverage: in.Leverage,
			Now: time.UnixMilli(walkBarMs(b)), OpenedAt: simOpen, Plan: plan,
		})
		emergency := errors.Is(out.Err, errProtectionEmergencyClosed)
		why := string(out.Action)
		if emergency {
			why = "dynamic_stop_emergency"
		}
		for k := nLegs; k < len(v.legs); k++ {
			v.legs[k].Why = why
		}
		if emergency {
			res.Exit = "protector_close"
			break
		}
		if out.Action == protectionNone {
			continue
		}
		res.Actions = append(res.Actions, string(out.Action))
		if v.qty == 0 {
			res.Exit = "protector_close:" + string(out.Action)
			break
		}
	}
	if v.qty > 0 {
		v.mark, v.why = in.Bars[len(in.Bars)-1].Close, "timeout"
		v.closeAt(0)
	}
	res.Legs, res.StopMoves, res.Orders = v.legs, v.moves, v.orders
	return res
}

func newProtectorWalkTrader(v *protectorSimVenue, cfg *store.ProtectorConfig) *AutoTrader {
	return &AutoTrader{
		trader:                v,
		config:                AutoTraderConfig{StrategyConfig: &store.StrategyConfig{RiskControl: store.RiskControlConfig{Protector: cfg}}},
		protectionState:       map[string]*positionProtectionState{},
		peakPnLCache:          map[string]float64{},
		positionFirstSeenTime: map[string]int64{},
	}
}
