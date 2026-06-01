package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// ── Test-only Backend implementation ────────────────────────────────────────

// testBackend is a minimal Backend implementation used by registry tests.
// Each instance records the config it was created with so tests can verify
// that Create forwards BackendConfig correctly.
type testBackend struct {
	name string
	cfg  BackendConfig
}

func (b *testBackend) Execute(_ context.Context, _ *ExecuteRequest, _ *ExecuteOptions) (*Session, error) {
	return nil, nil
}

func (b *testBackend) Name() string { return b.name }

// newTestBackendFactory returns a BackendFactory that records the config
// into a channel for verification.
func newTestBackendFactory(name string, recorded *[]BackendConfig) BackendFactory {
	return func(cfg BackendConfig) (Backend, error) {
		if recorded != nil {
			*recorded = append(*recorded, cfg)
		}
		return &testBackend{name: name, cfg: cfg}, nil
	}
}

// ── Registration and Create correctness ─────────────────────────────────────

func TestRegistry_RegisterAndCreate(t *testing.T) {
	reg := &BackendRegistry{backends: make(map[string]registryEntry)}

	var recordedA, recordedB, recordedC []BackendConfig

	reg.Register(AdapterMeta{
		Type:        "alpha",
		DisplayName: "Alpha Test Backend",
		Protocols:   []string{"stream-json"},
	}, newTestBackendFactory("alpha", &recordedA))

	reg.Register(AdapterMeta{
		Type:        "beta",
		DisplayName: "Beta Test Backend",
		Protocols:   []string{"json-rpc"},
	}, newTestBackendFactory("beta", &recordedB))

	reg.Register(AdapterMeta{
		Type:        "gamma",
		DisplayName: "Gamma Test Backend",
		Protocols:   []string{"acp"},
	}, newTestBackendFactory("gamma", &recordedC))

	cfgA := BackendConfig{ProviderType: "alpha", APIKey: "key-a"}
	bA, err := reg.Create("alpha", cfgA)
	if err != nil {
		t.Fatalf("Create(alpha): %v", err)
	}
	if bA.Name() != "alpha" {
		t.Errorf("expected name 'alpha', got %q", bA.Name())
	}
	if len(recordedA) != 1 || recordedA[0].APIKey != "key-a" {
		t.Errorf("factory not called with correct config: %+v", recordedA)
	}

	cfgB := BackendConfig{ProviderType: "beta", ExecPath: "/usr/bin/beta"}
	bB, err := reg.Create("beta", cfgB)
	if err != nil {
		t.Fatalf("Create(beta): %v", err)
	}
	if bB.Name() != "beta" {
		t.Errorf("expected name 'beta', got %q", bB.Name())
	}
	if len(recordedB) != 1 || recordedB[0].ExecPath != "/usr/bin/beta" {
		t.Errorf("factory not called with correct config: %+v", recordedB)
	}

	cfgC := BackendConfig{ProviderType: "gamma"}
	bC, err := reg.Create("gamma", cfgC)
	if err != nil {
		t.Fatalf("Create(gamma): %v", err)
	}
	if bC.Name() != "gamma" {
		t.Errorf("expected name 'gamma', got %q", bC.Name())
	}
	if len(recordedC) != 1 {
		t.Errorf("factory not called: %+v", recordedC)
	}
}

// ── Unknown type returns error ──────────────────────────────────────────────

func TestRegistry_CreateUnknownType(t *testing.T) {
	reg := &BackendRegistry{backends: make(map[string]registryEntry)}

	b, err := reg.Create("nonexistent", BackendConfig{})
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
	if b != nil {
		t.Errorf("expected nil backend, got %v", b)
	}
	if !strings.Contains(err.Error(), "unknown backend type") {
		t.Errorf("expected 'unknown backend type' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"nonexistent"`) {
		t.Errorf("expected quoted type name in error, got: %v", err)
	}
}

// ── ListMeta returns all registered entries ─────────────────────────────────

func TestRegistry_ListMeta(t *testing.T) {
	reg := &BackendRegistry{backends: make(map[string]registryEntry)}

	// Empty registry should return empty slice, not nil.
	empty := reg.ListMeta()
	if len(empty) != 0 {
		t.Errorf("expected empty list, got %d entries", len(empty))
	}

	reg.Register(AdapterMeta{Type: "a", DisplayName: "A"}, nil)
	reg.Register(AdapterMeta{Type: "b", DisplayName: "B"}, nil)
	reg.Register(AdapterMeta{Type: "c", DisplayName: "C"}, nil)

	metas := reg.ListMeta()
	if len(metas) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(metas))
	}

	seen := make(map[string]bool)
	for _, m := range metas {
		seen[m.Type] = true
	}
	for _, want := range []string{"a", "b", "c"} {
		if !seen[want] {
			t.Errorf("missing entry for type %q", want)
		}
	}
}

// ── Register overwrites existing entry ──────────────────────────────────────

func TestRegistry_RegisterOverwrite(t *testing.T) {
	reg := &BackendRegistry{backends: make(map[string]registryEntry)}

	var configs []BackendConfig

	reg.Register(AdapterMeta{Type: "x", DisplayName: "First"}, newTestBackendFactory("first", &configs))
	reg.Register(AdapterMeta{Type: "x", DisplayName: "Second"}, newTestBackendFactory("second", &configs))

	b, err := reg.Create("x", BackendConfig{ProviderType: "x"})
	if err != nil {
		t.Fatalf("Create(x): %v", err)
	}
	if b.Name() != "second" {
		t.Errorf("expected 'second' factory to be used after overwrite, got %q", b.Name())
	}

	metas := reg.ListMeta()
	if len(metas) != 1 {
		t.Fatalf("expected 1 entry after overwrite, got %d", len(metas))
	}
	if metas[0].DisplayName != "Second" {
		t.Errorf("expected DisplayName 'Second', got %q", metas[0].DisplayName)
	}
}

// ── Concurrent safety ───────────────────────────────────────────────────────

func TestRegistry_ConcurrentRegisterCreate(t *testing.T) {
	reg := &BackendRegistry{backends: make(map[string]registryEntry)}

	for i := 0; i < 10; i++ {
		typ := fmt.Sprintf("type-%d", i)
		reg.Register(AdapterMeta{Type: typ, DisplayName: typ},
			func(cfg BackendConfig) (Backend, error) {
				return &testBackend{name: typ}, nil
			},
		)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 200)

	// Concurrently create backends.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			typ := fmt.Sprintf("type-%d", idx%10)
			b, err := reg.Create(typ, BackendConfig{ProviderType: typ})
			if err != nil {
				errs <- err
				return
			}
			if b == nil {
				errs <- fmt.Errorf("nil backend for %q", typ)
			}
		}(i)
	}

	// Concurrently register new types while creating.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			typ := fmt.Sprintf("concurrent-%d", idx)
			reg.Register(AdapterMeta{Type: typ},
				func(cfg BackendConfig) (Backend, error) {
					return &testBackend{name: typ}, nil
				},
			)
		}(i)
	}

	wg.Wait()
	close(errs)

	for e := range errs {
		t.Errorf("concurrent error: %v", e)
	}
}

// ── GlobalRegistry singleton ────────────────────────────────────────────────

func TestGlobalRegistry_SameInstance(t *testing.T) {
	r1 := GlobalRegistry()
	r2 := GlobalRegistry()
	if r1 != r2 {
		t.Error("GlobalRegistry() returned different instances")
	}
}

func TestGlobalRegistry_IsolateFromLocal(t *testing.T) {
	// Local registries should be independent of the global one.
	local := &BackendRegistry{backends: make(map[string]registryEntry)}
	local.Register(AdapterMeta{Type: "local-only"}, nil)

	// Global should not see the local registration.
	b, err := GlobalRegistry().Create("local-only", BackendConfig{})
	if err == nil {
		t.Fatal("expected error: global registry should not see local-only registration")
	}
	if b != nil {
		t.Error("expected nil backend")
	}
}

// ── Factory error propagation ───────────────────────────────────────────────

func TestRegistry_CreatePropagatesFactoryError(t *testing.T) {
	reg := &BackendRegistry{backends: make(map[string]registryEntry)}

	reg.Register(AdapterMeta{Type: "failing"}, func(cfg BackendConfig) (Backend, error) {
		return nil, fmt.Errorf("simulated factory failure")
	})

	b, err := reg.Create("failing", BackendConfig{})
	if err == nil {
		t.Fatal("expected error from factory")
	}
	if err.Error() != "simulated factory failure" {
		t.Errorf("expected 'simulated factory failure', got %q", err.Error())
	}
	if b != nil {
		t.Error("expected nil backend on factory error")
	}
}
