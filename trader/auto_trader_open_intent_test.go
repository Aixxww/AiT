package trader

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Aixxww/AiT/kernel"
)

// F04 tests: deterministic client ids and read-back of ambiguous opens.

type intentTestTrader struct {
	contextTestTrader
	openErr     error
	lookups     []intentLookup // per LookupOrderByClientID call; last repeats
	opens       []string       // client ids submitted
	lookupCalls int
	legacyOpens int
}

type intentLookup struct {
	order map[string]interface{}
	found bool
	err   error
}

var errTimeout = errors.New("read tcp: i/o timeout")
var errRejected = errors.New("api: -2019 margin is insufficient")

func (t *intentTestTrader) OpenLong(string, float64, int) (map[string]interface{}, error) {
	t.legacyOpens++
	return nil, nil
}
func (t *intentTestTrader) OpenLongWithClientID(_ string, _ float64, _ int, cid string) (map[string]interface{}, error) {
	t.opens = append(t.opens, cid)
	if t.openErr != nil {
		return nil, t.openErr
	}
	return map[string]interface{}{"orderId": int64(7), "clientOrderId": cid}, nil
}
func (t *intentTestTrader) OpenShortWithClientID(s string, q float64, l int, cid string) (map[string]interface{}, error) {
	return t.OpenLongWithClientID(s, q, l, cid)
}
func (t *intentTestTrader) LookupOrderByClientID(string, string) (map[string]interface{}, bool, error) {
	i := t.lookupCalls
	t.lookupCalls++
	if i >= len(t.lookups) {
		i = len(t.lookups) - 1
	}
	l := t.lookups[i]
	return l.order, l.found, l.err
}
func (t *intentTestTrader) IsAmbiguousOrderError(err error) bool { return errors.Is(err, errTimeout) }
func (t *intentTestTrader) ClientOrderIDForIntent(key string) string {
	return "cid:" + key
}

func intentTrader(ft *intentTestTrader) *AutoTrader {
	return &AutoTrader{id: "tr1", callCount: 42, trader: ft, readBackSleep: func(time.Duration) {}}
}

func intentDecision() *kernel.Decision {
	return &kernel.Decision{Symbol: "AAAUSDT", Action: "open_long", Leverage: 5, SelectedHunterV7SignalID: "sig-1"}
}

func TestF04DeterministicClientID(t *testing.T) {
	ft := &intentTestTrader{}
	at := intentTrader(ft)
	if _, err := at.openMarketPosition(intentDecision(), "long", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := at.openMarketPosition(intentDecision(), "long", 1); err != nil {
		t.Fatal(err)
	}
	if len(ft.opens) != 2 || ft.opens[0] != ft.opens[1] || ft.opens[0] != "cid:tr1|42|AAAUSDT|long|sig-1" {
		t.Fatalf("client ids = %v, want the same deterministic id twice", ft.opens)
	}
	at.callCount = 43
	at.openMarketPosition(intentDecision(), "long", 1)
	if ft.opens[2] == ft.opens[0] {
		t.Fatalf("a new cycle must give a new intent id")
	}
}

func TestF04AmbiguousErrorOrderFoundIsAdopted(t *testing.T) {
	ft := &intentTestTrader{openErr: errTimeout, lookups: []intentLookup{{order: map[string]interface{}{"orderId": int64(9), "status": "FILLED"}, found: true}}}
	order, err := intentTrader(ft).openMarketPosition(intentDecision(), "long", 1)
	if err != nil || order["adopted"] != true || order["orderId"] != int64(9) {
		t.Fatalf("want adopted order, got %v / %v", order, err)
	}
	if len(ft.opens) != 1 {
		t.Fatalf("order must not be resubmitted, opens=%d", len(ft.opens))
	}
}

func TestF04AmbiguousErrorAbsentTwiceIsNotPlaced(t *testing.T) {
	ft := &intentTestTrader{openErr: errTimeout, lookups: []intentLookup{{found: false}}}
	_, err := intentTrader(ft).openMarketPosition(intentDecision(), "long", 1)
	if err == nil || !strings.Contains(err.Error(), "absent on 2 reads") || ft.lookupCalls != 2 {
		t.Fatalf("want not-placed after 2 absent reads, got %v (lookups=%d)", err, ft.lookupCalls)
	}
}

func TestF04AbsentThenFoundIsAdopted(t *testing.T) {
	ft := &intentTestTrader{openErr: errTimeout, lookups: []intentLookup{{found: false},
		{order: map[string]interface{}{"orderId": int64(5), "status": "NEW"}, found: true}}}
	order, err := intentTrader(ft).openMarketPosition(intentDecision(), "long", 1)
	if err != nil || order["adopted"] != true {
		t.Fatalf("a late-visible order must be adopted (read-after-write lag), got %v / %v", order, err)
	}
}

func TestF04ReadFailureIsUnknownNotRetried(t *testing.T) {
	ft := &intentTestTrader{openErr: errTimeout, lookups: []intentLookup{{err: errors.New("429")}}}
	_, err := intentTrader(ft).openMarketPosition(intentDecision(), "long", 1)
	if err == nil || !strings.Contains(err.Error(), "order_state_unknown") || len(ft.opens) != 1 {
		t.Fatalf("want order_state_unknown without resubmit, got %v (opens=%d)", err, len(ft.opens))
	}
}

func TestF04FoundButRejectedReturnsError(t *testing.T) {
	ft := &intentTestTrader{openErr: errTimeout, lookups: []intentLookup{{order: map[string]interface{}{"status": "EXPIRED"}, found: true}}}
	if _, err := intentTrader(ft).openMarketPosition(intentDecision(), "long", 1); err == nil || !strings.Contains(err.Error(), "EXPIRED") {
		t.Fatalf("want error for an expired order, got %v", err)
	}
}

func TestF04DefiniteRejectionNoReadBack(t *testing.T) {
	ft := &intentTestTrader{openErr: errRejected}
	if _, err := intentTrader(ft).openMarketPosition(intentDecision(), "long", 1); !errors.Is(err, errRejected) || ft.lookupCalls != 0 {
		t.Fatalf("definite rejection must return as-is without read-back, got %v (lookups=%d)", err, ft.lookupCalls)
	}
}

func TestF04LegacyTraderUnchanged(t *testing.T) {
	ft := &contextTestTrader{}
	at := &AutoTrader{trader: ft}
	if _, err := at.openMarketPosition(intentDecision(), "long", 1); err != nil {
		t.Fatalf("legacy trader path must still work, got %v", err)
	}
}
