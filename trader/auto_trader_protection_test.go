package trader

import (
	"errors"
	"strings"
	"testing"
)

// F03 tests for the emergency close of an unprotected position.

type protectionTestTrader struct {
	contextTestTrader
	closeErrs     []error                    // per CloseLong/CloseShort call; nil = success
	positionReads [][]map[string]interface{} // per fresh read; last one repeats
	closeCalls    int
	reads         int
}

func (t *protectionTestTrader) closeOnce() (map[string]interface{}, error) {
	i := t.closeCalls
	t.closeCalls++
	if i < len(t.closeErrs) && t.closeErrs[i] != nil {
		return nil, t.closeErrs[i]
	}
	return map[string]interface{}{"orderId": int64(1)}, nil
}
func (t *protectionTestTrader) CloseLong(string, float64) (map[string]interface{}, error) {
	return t.closeOnce()
}
func (t *protectionTestTrader) CloseShort(string, float64) (map[string]interface{}, error) {
	return t.closeOnce()
}
func (t *protectionTestTrader) GetPositions() ([]map[string]interface{}, error) {
	i := t.reads
	t.reads++
	if len(t.positionReads) == 0 {
		return nil, nil
	}
	if i >= len(t.positionReads) {
		i = len(t.positionReads) - 1
	}
	return t.positionReads[i], nil
}

var stillLong = []map[string]interface{}{{"symbol": "AAAUSDT", "side": "long"}}

func TestF03EmergencyCloseFlatOnFirstAttempt(t *testing.T) {
	ft := &protectionTestTrader{}
	at := &AutoTrader{trader: ft}
	err := at.closeUnprotectedPosition("AAAUSDT", "long", 1, errors.New("failed to set mandatory stop loss: boom"))
	if err == nil || !strings.Contains(err.Error(), "emergency-closed") {
		t.Fatalf("want emergency-closed error, got %v", err)
	}
	if ft.closeCalls != 1 || ft.invalidatePositionsCalled != 1 {
		t.Fatalf("closeCalls=%d invalidations=%d, want 1/1 (fresh read)", ft.closeCalls, ft.invalidatePositionsCalled)
	}
}

func TestF03EmergencyCloseRetriesUntilFlat(t *testing.T) {
	ft := &protectionTestTrader{
		closeErrs:     []error{errors.New("timeout"), nil, nil},
		positionReads: [][]map[string]interface{}{stillLong, nil},
	}
	at := &AutoTrader{trader: ft}
	if err := at.emergencyClosePositionAndVerify("AAAUSDT", "long", 1); err != nil {
		t.Fatalf("want flat after retries, got %v", err)
	}
	if ft.closeCalls != 3 {
		t.Fatalf("closeCalls = %d, want 3 (error, residual, flat)", ft.closeCalls)
	}
}

func TestF03EmergencyCloseReportsFailureWhenResidualRemains(t *testing.T) {
	ft := &protectionTestTrader{positionReads: [][]map[string]interface{}{stillLong}}
	at := &AutoTrader{trader: ft}
	err := at.closeUnprotectedPosition("AAAUSDT", "long", 1, errors.New("failed to set mandatory stop loss"))
	if err == nil || !strings.Contains(err.Error(), "emergency close FAILED") || !strings.Contains(err.Error(), "residual") {
		t.Fatalf("want explicit failure with residual, got %v", err)
	}
	if ft.closeCalls != emergencyCloseAttempts {
		t.Fatalf("closeCalls = %d, want %d", ft.closeCalls, emergencyCloseAttempts)
	}
}

func TestF03OppositeSidePositionIsNotResidual(t *testing.T) {
	ft := &protectionTestTrader{positionReads: [][]map[string]interface{}{{{"symbol": "AAAUSDT", "side": "short"}}}}
	at := &AutoTrader{trader: ft}
	if err := at.emergencyClosePositionAndVerify("AAAUSDT", "long", 1); err != nil {
		t.Fatalf("an opposite-side position must not count as residual, got %v", err)
	}
}
