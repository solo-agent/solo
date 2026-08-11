package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

// BackendFactory creates a Backend from configuration. Each adapter type
// registers its own factory via BackendRegistry.Register.
type BackendFactory func(cfg BackendConfig) (Backend, error)

// BackendConfig carries the parameters needed to construct a Backend.
type BackendConfig struct {
	ProviderType string
	APIKey       string
	ExecPath     string
	Logger       *slog.Logger
}

// CapabilityStatus describes whether a backend capability is available
// through Solo's integration. The zero value is normalized to unknown so
// adapters compiled before the capability table remain safe and visible.
type CapabilityStatus string

const (
	CapabilitySupported   CapabilityStatus = "supported"
	CapabilityUnsupported CapabilityStatus = "unsupported"
	CapabilityUnknown     CapabilityStatus = "unknown"
)

// BackendCapabilities is the runtime contract for one backend adapter.
// It describes Solo's integration rather than every feature the underlying
// CLI may expose on its own.
type BackendCapabilities struct {
	PersistentConversation CapabilityStatus `json:"persistent_conversation"`
	ResumeConversation     CapabilityStatus `json:"resume_conversation"`
	BusyMessageDelivery    CapabilityStatus `json:"busy_message_delivery"`
	SafeStop               CapabilityStatus `json:"safe_stop"`
	InteractiveInput       CapabilityStatus `json:"interactive_input"`
	TokenUsage             CapabilityStatus `json:"token_usage"`
}

func normalizeCapabilityStatus(status CapabilityStatus) CapabilityStatus {
	switch status {
	case CapabilitySupported, CapabilityUnsupported, CapabilityUnknown:
		return status
	default:
		return CapabilityUnknown
	}
}

func (c BackendCapabilities) normalized() BackendCapabilities {
	return BackendCapabilities{
		PersistentConversation: normalizeCapabilityStatus(c.PersistentConversation),
		ResumeConversation:     normalizeCapabilityStatus(c.ResumeConversation),
		BusyMessageDelivery:    normalizeCapabilityStatus(c.BusyMessageDelivery),
		SafeStop:               normalizeCapabilityStatus(c.SafeStop),
		InteractiveInput:       normalizeCapabilityStatus(c.InteractiveInput),
		TokenUsage:             normalizeCapabilityStatus(c.TokenUsage),
	}
}

// AdapterMeta describes a registered backend adapter for discovery and UI.
type AdapterMeta struct {
	Type           string              `json:"type"`            // "claude", "codex", "opencode"...
	DisplayName    string              `json:"display_name"`    // "Claude Code", "Codex CLI"
	RequiresBinary string              `json:"requires_binary"` // CLI binary name, e.g. "claude", "codex", "opencode"
	DetectCommand  string              `json:"-"`               // e.g. "--version"
	Protocols      []string            `json:"protocols"`       // "stream-json", "json-rpc", "acp", "jsonl"
	Capabilities   BackendCapabilities `json:"capabilities"`
}

// Meta returns the registered metadata for typ.
func (r *BackendRegistry) Meta(typ string) (AdapterMeta, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.backends[typ]
	return entry.Meta, ok
}

// BackendRegistry is a thread-safe registry of backend adapters.
// Adapters register themselves via Register, and callers create
// Backend instances by type name via Create.
type BackendRegistry struct {
	mu       sync.RWMutex
	backends map[string]registryEntry
}

type registryEntry struct {
	Factory BackendFactory
	Meta    AdapterMeta
}

// globalRegistry is the package-level singleton.
var globalRegistry = &BackendRegistry{
	backends: make(map[string]registryEntry),
}

// GlobalRegistry returns the package-level BackendRegistry singleton.
func GlobalRegistry() *BackendRegistry { return globalRegistry }

// Register adds a backend adapter to the registry. It overwrites any
// existing entry with the same meta.Type. Safe for concurrent use.
func (r *BackendRegistry) Register(meta AdapterMeta, factory BackendFactory) {
	meta.Capabilities = meta.Capabilities.normalized()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backends[meta.Type] = registryEntry{Factory: factory, Meta: meta}
}

// Create constructs a Backend of the given type using the supplied config.
// Returns an error if typ has not been registered.
func (r *BackendRegistry) Create(typ string, cfg BackendConfig) (Backend, error) {
	r.mu.RLock()
	entry, ok := r.backends[typ]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown backend type: %q", typ)
	}
	return entry.Factory(cfg)
}

// BackendStatus reports the availability of a single registered backend on the local machine.
type BackendStatus struct {
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	Binary      string `json:"binary"`
	Available   bool   `json:"available"`
	Version     string `json:"version,omitempty"`
	Error       string `json:"error,omitempty"`
}

// Detect checks every registered backend for local availability by looking up
// its binary with exec.LookPath. If found and a DetectCommand is configured, it
// also captures the version output. Each check is capped at 5 seconds.
func (r *BackendRegistry) Detect() []BackendStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make([]BackendStatus, 0, len(r.backends))
	for _, entry := range r.backends {
		status := BackendStatus{
			Type:        entry.Meta.Type,
			DisplayName: entry.Meta.DisplayName,
			Binary:      entry.Meta.RequiresBinary,
		}
		path, err := exec.LookPath(entry.Meta.RequiresBinary)
		if err != nil {
			status.Available = false
			status.Error = err.Error()
		} else {
			status.Available = true
			if entry.Meta.DetectCommand != "" {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				out, err := exec.CommandContext(ctx, path, entry.Meta.DetectCommand).Output()
				cancel()
				if err == nil {
					v := strings.TrimSpace(string(out))
					if idx := strings.IndexByte(v, '\n'); idx >= 0 {
						v = v[:idx]
					}
					status.Version = strings.TrimSpace(v)
				}
			}
		}
		results = append(results, status)
	}
	return results
}

// ListMeta returns a snapshot of AdapterMeta for every registered backend.
// The order is non-deterministic.
func (r *BackendRegistry) ListMeta() []AdapterMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()
	metas := make([]AdapterMeta, 0, len(r.backends))
	for _, e := range r.backends {
		metas = append(metas, e.Meta)
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].Type < metas[j].Type })
	return metas
}
