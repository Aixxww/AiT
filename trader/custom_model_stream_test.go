package trader

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Aixxww/AiT/mcp"
	"github.com/Aixxww/AiT/store"
)

// #75: a custom (other) model entry saved in Settings resolves to the
// generic OpenAI-compatible client and its decision call streams, with the
// entry's base URL and model name. Placeholder key only.
func TestCustomModelEntryDecisionCallStreams(t *testing.T) {
	var got map[string]any
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		if stream, _ := got["stream"].(bool); !stream {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"code":11101,"msg":"Non-stream chat request is currently not supported"}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"[{\\\"symbol\\\":\\\"ALL\\\",\\\"action\\\":\\\"wait\\\"}]\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	st, err := store.New(t.TempDir() + "/data.db")
	if err != nil {
		t.Fatal(err)
	}
	id, err := st.AIModel().UpsertCustom("u1", "custom", "Gateway", true, "sk-placeholder", srv.URL+"/v2", "deepseek-v4.1-flash")
	if err != nil {
		t.Fatal(err)
	}
	model, err := st.AIModel().Get("u1", id)
	if err != nil {
		t.Fatal(err)
	}

	// The same construction NewAutoTrader uses for config.AIModel = provider.
	client := newTraderAIClient(model.Provider, string(model.APIKey), model.CustomAPIURL, model.CustomModelName)
	base := client.(mcp.ClientEmbedder).BaseClient()
	if base.BaseURL != srv.URL+"/v2" || base.Model != "deepseek-v4.1-flash" {
		t.Fatalf("client base URL %q model %q", base.BaseURL, base.Model)
	}
	if !mcp.SupportsChatCompletionStream(client) {
		t.Fatal("custom entries must take the streaming decision path")
	}
	base.HTTPClient = &http.Client{} // loopback stub (the default client blocks private IPs)

	// Decision call as kernel.callStrategyDecisionAI issues it (default path).
	text, err := mcp.CallWithMessagesPreferStream(client, "system", "user")
	if err != nil {
		t.Fatalf("decision call: %v", err)
	}
	if stream, _ := got["stream"].(bool); !stream || got["model"] != "deepseek-v4.1-flash" || path != "/v2/chat/completions" {
		t.Fatalf("request stream=%v model=%v path=%s", got["stream"], got["model"], path)
	}
	if text != `[{"symbol":"ALL","action":"wait"}]` {
		t.Fatalf("decision text %q", text)
	}
}
