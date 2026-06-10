// Package workspace defines shared types for workspace file proxying
// between the server, handler, and service packages.
package workspace

import "context"

// Daemon holds the connection info for proxying workspace requests to a daemon.
type Daemon struct {
	Host string
	Port int
}

// Proxy is the interface for proxying workspace requests to daemons.
type Proxy interface {
	FindDaemonForAgent(ctx context.Context, agentID string) (*Daemon, bool)
	ProxyWorkspaceList(ctx context.Context, daemon *Daemon, agentID, path string) ([]byte, error)
	ProxyWorkspaceRead(ctx context.Context, daemon *Daemon, agentID, path string) ([]byte, error)
	ProxySkillList(ctx context.Context, daemon *Daemon, agentID string) ([]byte, error)
}
