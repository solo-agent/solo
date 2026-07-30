package agent

import "log/slog"

// traeBlockedArgs are protocol-owned positional arguments. Users may pass
// other Trae CLI flags through custom_args, but cannot replace the ACP server
// command that Solo communicates with over stdio.
var traeBlockedArgs = map[string]blockedArgMode{
	"acp":   blockedStandalone,
	"serve": blockedStandalone,
}

// TraeBackend is Solo's dedicated Trae CLI adapter. Trae's native ACP server
// shares the transport and persistent-session implementation used by Hermes,
// while retaining its own provider identity, invocation, and diagnostics.
type TraeBackend struct {
	*HermesBackend
}

// NewTraeBackend creates a Trae CLI backend. If executablePath is empty it
// defaults to "traex".
func NewTraeBackend(executablePath string, logger *slog.Logger) *TraeBackend {
	if executablePath == "" {
		executablePath = "traex"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &TraeBackend{
		HermesBackend: &HermesBackend{
			executablePath: executablePath,
			logger:         logger,
			providerName:   "trae",
			baseArgs:       []string{"acp", "serve"},
			blockedArgs:    traeBlockedArgs,
		},
	}
}

var _ Backend = (*TraeBackend)(nil)
var _ PersistentBackend = (*TraeBackend)(nil)
