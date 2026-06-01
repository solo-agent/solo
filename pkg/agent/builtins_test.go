package agent

import (
	"strings"
	"testing"
)

// ── All built-in types are creatable via the global registry ──────────────────

func TestBuiltins_AllTypesCreatable(t *testing.T) {
	types := []string{
		"claude", "local", "codex", "opencode", "cursor",
		"gemini", "kimi", "kiro", "copilot", "openclaw", "hermes", "pi",
	}

	reg := GlobalRegistry()
	for _, typ := range types {
		b, err := reg.Create(typ, BackendConfig{ProviderType: typ})
		if err != nil {
			t.Errorf("Create(%q): unexpected error: %v", typ, err)
			continue
		}
		if b == nil {
			t.Errorf("Create(%q): nil backend", typ)
			continue
		}
		// "local" is an alias for claude — its Name() returns "claude".
		wantName := typ
		if typ == "local" {
			wantName = "claude"
		}
		if got := b.Name(); got != wantName {
			t.Errorf("Create(%q): Name() = %q, want %q", typ, got, wantName)
		}
	}
}

// ── ListMeta returns at least 12 entries (11 types + local alias) ─────────────

func TestBuiltins_ListMeta(t *testing.T) {
	expected := []string{
		"claude", "local", "codex", "opencode", "cursor",
		"gemini", "kimi", "kiro", "copilot", "openclaw", "hermes", "pi",
	}

	metas := GlobalRegistry().ListMeta()
	if len(metas) < 11 {
		t.Errorf("ListMeta: expected >= 11 entries, got %d", len(metas))
	}

	typeSet := make(map[string]bool, len(metas))
	for _, m := range metas {
		if m.Type == "" {
			t.Error("ListMeta: entry with empty Type")
		}
		if m.DisplayName == "" {
			t.Errorf("ListMeta: Type %q has empty DisplayName", m.Type)
		}
		if len(m.Models) == 0 {
			t.Errorf("ListMeta: Type %q has no models", m.Type)
		}
		typeSet[m.Type] = true
	}

	for _, want := range expected {
		if !typeSet[want] {
			t.Errorf("ListMeta: missing type %q", want)
		}
	}
}

// ── Unknown type produces an error ────────────────────────────────────────────

func TestBuiltins_UnknownTypeError(t *testing.T) {
	reg := GlobalRegistry()
	b, err := reg.Create("nonexistent-backend-type", BackendConfig{})
	if err == nil {
		t.Fatal("expected error for unknown backend type")
	}
	if b != nil {
		t.Errorf("expected nil backend, got %v", b)
	}
	if !strings.Contains(err.Error(), "unknown backend type") {
		t.Errorf("expected 'unknown backend type' in error, got: %v", err)
	}
}

// ── PersistentBackend detection ───────────────────────────────────────────────

func TestBuiltins_PersistentBackend(t *testing.T) {
	reg := GlobalRegistry()

	// claude, local, codex, and opencode should implement PersistentBackend (v1.4).
	for _, typ := range []string{"claude", "local", "codex", "opencode"} {
		b, err := reg.Create(typ, BackendConfig{ProviderType: typ})
		if err != nil {
			t.Fatalf("Create(%q): %v", typ, err)
		}
		if _, ok := b.(PersistentBackend); !ok {
			t.Errorf("%q should implement PersistentBackend", typ)
		}
	}

	// All other built-in types should NOT implement PersistentBackend.
	for _, typ := range []string{"cursor", "gemini", "kimi", "kiro", "copilot", "openclaw", "hermes", "pi"} {
		b, err := reg.Create(typ, BackendConfig{ProviderType: typ})
		if err != nil {
			t.Fatalf("Create(%q): %v", typ, err)
		}
		if _, ok := b.(PersistentBackend); ok {
			t.Errorf("%q should NOT implement PersistentBackend", typ)
		}
	}
}

// ── PersistentBackend detection via NewPersistentBackend ──────────────────────

func TestBuiltins_NewPersistentBackend(t *testing.T) {
	// Persistent types should succeed (v1.4: codex + opencode added).
	for _, typ := range []string{"claude", "local", "codex", "opencode"} {
		pb, err := NewPersistentBackend(typ)
		if err != nil {
			t.Fatalf("NewPersistentBackend(%q): unexpected error: %v", typ, err)
		}
		if pb == nil {
			t.Fatalf("NewPersistentBackend(%q): nil backend", typ)
		}
	}

	// Non-persistent types should return an error.
	for _, typ := range []string{"cursor", "gemini", "kimi", "kiro", "copilot", "openclaw", "hermes", "pi"} {
		pb, err := NewPersistentBackend(typ)
		if err == nil {
			t.Errorf("NewPersistentBackend(%q): expected error, got nil", typ)
		}
		if pb != nil {
			t.Errorf("NewPersistentBackend(%q): expected nil backend on error", typ)
		}
		if err != nil && !strings.Contains(err.Error(), "persistent backend not supported") {
			t.Errorf("NewPersistentBackend(%q): error missing 'persistent backend not supported': %v", typ, err)
		}
	}

	// Unknown type should also error.
	pb, err := NewPersistentBackend("unknown")
	if err == nil {
		t.Error("NewPersistentBackend(unknown): expected error")
	}
	if pb != nil {
		t.Error("NewPersistentBackend(unknown): expected nil backend")
	}
}

// ── Factory receives BackendConfig correctly ──────────────────────────────────

func TestBuiltins_FactoryReceivesConfig(t *testing.T) {
	reg := GlobalRegistry()

	// ExecPath should flow through to the constructed backend.
	cfg := BackendConfig{
		ProviderType: "claude",
		ExecPath:     "/custom/path/claude",
	}
	b, err := reg.Create("claude", cfg)
	if err != nil {
		t.Fatalf("Create(claude) with ExecPath: %v", err)
	}
	cb, ok := b.(*ClaudeBackend)
	if !ok {
		t.Fatal("expected *ClaudeBackend")
	}
	if cb.executablePath != "/custom/path/claude" {
		t.Errorf("expected ExecPath /custom/path/claude, got %q", cb.executablePath)
	}

	// Empty ExecPath falls back to env var.
	t.Setenv("CODEX_BIN", "/env/path/codex")
	b2, err := reg.Create("codex", BackendConfig{ProviderType: "codex"})
	if err != nil {
		t.Fatalf("Create(codex) with env ExecPath: %v", err)
	}
	cb2, ok := b2.(*CodexBackend)
	if !ok {
		t.Fatal("expected *CodexBackend")
	}
	if cb2.executablePath != "/env/path/codex" {
		t.Errorf("expected ExecPath /env/path/codex from env, got %q", cb2.executablePath)
	}

	// ExecPath in config takes priority over env var.
	b3, err := reg.Create("codex", BackendConfig{
		ProviderType: "codex",
		ExecPath:     "/explicit/codex",
	})
	if err != nil {
		t.Fatalf("Create(codex) with explicit ExecPath: %v", err)
	}
	cb3, ok := b3.(*CodexBackend)
	if !ok {
		t.Fatal("expected *CodexBackend")
	}
	if cb3.executablePath != "/explicit/codex" {
		t.Errorf("expected ExecPath /explicit/codex, got %q", cb3.executablePath)
	}
}

// ── Model lists are non-empty for well-known types ────────────────────────────

func TestBuiltins_ModelLists(t *testing.T) {
	metas := GlobalRegistry().ListMeta()
	metaByType := make(map[string]AdapterMeta, len(metas))
	for _, m := range metas {
		metaByType[m.Type] = m
	}

	// Claude has 3 known models.
	if m, ok := metaByType["claude"]; ok {
		if len(m.Models) != 3 {
			t.Errorf("claude: expected 3 models, got %d", len(m.Models))
		}
		// Check default model.
		if m.DefaultModel != "claude-sonnet-4-6" {
			t.Errorf("claude: expected default model claude-sonnet-4-6, got %q", m.DefaultModel)
		}
		hasDefault := false
		for _, mi := range m.Models {
			if mi.Default {
				hasDefault = true
				if mi.ID != m.DefaultModel {
					t.Errorf("claude: default model marker on %q but DefaultModel is %q", mi.ID, m.DefaultModel)
				}
			}
		}
		if !hasDefault {
			t.Error("claude: no model marked as Default")
		}
	}

	// Codex has 2 known models.
	if m, ok := metaByType["codex"]; ok {
		if len(m.Models) != 2 {
			t.Errorf("codex: expected 2 models, got %d", len(m.Models))
		}
		if m.DefaultModel != "gpt-5.1-codex" {
			t.Errorf("codex: expected default model gpt-5.1-codex, got %q", m.DefaultModel)
		}
	}

	// OpenCode has 2 known models.
	if m, ok := metaByType["opencode"]; ok {
		if len(m.Models) != 2 {
			t.Errorf("opencode: expected 2 models, got %d", len(m.Models))
		}
		if m.DefaultModel != "gpt-5" {
			t.Errorf("opencode: expected default model gpt-5, got %q", m.DefaultModel)
		}
	}
}

// ── local is a true alias of claude ──────────────────────────────────────────

func TestBuiltins_LocalIsaClaudeAlias(t *testing.T) {
	reg := GlobalRegistry()

	bLocal, err := reg.Create("local", BackendConfig{ProviderType: "local"})
	if err != nil {
		t.Fatalf("Create(local): %v", err)
	}
	bClaude, err := reg.Create("claude", BackendConfig{ProviderType: "claude"})
	if err != nil {
		t.Fatalf("Create(claude): %v", err)
	}

	// Both should be *ClaudeBackend instances.
	if _, ok := bLocal.(*ClaudeBackend); !ok {
		t.Error("local backend is not *ClaudeBackend")
	}
	if _, ok := bClaude.(*ClaudeBackend); !ok {
		t.Error("claude backend is not *ClaudeBackend")
	}

	// Name() from local should be "claude" (the underlying name).
	if bLocal.Name() != "claude" {
		t.Errorf("local Name() = %q, want 'claude'", bLocal.Name())
	}
}
