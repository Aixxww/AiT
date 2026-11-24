package store

import (
	"errors"
	"strings"
	"testing"
)

// #75: custom (other, OpenAI-compatible) model entries. Placeholder keys only.

func newCustomModelTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := New(t.TempDir() + "/data.db")
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestUpsertCustomCreatesSelectableEntries(t *testing.T) {
	st := newCustomModelTestStore(t)
	m := st.AIModel()
	id1, err := m.UpsertCustom("u1", "custom", "WorkBuddy", true, "sk-placeholder-1", "https://gateway.example.com/v2", "deepseek-v4.1-flash")
	if err != nil {
		t.Fatal(err)
	}
	id2, err := m.UpsertCustom("u1", "", "Second gateway", true, "sk-placeholder-2", "https://other.example.com/v1", "model-b")
	if err != nil {
		t.Fatal(err)
	}
	if id1 == id2 || !IsCustomModelID(id1) || !strings.HasPrefix(id1, "u1_custom_") {
		t.Fatalf("ids %q %q", id1, id2)
	}
	got, err := m.Get("u1", id1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != ProviderCustom || got.Name != "WorkBuddy" || got.CustomAPIURL != "https://gateway.example.com/v2" ||
		got.CustomModelName != "deepseek-v4.1-flash" || string(got.APIKey) != "sk-placeholder-1" || !got.Enabled {
		t.Fatalf("stored entry %+v", got)
	}
	list, _ := m.List("u1")
	if len(list) != 2 {
		t.Fatalf("want 2 custom entries, got %d", len(list))
	}
	// Built-in entries are unaffected and coexist.
	if err := m.Update("u1", "deepseek", true, "sk-placeholder-ds", "", ""); err != nil {
		t.Fatal(err)
	}
	if ds, err := m.Get("u1", "u1_deepseek"); err != nil || ds.Provider != "deepseek" {
		t.Fatalf("built-in entry: %+v %v", ds, err)
	}
}

func TestUpsertCustomUpdatesRenamesAndKeepsKey(t *testing.T) {
	st := newCustomModelTestStore(t)
	m := st.AIModel()
	id, err := m.UpsertCustom("u1", "custom", "Old name", true, "sk-placeholder", "https://a.example.com/v1", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.UpsertCustom("u1", id, "New name", false, "", "https://b.example.com/v1", "m2"); err != nil {
		t.Fatal(err)
	}
	got, _ := m.Get("u1", id)
	if got.Name != "New name" || got.Enabled || got.CustomAPIURL != "https://b.example.com/v1" || got.CustomModelName != "m2" || string(got.APIKey) != "sk-placeholder" {
		t.Fatalf("after update %+v", got)
	}
}

func TestUpsertCustomValidation(t *testing.T) {
	st := newCustomModelTestStore(t)
	m := st.AIModel()
	cases := []struct{ name, key, url, model string }{
		{"", "k", "https://a.example.com", "m"},
		{"n", "k", "", "m"},
		{"n", "k", "https://a.example.com", ""},
		{"n", "", "https://a.example.com", "m"},
	}
	for _, c := range cases {
		if _, err := m.UpsertCustom("u1", "custom", c.name, true, c.key, c.url, c.model); !errors.Is(err, ErrCustomModelInvalid) {
			t.Errorf("%+v: want ErrCustomModelInvalid, got %v", c, err)
		}
	}
	if err := m.Update("u1", "deepseek", true, "k", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := m.UpsertCustom("u1", "u1_deepseek", "n", true, "", "https://a.example.com", "m"); !errors.Is(err, ErrCustomModelInvalid) {
		t.Fatalf("a built-in entry must not be rewritten as custom, got %v", err)
	}
	if _, err := m.UpsertCustom("u2", "u1_custom_deadbeef", "n", true, "", "https://a.example.com", "m"); !errors.Is(err, ErrCustomModelInvalid) {
		t.Fatalf("another user's id must not resolve, got %v", err)
	}
}

func TestDeletedCustomEntryFailsLookup(t *testing.T) {
	st := newCustomModelTestStore(t)
	m := st.AIModel()
	id, err := m.UpsertCustom("u1", "custom", "Gateway", true, "sk-placeholder", "https://a.example.com/v1", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Delete("u1", id); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get("u1", id); err == nil {
		t.Fatal("a deleted custom entry must not resolve (no silent fallback)")
	}
}
