package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

const streamTestText = `{"decisions":[{"symbol":"ETHUSDT","action":"open_long"}]}`

// dualModeServer answers like an OpenAI-compatible provider that supports
// both shapes: JSON for non-stream requests, SSE for "stream": true.
type dualModeServer struct {
	mu       sync.Mutex
	bodies   []map[string]any
	failures []int // status codes to return before succeeding
	noSpace  bool  // emit "data:{...}" instead of "data: {...}"
}

func (s *dualModeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)

	s.mu.Lock()
	s.bodies = append(s.bodies, body)
	var fail int
	if len(s.failures) > 0 {
		fail, s.failures = s.failures[0], s.failures[1:]
	}
	s.mu.Unlock()

	if fail != 0 {
		w.WriteHeader(fail)
		fmt.Fprintf(w, `{"error":"injected %d"}`, fail)
		return
	}

	if stream, _ := body["stream"].(bool); !stream {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`, streamTestText)
		return
	}

	prefix := "data: "
	if s.noSpace {
		prefix = "data:"
	}
	w.Header().Set("Content-Type", "text/event-stream")
	for i := 0; i < len(streamTestText); i += 5 {
		end := i + 5
		if end > len(streamTestText) {
			end = len(streamTestText)
		}
		chunk, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{{"delta": map[string]any{"content": streamTestText[i:end]}}},
		})
		fmt.Fprintf(w, "%s%s\n\n", prefix, chunk)
	}
	fmt.Fprintf(w, "%s%s\n\n", prefix, `{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
	fmt.Fprintf(w, "%s[DONE]\n\n", prefix)
}

func (s *dualModeServer) seen() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]map[string]any(nil), s.bodies...)
}

func newStreamTestClient(t *testing.T, url string) *Client {
	t.Helper()
	c := NewClient(
		WithHTTPClient(&http.Client{Timeout: 10 * time.Second}),
		WithLogger(NewNoopLogger()),
		WithRetryWaitBase(time.Millisecond),
	).(*Client)
	c.SetAPIKey("test-key", url, "test-model")
	return c
}

func newStreamTestRequest(t *testing.T) *Request {
	t.Helper()
	req, err := NewRequestBuilder().
		WithSystemPrompt("sys").
		WithUserPrompt("user").
		WithTemperature(0.15).
		WithMaxTokens(1600).
		Build()
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	return req
}

func TestCallWithRequestStream_SameTextAsNonStream(t *testing.T) {
	srv := &dualModeServer{}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	c := newStreamTestClient(t, ts.URL)

	nonStream, err := c.CallWithRequest(newStreamTestRequest(t))
	if err != nil {
		t.Fatalf("non-stream call: %v", err)
	}
	streamed, err := c.CallWithRequestStream(newStreamTestRequest(t), nil)
	if err != nil {
		t.Fatalf("stream call: %v", err)
	}
	if streamed != nonStream || streamed != streamTestText {
		t.Fatalf("stream text %q != non-stream text %q (want %q)", streamed, nonStream, streamTestText)
	}

	bodies := srv.seen()
	if len(bodies) != 2 {
		t.Fatalf("server saw %d requests, want 2", len(bodies))
	}
	if _, ok := bodies[0]["stream"]; ok {
		t.Fatalf("non-stream body must not carry stream: %v", bodies[0])
	}
	if stream, _ := bodies[1]["stream"].(bool); !stream {
		t.Fatalf(`stream body missing "stream": true: %v`, bodies[1])
	}
	opts, _ := bodies[1]["stream_options"].(map[string]any)
	if include, _ := opts["include_usage"].(bool); !include {
		t.Fatalf("stream body missing stream_options.include_usage: %v", bodies[1])
	}
	// Apart from stream / stream_options, both bodies are identical.
	for _, key := range []string{"model", "temperature", "max_tokens"} {
		if fmt.Sprint(bodies[0][key]) != fmt.Sprint(bodies[1][key]) {
			t.Fatalf("%s differs: non-stream=%v stream=%v", key, bodies[0][key], bodies[1][key])
		}
	}
	if c.LastUsage.TotalTokens != 15 {
		t.Fatalf("LastUsage after stream = %+v, want total 15", c.LastUsage)
	}
}

func TestCallWithRequestStream_ReportsUsage(t *testing.T) {
	ts := httptest.NewServer(&dualModeServer{})
	defer ts.Close()
	c := newStreamTestClient(t, ts.URL)

	var got []TokenUsage
	prev := TokenUsageCallback
	TokenUsageCallback = func(u TokenUsage) { got = append(got, u) }
	defer func() { TokenUsageCallback = prev }()

	if _, err := c.CallWithRequestStream(newStreamTestRequest(t), nil); err != nil {
		t.Fatalf("stream call: %v", err)
	}
	if len(got) != 1 || got[0].TotalTokens != 15 || got[0].Model != "test-model" {
		t.Fatalf("usage callbacks = %+v, want one with total 15 for test-model", got)
	}
}

func TestCallWithRequestStream_RetriesRetryableStatus(t *testing.T) {
	srv := &dualModeServer{failures: []int{http.StatusBadGateway}}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	c := newStreamTestClient(t, ts.URL)

	got, err := c.CallWithRequestStream(newStreamTestRequest(t), nil)
	if err != nil {
		t.Fatalf("stream call: %v", err)
	}
	if got != streamTestText {
		t.Fatalf("text = %q, want %q", got, streamTestText)
	}
	bodies := srv.seen()
	if len(bodies) != 2 {
		t.Fatalf("server saw %d requests, want 2 (one 502, one retry)", len(bodies))
	}
	for i, b := range bodies {
		if stream, _ := b["stream"].(bool); !stream {
			t.Fatalf("attempt %d was not a stream request: %v", i+1, b)
		}
	}
}

func TestCallWithRequestStream_DoesNotRetry400(t *testing.T) {
	srv := &dualModeServer{failures: []int{http.StatusBadRequest, http.StatusBadRequest, http.StatusBadRequest}}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	c := newStreamTestClient(t, ts.URL)

	if _, err := c.CallWithRequestStream(newStreamTestRequest(t), nil); err == nil {
		t.Fatal("expected error for 400")
	}
	if n := len(srv.seen()); n != 1 {
		t.Fatalf("server saw %d requests, want 1", n)
	}
}

func TestCallWithRequestStream_DataPrefixWithoutSpace(t *testing.T) {
	ts := httptest.NewServer(&dualModeServer{noSpace: true})
	defer ts.Close()
	c := newStreamTestClient(t, ts.URL)

	got, err := c.CallWithRequestStream(newStreamTestRequest(t), nil)
	if err != nil {
		t.Fatalf("stream call: %v", err)
	}
	if got != streamTestText {
		t.Fatalf("text = %q, want %q", got, streamTestText)
	}
}

// A server that ignores "stream": true and answers with plain JSON is still parsed.
func TestCallWithRequestStream_JSONFallback(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`, streamTestText)
	}))
	defer ts.Close()
	c := newStreamTestClient(t, ts.URL)

	got, err := c.CallWithRequestStream(newStreamTestRequest(t), nil)
	if err != nil {
		t.Fatalf("stream call: %v", err)
	}
	if got != streamTestText {
		t.Fatalf("text = %q, want %q", got, streamTestText)
	}
	if c.LastUsage.TotalTokens != 3 {
		t.Fatalf("LastUsage = %+v, want total 3", c.LastUsage)
	}
}

type nonStreamHooks struct{ *Client }

func (nonStreamHooks) ChatCompletionStreamUnsupported() {}

func TestCallWithRequestPreferStream_UnsupportedClientUsesNonStream(t *testing.T) {
	srv := &dualModeServer{}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	c := newStreamTestClient(t, ts.URL)
	c.Hooks = nonStreamHooks{c}

	if SupportsChatCompletionStream(c) {
		t.Fatal("client with non-stream hooks must not report stream support")
	}
	req := newStreamTestRequest(t)
	req.Stream = true
	got, err := CallWithRequestPreferStream(c, req)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if got != streamTestText {
		t.Fatalf("text = %q, want %q", got, streamTestText)
	}
	bodies := srv.seen()
	if len(bodies) != 1 {
		t.Fatalf("server saw %d requests, want 1", len(bodies))
	}
	if _, ok := bodies[0]["stream"]; ok {
		t.Fatalf("non-stream fallback body must not carry stream: %v", bodies[0])
	}
}

func TestCallWithMessagesPreferStream_BaseClientStreams(t *testing.T) {
	srv := &dualModeServer{}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	c := newStreamTestClient(t, ts.URL)

	if !SupportsChatCompletionStream(c) {
		t.Fatal("base client must report stream support")
	}
	got, err := CallWithMessagesPreferStream(c, "sys", "user")
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if got != streamTestText {
		t.Fatalf("text = %q, want %q", got, streamTestText)
	}
	body := srv.seen()[0]
	if stream, _ := body["stream"].(bool); !stream {
		t.Fatalf(`body missing "stream": true: %v`, body)
	}
	// Same temperature / max tokens as CallWithMessages would send.
	if body["temperature"] != c.Cfg.Temperature {
		t.Fatalf("temperature = %v, want client default %v", body["temperature"], c.Cfg.Temperature)
	}
	if mt, _ := body["max_tokens"].(float64); int(mt) != c.MaxTokens {
		t.Fatalf("max_tokens = %v, want client default %d", body["max_tokens"], c.MaxTokens)
	}
}

type fakeNonEmbeddingClient struct{ AIClient }

func TestSupportsChatCompletionStream_NonEmbeddingClientIsFalse(t *testing.T) {
	if SupportsChatCompletionStream(fakeNonEmbeddingClient{}) {
		t.Fatal("clients that do not embed *Client must keep the non-stream path")
	}
}
