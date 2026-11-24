package provider_test

import (
	"testing"

	"github.com/Aixxww/AiT/mcp"
	_ "github.com/Aixxww/AiT/mcp/payment"
	"github.com/Aixxww/AiT/mcp/provider"
)

// Issue #68: every provider the decision path can use either streams through
// Client.CallWithRequestStream (OpenAI chat.completion SSE) or is explicitly
// kept on its non-stream call.
func TestSupportsChatCompletionStreamPerProvider(t *testing.T) {
	cases := map[string]bool{
		mcp.ProviderDeepSeek: true,
		mcp.ProviderOpenAI:   true,
		mcp.ProviderQwen:     true,
		mcp.ProviderGemini:   true, // OpenAI-compatible endpoint (/v1beta/openai)
		mcp.ProviderGrok:     true,
		mcp.ProviderKimi:     true,
		mcp.ProviderMiniMax:  true,
		mcp.ProviderMiMo:     true,
		mcp.ProviderClaude:   false, // Anthropic Messages SSE is a different format
		mcp.ProviderClaw402:  false, // x402 payment flow; streams via its own Call hook
	}
	for name, want := range cases {
		client := mcp.NewAIClientByProvider(name)
		if client == nil {
			t.Fatalf("provider %q not registered", name)
		}
		if got := mcp.SupportsChatCompletionStream(client); got != want {
			t.Errorf("SupportsChatCompletionStream(%s) = %v, want %v", name, got, want)
		}
	}

	// "custom" (e.g. WorkBuddy) is the plain base client built by mcp.New().
	custom := mcp.New()
	custom.SetAPIKey("k", "https://www.workbuddy.cn/v2", "deepseek-v4.1-flash")
	if !mcp.SupportsChatCompletionStream(custom) {
		t.Error("custom OpenAI-compatible client (WorkBuddy) must stream")
	}
}

// A *Client copied out of a Claude wrapper (as backtest.cloneBaseClient does)
// still dispatches through Claude hooks, so it must not report stream support.
func TestSupportsChatCompletionStreamFollowsHooks(t *testing.T) {
	claude := provider.NewClaudeClient().(*provider.ClaudeClient)
	cp := *claude.Client
	if mcp.SupportsChatCompletionStream(&cp) {
		t.Error("copied Claude base client must not report stream support")
	}
}
