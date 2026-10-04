package backtest

import "testing"

// validTestConfig returns a minimal config that passes Validate().
func validTestConfig() *BacktestConfig {
	return &BacktestConfig{
		RunID:    "test-run",
		Symbols:  []string{"BTCUSDT"},
		StartTS:  1000,
		EndTS:    2000,
	}
}

func TestValidateFeeSlippageDefaults(t *testing.T) {
	cfg := validTestConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() returned unexpected error: %v", err)
	}
	if cfg.FeeBps != DefaultFeeBps {
		t.Errorf("expected FeeBps default %v, got %v", DefaultFeeBps, cfg.FeeBps)
	}
	if cfg.SlippageBps != DefaultSlippageBps {
		t.Errorf("expected SlippageBps default %v, got %v", DefaultSlippageBps, cfg.SlippageBps)
	}
	if DefaultFeeBps != 5 || DefaultSlippageBps != 5 {
		t.Errorf("expected defaults to be 5/5, got %v/%v", DefaultFeeBps, DefaultSlippageBps)
	}
}

func TestValidateFeeSlippageExplicitValuesKept(t *testing.T) {
	cfg := validTestConfig()
	cfg.FeeBps = 8
	cfg.SlippageBps = 8
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() returned unexpected error: %v", err)
	}
	if cfg.FeeBps != 8 {
		t.Errorf("expected FeeBps 8 to be kept, got %v", cfg.FeeBps)
	}
	if cfg.SlippageBps != 8 {
		t.Errorf("expected SlippageBps 8 to be kept, got %v", cfg.SlippageBps)
	}
}

func TestValidateFeeSlippageNegativeRejected(t *testing.T) {
	cfg := validTestConfig()
	cfg.FeeBps = -1
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected error for negative fee_bps, got nil")
	}

	cfg = validTestConfig()
	cfg.SlippageBps = -0.5
	if err := cfg.Validate(); err == nil {
		t.Errorf("expected error for negative slippage_bps, got nil")
	}
}
