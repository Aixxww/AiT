package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Aixxww/AiT/manager"
	"github.com/Aixxww/AiT/store"

	"github.com/gin-gonic/gin"
)

// #75: Settings → AI models → "Add other model". Placeholder keys only.

func newCustomModelTestServer(t *testing.T) (*Server, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	st, err := store.New(t.TempDir() + "/data.db")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{store: st, traderManager: manager.NewTraderManager()}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", "u1"); c.Next() })
	r.GET("/models", s.handleGetModelConfigs)
	r.PUT("/models", s.handleUpdateModelConfigs)
	r.DELETE("/models/:id", s.handleDeleteModelConfig)
	r.GET("/supported-models", s.handleGetSupportedModels)
	return s, r
}

func doJSON(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func customModelsOf(t *testing.T, r *gin.Engine) []SafeModelConfig {
	t.Helper()
	w := doJSON(t, r, "GET", "/models", nil)
	var got []SafeModelConfig
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	var out []SafeModelConfig
	for _, m := range got {
		if m.Provider == store.ProviderCustom {
			out = append(out, m)
		}
	}
	return out
}

func TestCustomModelEntryAddRenameDelete(t *testing.T) {
	_, r := newCustomModelTestServer(t)

	w := doJSON(t, r, "GET", "/supported-models", nil)
	if !strings.Contains(w.Body.String(), `"provider":"custom"`) {
		t.Fatalf("supported models must offer the custom entry: %s", w.Body.String())
	}

	add := map[string]any{"models": map[string]any{"custom": map[string]any{
		"provider": "custom", "name": "WorkBuddy", "enabled": true, "api_key": "sk-placeholder",
		"custom_api_url": "https://gateway.example.com/v2", "custom_model_name": "deepseek-v4.1-flash",
	}}}
	if w := doJSON(t, r, "PUT", "/models", add); w.Code != http.StatusOK {
		t.Fatalf("add: %d %s", w.Code, w.Body.String())
	}
	customs := customModelsOf(t, r)
	if len(customs) != 1 || customs[0].Name != "WorkBuddy" || !customs[0].HasAPIKey || customs[0].CustomModelName != "deepseek-v4.1-flash" || !store.IsCustomModelID(customs[0].ID) {
		t.Fatalf("listed entries %+v", customs)
	}
	id := customs[0].ID

	rename := map[string]any{"models": map[string]any{id: map[string]any{
		"provider": "custom", "name": "WorkBuddy v2", "enabled": true,
		"custom_api_url": "https://gateway.example.com/v2", "custom_model_name": "deepseek-v4.1-flash",
	}}}
	if w := doJSON(t, r, "PUT", "/models", rename); w.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", w.Code, w.Body.String())
	}
	if customs = customModelsOf(t, r); len(customs) != 1 || customs[0].Name != "WorkBuddy v2" || !customs[0].HasAPIKey {
		t.Fatalf("after rename %+v", customs)
	}

	if w := doJSON(t, r, "DELETE", "/models/"+id, nil); w.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	if customs = customModelsOf(t, r); len(customs) != 0 {
		t.Fatalf("entry still listed: %+v", customs)
	}
}

func TestCustomModelEntryValidation(t *testing.T) {
	_, r := newCustomModelTestServer(t)
	for name, entry := range map[string]map[string]any{
		"missing url":   {"provider": "custom", "name": "X", "enabled": true, "api_key": "k", "custom_model_name": "m"},
		"missing model": {"provider": "custom", "name": "X", "enabled": true, "api_key": "k", "custom_api_url": "https://a.example.com"},
		"missing name":  {"provider": "custom", "enabled": true, "api_key": "k", "custom_api_url": "https://a.example.com", "custom_model_name": "m"},
	} {
		w := doJSON(t, r, "PUT", "/models", map[string]any{"models": map[string]any{"custom": entry}})
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d %s", name, w.Code, w.Body.String())
		}
	}
}

// Deleting is limited to custom entries; built-in entries stay (they are
// disabled instead), so strategies referencing built-ins are unaffected.
func TestBuiltInModelCannotBeDeleted(t *testing.T) {
	s, r := newCustomModelTestServer(t)
	if err := s.store.AIModel().Update("u1", "deepseek", true, "sk-placeholder", "", ""); err != nil {
		t.Fatal(err)
	}
	if w := doJSON(t, r, "DELETE", "/models/u1_deepseek", nil); w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d %s", w.Code, w.Body.String())
	}
	if _, err := s.store.AIModel().Get("u1", "u1_deepseek"); err != nil {
		t.Fatalf("built-in entry was removed: %v", err)
	}
	if w := doJSON(t, r, "DELETE", "/models/u1_custom_missing", nil); w.Code != http.StatusNotFound {
		t.Fatalf("unknown entry: want 404, got %d", w.Code)
	}
}
