package middleware

import (
	"bufio"
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/solo-ai/solo/pkg/metrics"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

// Hijack implements http.Hijacker so gorilla/websocket can upgrade the
// connection to a WebSocket. Without this, WebSocket upgrades silently
// fail because the wrapped ResponseWriter doesn't implement Hijacker.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// Flush implements http.Flusher so SSE (Server-Sent Events) streaming works
// through the logging middleware. Without this, SSE handlers receive a
// non-Flusher writer and return 500.
func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

type contextKey string

const requestIDKey contextKey = "request_id"

// WithRequestID embeds a request ID into the context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// GetRequestID extracts the request ID from the context. Returns empty string
// if no request ID is present.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// Logging returns a middleware that logs every HTTP request using slog in JSON
// format. Each request receives a unique request_id (UUID v4). The log record
// includes method, path, status code, and duration.
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := uuid.New().String()
			w.Header().Set("X-Request-ID", requestID)

			rw := newResponseWriter(w)
			start := time.Now()

			metrics.Global.IncRequests()
			next.ServeHTTP(rw, r.WithContext(WithRequestID(r.Context(), requestID)))
			metrics.Global.DecRequests()
			metrics.Global.AddRequestDuration(time.Since(start).Nanoseconds())
			if rw.statusCode >= 500 {
				metrics.Global.IncErrors()
			}

			duration := time.Since(start)
			logger.Info("request",
				"request_id", requestID,
				"method", r.Method,
				"path", r.URL.Path,
				"query", r.URL.RawQuery,
				"status", rw.statusCode,
				"duration", duration.String(),
				"duration_ms", duration.Milliseconds(),
				"remote_addr", r.RemoteAddr,
				"user_agent", r.UserAgent(),
			)
		})
	}
}
