package kernel

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Aixxww/AiT/mcp"
	"github.com/Aixxww/AiT/store"
)

// workBuddyNonStreamError is the exact body the WorkBuddy v2 gateway returns
// for a chat request without "stream": true (issue #68).
const workBuddyNonStreamError = `{"code":11101,"msg":"Non-stream chat request is currently not supported","retry":false}`

const streamTestDecisionText = `[{"symbol":"BTCUSDT","action":"wait","reasoning":"stub"}]`

// workBuddyStub mimics the WorkBuddy gateway: non-stream chat requests get
// 400/11101; streaming requests get an OpenAI-style SSE response.
type workBuddyStub struct {
	mu     sync.Mutex
	bodies []map[string]any
}

func (s *workBuddyStub) handler(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	s.mu.Lock()
	s.bodies = append(s.bodies, body)
	s.mu.Unlock()

	if stream, _ := body["stream"].(bool); !stream {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, workBuddyNonStreamError)
		return
	}
	writeSSEDecision(w, streamTestDecisionText)
}

func (s *workBuddyStub) requests() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]map[string]any(nil), s.bodies...)
}

// writeSSEDecision splits text into several delta chunks, then sends a usage
// chunk and [DONE], the way OpenAI-compatible gateways do.
func writeSSEDecision(w http.ResponseWriter, text string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	for i := 0; i < len(text); i += 7 {
		end := i + 7
		if end > len(text) {
			end = len(text)
		}
		chunk, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{{"delta": map[string]any{"content": text[i:end]}}},
		})
		fmt.Fprintf(w, "data: %s\n\n", chunk)
	}
	fmt.Fprint(w, `data: {"choices":[],"usage":{"prompt_tokens":120,"completion_tokens":30,"total_tokens":150}}`+"\n\n")
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func newWorkBuddyClient(t *testing.T, baseURL string) mcp.AIClient {
	t.Helper()
	// Same construction as trader/auto_trader.go for aiModel == "custom"
	// (mcp.New() == mcp.NewClient()), except the SSRF-guarded default HTTP
	// client is swapped for a plain one so the loopback stub is reachable.
	client := mcp.NewClient(mcp.WithHTTPClient(&http.Client{}))
	client.SetAPIKey("test-key", baseURL, "deepseek-v4.1-flash")
	return client
}

func hunterV7TestEngine() *StrategyEngine {
	return NewStrategyEngine(&store.StrategyConfig{
		CoinSource: store.CoinSourceConfig{SourceType: "hunter_v7"},
	})
}

// Issue #68: the hunter_v7 decision call must stream so the WorkBuddy gateway
// accepts it, and must keep temperature 0.15 / max_tokens 1600.
func TestCallStrategyDecisionAI_HunterV7StreamsAgainstWorkBuddy(t *testing.T) {
	stub := &workBuddyStub{}
	srv := httptest.NewServer(http.HandlerFunc(stub.handler))
	defer srv.Close()

	client := newWorkBuddyClient(t, srv.URL+"/v2")
	got, err := callStrategyDecisionAI(client, hunterV7TestEngine(), "system prompt", "user prompt")
	if err != nil {
		t.Fatalf("decision call failed: %v", err)
	}
	if got != streamTestDecisionText {
		t.Fatalf("decision text = %q, want %q", got, streamTestDecisionText)
	}

	reqs := stub.requests()
	if len(reqs) != 1 {
		t.Fatalf("gateway saw %d requests, want 1", len(reqs))
	}
	body := reqs[0]
	if stream, _ := body["stream"].(bool); !stream {
		t.Fatalf(`request body missing "stream": true: %v`, body)
	}
	if temp, _ := body["temperature"].(float64); temp != hunterV7DecisionTemperature {
		t.Fatalf("temperature = %v, want %v", body["temperature"], hunterV7DecisionTemperature)
	}
	if maxTok, _ := body["max_tokens"].(float64); int(maxTok) != hunterV7DecisionMaxOutputTokens {
		t.Fatalf("max_tokens = %v, want %d", body["max_tokens"], hunterV7DecisionMaxOutputTokens)
	}

	base := client.(mcp.ClientEmbedder).BaseClient()
	if base.LastUsage.TotalTokens != 150 || base.LastUsage.PromptTokens != 120 || base.LastUsage.CompletionTokens != 30 {
		t.Fatalf("LastUsage = %+v, want prompt=120 completion=30 total=150", base.LastUsage)
	}
}

// The non-hunter_v7 fallback (CallWithMessages) is also a decision call and
// must stream as well.
func TestCallStrategyDecisionAI_DefaultPathStreamsAgainstWorkBuddy(t *testing.T) {
	stub := &workBuddyStub{}
	srv := httptest.NewServer(http.HandlerFunc(stub.handler))
	defer srv.Close()

	client := newWorkBuddyClient(t, srv.URL+"/v2")
	engine := NewStrategyEngine(&store.StrategyConfig{})
	got, err := callStrategyDecisionAI(client, engine, "system prompt", "user prompt")
	if err != nil {
		t.Fatalf("decision call failed: %v", err)
	}
	if got != streamTestDecisionText {
		t.Fatalf("decision text = %q, want %q", got, streamTestDecisionText)
	}
	reqs := stub.requests()
	if len(reqs) != 1 {
		t.Fatalf("gateway saw %d requests, want 1", len(reqs))
	}
	if stream, _ := reqs[0]["stream"].(bool); !stream {
		t.Fatalf(`request body missing "stream": true: %v`, reqs[0])
	}
}

// A 400 from the gateway must not be retried, and must not be retried as a
// non-stream request.
func TestCallStrategyDecisionAI_400IsNotRetried(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, workBuddyNonStreamError)
	}))
	defer srv.Close()

	client := newWorkBuddyClient(t, srv.URL+"/v2")
	_, err := callStrategyDecisionAI(client, hunterV7TestEngine(), "system prompt", "user prompt")
	if err == nil || !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("err = %v, want status 400 error", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("gateway saw %d requests, want exactly 1 (no retry)", len(bodies))
	}
	if stream, _ := bodies[0]["stream"].(bool); !stream {
		t.Fatalf(`request body missing "stream": true: %v`, bodies[0])
	}
}
