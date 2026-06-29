package hook

import (
	"testing"
)

// ---------------------------------------------------------------------------
// RegisterHook / HookExec
// ---------------------------------------------------------------------------

func TestRegisterHookAndExec(t *testing.T) {
	RegisterHook("test_hook", func(args ...any) any {
		s := args[0].(string)
		result := IpResult{IP: s}
		return &result
	})
	defer delete(Hooks, "test_hook")

	result := HookExec[IpResult]("test_hook", "1.2.3.4")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.IP != "1.2.3.4" {
		t.Fatalf("IP = %q, want %q", result.IP, "1.2.3.4")
	}
}

func TestHookExec_NotRegistered(t *testing.T) {
	result := HookExec[IpResult]("nonexistent_hook")
	if result != nil {
		t.Fatal("expected nil for unregistered hook")
	}
}

func TestHookExec_Disabled(t *testing.T) {
	origEnabled := EnableHooks
	EnableHooks = false
	defer func() { EnableHooks = origEnabled }()

	RegisterHook("disabled_hook", func(args ...any) any {
		result := IpResult{IP: "should-not-run"}
		return &result
	})
	defer delete(Hooks, "disabled_hook")

	result := HookExec[IpResult]("disabled_hook")
	if result != nil {
		t.Fatal("expected nil when hooks are disabled")
	}
}

// ---------------------------------------------------------------------------
// IpResult
// ---------------------------------------------------------------------------

func TestIpResult_GetResult(t *testing.T) {
	r := &IpResult{IP: "8.8.8.8"}
	if r.GetResult() != "8.8.8.8" {
		t.Fatalf("got %q", r.GetResult())
	}
	if r.Error() != nil {
		t.Fatal("expected no error")
	}
}

// ---------------------------------------------------------------------------
// SetHttpClientResult
// ---------------------------------------------------------------------------

func TestSetHttpClientResult_NoError(t *testing.T) {
	r := &SetHttpClientResult{}
	if r.Error() != nil {
		t.Fatal("expected no error")
	}
	if r.GetResult() != nil {
		t.Fatal("expected nil client when none set")
	}
}

// ---------------------------------------------------------------------------
// NewBinanceTraderResult / NewAsterTraderResult
// ---------------------------------------------------------------------------

func TestNewBinanceTraderResult_NoError(t *testing.T) {
	r := &NewBinanceTraderResult{}
	if r.Error() != nil {
		t.Fatal("expected no error")
	}
	if r.GetResult() != nil {
		t.Fatal("expected nil client")
	}
}

func TestNewAsterTraderResult_NoError(t *testing.T) {
	r := &NewAsterTraderResult{}
	if r.Error() != nil {
		t.Fatal("expected no error")
	}
	if r.GetResult() != nil {
		t.Fatal("expected nil client")
	}
}

// ---------------------------------------------------------------------------
// Hook constants
// ---------------------------------------------------------------------------

func TestHookConstants(t *testing.T) {
	if GETIP != "GETIP" {
		t.Fatalf("GETIP = %q", GETIP)
	}
	if NEW_BINANCE_TRADER != "NEW_BINANCE_TRADER" {
		t.Fatalf("NEW_BINANCE_TRADER = %q", NEW_BINANCE_TRADER)
	}
	if NEW_ASTER_TRADER != "NEW_ASTER_TRADER" {
		t.Fatalf("NEW_ASTER_TRADER = %q", NEW_ASTER_TRADER)
	}
	if SET_HTTP_CLIENT != "SET_HTTP_CLIENT" {
		t.Fatalf("SET_HTTP_CLIENT = %q", SET_HTTP_CLIENT)
	}
}
