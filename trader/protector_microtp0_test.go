package trader

// Unit tests for P1-4 micro_tp0: planned TP0 distance cap.
import "testing"

func TestMicroTP0Price(t *testing.T) {
	// Long, entry 100, SL 98 (2% dist), planned TP0 101.5 (1.5% away, too far).
	// micro = max(0.55%*100=0.55, 0.45*2=0.9) = 0.9 -> TP0 = 100.9
	got := microTP0Price("long", 100, 98, 101.5)
	if got != 100.9 {
		t.Errorf("long far TP0: got %.4f, want 100.9", got)
	}

	// Short, entry 100, SL 102, planned TP0 98.5 -> micro 0.9 -> 99.1
	got = microTP0Price("short", 100, 102, 98.5)
	if got != 99.1 {
		t.Errorf("short far TP0: got %.4f, want 99.1", got)
	}

	// Already closer than micro: untouched.
	got = microTP0Price("long", 100, 98, 100.5)
	if got != 100.5 {
		t.Errorf("close TP0 should be untouched, got %.4f", got)
	}

	// No stop loss: micro = max(0.55, 0) = 0.55
	got = microTP0Price("long", 100, 0, 101.5)
	if got != 100.55 {
		t.Errorf("no-SL TP0: got %.4f, want 100.55", got)
	}

	// Not a profit target (TP below entry for long): untouched.
	got = microTP0Price("long", 100, 98, 99.5)
	if got != 99.5 {
		t.Errorf("non-profit TP0 should be untouched, got %.4f", got)
	}

	// Invalid inputs: passthrough.
	if got := microTP0Price("long", 0, 98, 101.5); got != 101.5 {
		t.Errorf("zero entry should passthrough, got %.4f", got)
	}
}
