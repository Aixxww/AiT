package trader

import (
	"errors"
	"strings"
	"testing"

	"github.com/Aixxww/AiT/trader/binance"
)

// Compile-time: the Binance trader supplies both optional capabilities.
var (
	_ sideScopedProtectionCanceller = (*binance.FuturesTrader)(nil)
	_ protectiveStopChecker         = (*binance.FuturesTrader)(nil)
)

type ownedCancelCall struct {
	symbol, positionSide string
	stopsOnly            bool
}

// rebuildTestTrader: an exchange with side-scoped cancel and stop check.
type rebuildTestTrader struct {
	protectionTestTrader
	ownedCancels    []ownedCancelCall
	ownedCancelErr  error
	setErr          error
	stopOpen        bool
	stopOpenErr     error
	symbolWideCalls int
}

func (t *rebuildTestTrader) CancelOwnedProtectionOrders(symbol, positionSide string, stopsOnly bool) error {
	t.ownedCancels = append(t.ownedCancels, ownedCancelCall{symbol, positionSide, stopsOnly})
	return t.ownedCancelErr
}
func (t *rebuildTestTrader) ProtectiveStopOpen(string, string) (bool, error) {
	return t.stopOpen, t.stopOpenErr
}
func (t *rebuildTestTrader) SetStopLoss(symbol, positionSide string, quantity, stopPrice float64) error {
	if t.setErr != nil {
		return t.setErr
	}
	return t.contextTestTrader.SetStopLoss(symbol, positionSide, quantity, stopPrice)
}
func (t *rebuildTestTrader) CancelStopOrders(string) error     { t.symbolWideCalls++; return nil }
func (t *rebuildTestTrader) CancelStopLossOrders(string) error { t.symbolWideCalls++; return nil }

// legacyTestTrader: an exchange without the optional capabilities.
type legacyTestTrader struct {
	protectionTestTrader
	stopOrdersCalls, stopLossCalls int
	setErr                         error
}

func (t *legacyTestTrader) CancelStopOrders(string) error     { t.stopOrdersCalls++; return nil }
func (t *legacyTestTrader) CancelStopLossOrders(string) error { t.stopLossCalls++; return nil }
func (t *legacyTestTrader) SetStopLoss(symbol, positionSide string, quantity, stopPrice float64) error {
	if t.setErr != nil {
		return t.setErr
	}
	return t.contextTestTrader.SetStopLoss(symbol, positionSide, quantity, stopPrice)
}

// F27: the breakeven rebuild cancels side-scoped, never symbol-wide, then
// places the stop. #93 / AUD-D5: stops only; the take-profit is kept.
func TestF27RebuildUsesSideScopedCancel(t *testing.T) {
	ft := &rebuildTestTrader{}
	at := &AutoTrader{trader: ft}
	closed, err := at.rebuildProtectionStopsWithBuffer("AAAUSDT", "short", 5, 100, 0.001)
	if err != nil || closed {
		t.Fatalf("rebuild: closed=%v err=%v", closed, err)
	}
	if len(ft.ownedCancels) != 1 || ft.ownedCancels[0] != (ownedCancelCall{"AAAUSDT", "SHORT", true}) {
		t.Fatalf("owned cancels = %+v, want one SHORT stops-only cancel", ft.ownedCancels)
	}
	if ft.symbolWideCalls != 0 {
		t.Fatalf("symbol-wide cancel called %d times", ft.symbolWideCalls)
	}
	if len(ft.stopLossCalls) != 1 || ft.stopLossCalls[0].positionSide != "SHORT" || ft.stopLossCalls[0].stopPrice != 100*(1-0.001) {
		t.Fatalf("stop placement = %+v", ft.stopLossCalls)
	}
}

// F27: the dynamic stop update cancels stops only, side-scoped.
func TestF27DynamicStopUsesStopsOnlySideScopedCancel(t *testing.T) {
	ft := &rebuildTestTrader{}
	at := &AutoTrader{trader: ft}
	placed, err := at.replaceDynamicStop("AAAUSDT", "long", 5, 98)
	if !placed || err != nil {
		t.Fatalf("placed=%v err=%v", placed, err)
	}
	if len(ft.ownedCancels) != 1 || ft.ownedCancels[0] != (ownedCancelCall{"AAAUSDT", "LONG", true}) || ft.symbolWideCalls != 0 {
		t.Fatalf("owned cancels = %+v symbol-wide = %d", ft.ownedCancels, ft.symbolWideCalls)
	}
}

// F27: exchanges without the capability keep their previous calls.
func TestF27FallbackKeepsPreviousCalls(t *testing.T) {
	ft := &legacyTestTrader{}
	at := &AutoTrader{trader: ft}
	if _, err := at.rebuildProtectionStopsWithBuffer("AAAUSDT", "long", 5, 100, 0.001); err != nil {
		t.Fatal(err)
	}
	if _, err := at.replaceDynamicStop("AAAUSDT", "long", 5, 99); err != nil {
		t.Fatal(err)
	}
	// #93 / AUD-D5: the rebuild now cancels stops only too (never the TP).
	if ft.stopOrdersCalls != 0 || ft.stopLossCalls != 2 {
		t.Fatalf("fallback calls: CancelStopOrders=%d CancelStopLossOrders=%d, want 0/2", ft.stopOrdersCalls, ft.stopLossCalls)
	}
}

// F28: stop placement fails but a stop still protects the side -> no close.
func TestF28FailedStopWithExistingStopDoesNotClose(t *testing.T) {
	ft := &rebuildTestTrader{setErr: errors.New("rejected"), stopOpen: true}
	at := &AutoTrader{trader: ft}
	closed, err := at.rebuildProtectionStopsWithBuffer("AAAUSDT", "long", 5, 100, 0.001)
	if closed || err == nil || !strings.Contains(err.Error(), "existing stop still protects") {
		t.Fatalf("closed=%v err=%v", closed, err)
	}
	if ft.closeCalls != 0 {
		t.Fatalf("close called %d times on a protected position", ft.closeCalls)
	}
}

// F28: stop placement fails and no stop is open -> emergency close, verified.
func TestF28FailedStopWithoutProtectionEmergencyCloses(t *testing.T) {
	ft := &rebuildTestTrader{setErr: errors.New("rejected"), stopOpen: false}
	at := &AutoTrader{trader: ft}
	closed, err := at.rebuildProtectionStopsWithBuffer("AAAUSDT", "long", 5, 100, 0.001)
	if !closed || !errors.Is(err, errProtectionEmergencyClosed) {
		t.Fatalf("closed=%v err=%v, want emergency close", closed, err)
	}
	if ft.closeCalls != 1 || ft.invalidatePositionsCalled != 1 {
		t.Fatalf("closeCalls=%d fresh reads=%d, want 1/1", ft.closeCalls, ft.invalidatePositionsCalled)
	}
}

// F28: unknown protection state counts as unprotected (F03 rule).
func TestF28UnknownProtectionStateEmergencyCloses(t *testing.T) {
	ft := &rebuildTestTrader{setErr: errors.New("rejected"), stopOpenErr: errors.New("timeout")}
	at := &AutoTrader{trader: ft}
	closed, err := at.rebuildProtectionStopsWithBuffer("AAAUSDT", "long", 5, 100, 0.001)
	if !closed || !errors.Is(err, errProtectionEmergencyClosed) {
		t.Fatalf("closed=%v err=%v", closed, err)
	}
}

// F28: a failed emergency close is reported and does not claim flat.
func TestF28EmergencyCloseFailureIsReported(t *testing.T) {
	ft := &rebuildTestTrader{setErr: errors.New("rejected")}
	ft.positionReads = [][]map[string]interface{}{stillLong}
	at := &AutoTrader{trader: ft}
	closed, err := at.rebuildProtectionStopsWithBuffer("AAAUSDT", "long", 5, 100, 0.001)
	if closed || err == nil || !strings.Contains(err.Error(), "emergency close FAILED") {
		t.Fatalf("closed=%v err=%v", closed, err)
	}
}

// F28: an unconfirmed cancel still places the new stop; nothing is closed.
func TestF28UnconfirmedCancelStillPlacesStop(t *testing.T) {
	ft := &rebuildTestTrader{ownedCancelErr: errors.New("1 owned stop order(s) still open")}
	at := &AutoTrader{trader: ft}
	closed, err := at.rebuildProtectionStopsWithBuffer("AAAUSDT", "long", 5, 100, 0.001)
	if closed || !errors.Is(err, errProtectionCancelUnconfirmed) {
		t.Fatalf("closed=%v err=%v", closed, err)
	}
	if len(ft.stopLossCalls) != 1 || ft.closeCalls != 0 {
		t.Fatalf("stops=%d closes=%d, want 1/0", len(ft.stopLossCalls), ft.closeCalls)
	}
	placed, err := at.replaceDynamicStop("AAAUSDT", "long", 5, 99)
	if !placed || err != nil {
		t.Fatalf("dynamic: placed=%v err=%v, want placed (new stop is on the exchange)", placed, err)
	}
}

// F28: a TP0 partial whose remainder cannot be re-protected reports the
// position closed, so the protector drops its state.
func TestF28PartialCloseReportsEmergencyClosedRemainder(t *testing.T) {
	ft := &rebuildTestTrader{setErr: errors.New("rejected")}
	at := &AutoTrader{trader: ft}
	closedAll, err := at.executeProtectionAction("AAAUSDT", "long", 10, 105, 100, protectionTP0)
	if err != nil || !closedAll {
		t.Fatalf("closedAll=%v err=%v, want remainder emergency-closed", closedAll, err)
	}
	if ft.closeCalls != 2 { // the TP0 partial, then the emergency close
		t.Fatalf("closeCalls = %d, want 2", ft.closeCalls)
	}
}
