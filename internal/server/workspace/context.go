package workspace

import (
	"context"
	"net/http"
)

const (
	PublicID = "00000000-0000-0000-0000-000000000001"
	Header   = "X-Workspace-ID"
)

type contextKey struct{}

type Scope struct {
	ID   string
	Role string
}

func WithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, contextKey{}, scope)
}

func FromContext(ctx context.Context) (Scope, bool) {
	scope, ok := ctx.Value(contextKey{}).(Scope)
	return scope, ok && scope.ID != ""
}

// ContextID returns the active Workspace for service-layer collection queries.
// Background jobs and legacy callers without request scope retain the historical
// public-Workspace behavior.
func ContextID(ctx context.Context) string {
	if scope, ok := FromContext(ctx); ok {
		return scope.ID
	}
	return PublicID
}

// FilterID returns the active Workspace when the caller is handling a scoped
// request. Service callers without request scope keep the legacy global view.
func FilterID(ctx context.Context) string {
	if scope, ok := FromContext(ctx); ok {
		return scope.ID
	}
	return ""
}

func ID(r *http.Request) string {
	if scope, ok := FromContext(r.Context()); ok {
		return scope.ID
	}
	return PublicID
}

func Role(r *http.Request) string {
	if scope, ok := FromContext(r.Context()); ok {
		return scope.Role
	}
	return ""
}
