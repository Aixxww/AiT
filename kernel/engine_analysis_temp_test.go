package kernel

import "testing"

// P0-4: hunter_v7 开仓决策降温 —— 构造出的请求 temperature 必须为 0.15。
func TestBuildHunterV7DecisionRequestUsesCooledTemperature(t *testing.T) {
	req, err := buildHunterV7DecisionRequest("system prompt", "user prompt")
	if err != nil {
		t.Fatalf("buildHunterV7DecisionRequest failed: %v", err)
	}
	if req.Temperature == nil {
		t.Fatal("Temperature is nil, want non-nil 0.15")
	}
	if *req.Temperature != 0.15 {
		t.Fatalf("Temperature = %v, want 0.15", *req.Temperature)
	}
	if *req.Temperature != hunterV7DecisionTemperature {
		t.Fatalf("Temperature = %v, want hunterV7DecisionTemperature = %v", *req.Temperature, hunterV7DecisionTemperature)
	}
}

// 降温改动不应破坏原有行为：max tokens 上限与 prompt 仍然正确装配。
func TestBuildHunterV7DecisionRequestPreservesMaxTokensAndPrompts(t *testing.T) {
	sys, user := "sys prompt", "user prompt"
	req, err := buildHunterV7DecisionRequest(sys, user)
	if err != nil {
		t.Fatalf("buildHunterV7DecisionRequest failed: %v", err)
	}
	if req.MaxTokens == nil || *req.MaxTokens != hunterV7DecisionMaxOutputTokens {
		t.Fatalf("MaxTokens = %v, want %d", req.MaxTokens, hunterV7DecisionMaxOutputTokens)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("Messages len = %d, want 2", len(req.Messages))
	}
	if req.Messages[0].Role != "system" || req.Messages[0].Content != sys {
		t.Fatalf("system message = %+v, want role=system content=%q", req.Messages[0], sys)
	}
	if req.Messages[1].Role != "user" || req.Messages[1].Content != user {
		t.Fatalf("user message = %+v, want role=user content=%q", req.Messages[1], user)
	}
}

// 空 prompt 时 Build 应失败 —— 触发 callStrategyDecisionAI 的 fallback 路径。
func TestBuildHunterV7DecisionRequestEmptyPromptsFails(t *testing.T) {
	if _, err := buildHunterV7DecisionRequest("", ""); err == nil {
		t.Fatal("expected error for empty prompts, got nil")
	}
}
