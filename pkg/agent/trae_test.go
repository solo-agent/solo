package agent

import (
	"reflect"
	"testing"
)

func TestTraeCommandArgs(t *testing.T) {
	b := NewTraeBackend("", nil)
	opts := &ExecuteOptions{
		ExtraArgs:  []string{"--enable", "memory", "acp"},
		CustomArgs: []string{"serve", "--profile", "solo"},
	}

	got := b.commandArgs(opts)
	want := []string{"acp", "serve", "--enable", "memory", "--profile", "solo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commandArgs() = %#v, want %#v", got, want)
	}
}

func TestTraeFactoryUsesEnvironmentOverride(t *testing.T) {
	t.Setenv("TRAEX_BIN", "/custom/bin/traex")

	backend, err := GlobalRegistry().Create("trae", BackendConfig{ProviderType: "trae"})
	if err != nil {
		t.Fatalf("Create(\"trae\") failed: %v", err)
	}
	trae, ok := backend.(*TraeBackend)
	if !ok {
		t.Fatalf("backend type = %T, want *TraeBackend", backend)
	}
	if trae.executablePath != "/custom/bin/traex" {
		t.Fatalf("executablePath = %q, want /custom/bin/traex", trae.executablePath)
	}
}

func TestTraeMetadata(t *testing.T) {
	var found *AdapterMeta
	for _, meta := range GlobalRegistry().ListMeta() {
		if meta.Type == "trae" {
			copy := meta
			found = &copy
			break
		}
	}
	if found == nil {
		t.Fatal("trae metadata not registered")
	}
	if found.DisplayName != "Trae CLI" || found.RequiresBinary != "traex" {
		t.Fatalf("unexpected metadata: %+v", *found)
	}
	if !reflect.DeepEqual(found.Protocols, []string{"acp"}) {
		t.Fatalf("protocols = %#v, want [acp]", found.Protocols)
	}
}
