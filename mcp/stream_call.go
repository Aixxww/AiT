package mcp

// Some OpenAI-compatible gateways (e.g. WorkBuddy, https://www.workbuddy.cn/v2)
// reject chat requests that do not set "stream": true (HTTP 400, code 11101).
// Callers that cannot know which gateway sits behind a custom base URL use the
// helpers below, which send a streaming request whenever the client's wire
// format supports OpenAI chat.completion SSE.

// ChatCompletionStreamUnsupported is a marker implemented by client hooks whose
// wire format is not OpenAI chat.completion SSE, so Client.CallWithRequestStream
// cannot parse their responses. Examples: the Anthropic Messages API (Claude)
// and the x402 payment proxy (claw402), which streams through its own Call hook.
type ChatCompletionStreamUnsupported interface {
	ChatCompletionStreamUnsupported()
}

// SupportsChatCompletionStream reports whether c.CallWithRequestStream can be
// used for c. It inspects the client's Hooks, because Hooks decide the wire
// format even when a provider's *Client has been copied out of its wrapper.
// Implementations that do not embed *Client (wrappers, test fakes) report
// false so they keep their non-stream behaviour.
func SupportsChatCompletionStream(c AIClient) bool {
	if _, ok := c.(ChatCompletionStreamUnsupported); ok {
		return false
	}
	embedder, ok := c.(ClientEmbedder)
	if !ok {
		return false
	}
	base := embedder.BaseClient()
	if base == nil {
		return false
	}
	if _, ok := base.Hooks.(ChatCompletionStreamUnsupported); ok {
		return false
	}
	return true
}

// CallWithRequestPreferStream sends req as a streaming request when the client
// supports chat.completion SSE, and through CallWithRequest otherwise.
func CallWithRequestPreferStream(c AIClient, req *Request) (string, error) {
	if !SupportsChatCompletionStream(c) {
		// CallWithRequest reads a single JSON body, so it must not ask for SSE.
		nonStream := *req
		nonStream.Stream = false
		return c.CallWithRequest(&nonStream)
	}
	return c.CallWithRequestStream(req, nil)
}

// CallWithMessagesPreferStream is the streaming counterpart of
// c.CallWithMessages(systemPrompt, userPrompt). The request carries no
// temperature or max-token overrides, so the body matches CallWithMessages
// (client temperature, client max tokens) plus "stream": true.
func CallWithMessagesPreferStream(c AIClient, systemPrompt, userPrompt string) (string, error) {
	if !SupportsChatCompletionStream(c) {
		return c.CallWithMessages(systemPrompt, userPrompt)
	}
	req, err := NewRequestBuilder().
		WithSystemPrompt(systemPrompt).
		WithUserPrompt(userPrompt).
		Build()
	if err != nil {
		return c.CallWithMessages(systemPrompt, userPrompt)
	}
	return c.CallWithRequestStream(req, nil)
}
