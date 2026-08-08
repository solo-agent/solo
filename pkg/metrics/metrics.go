// Package metrics provides lightweight application metrics for Prometheus scraping.
// It uses atomic counters for thread safety without external dependencies.
package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

// Global instance for convenient access from middleware and handlers.
var Global = &Metrics{startTime: time.Now()}

// Metrics holds atomic counters for key application observability signals.
type Metrics struct {
	requestsTotal    atomic.Int64
	activeRequests   atomic.Int64
	errorsTotal      atomic.Int64
	wsConnections    atomic.Int64
	daemonControls   atomic.Int64
	daemonConnects   atomic.Int64
	remoteRunsQueued atomic.Int64
	remoteRunAccepts atomic.Int64
	remoteEventDupes atomic.Int64
	remoteRPCTimeout atomic.Int64
	requestDurationN atomic.Int64 // total nanoseconds of all completed requests
	startTime        time.Time
}

// IncRequests increments the total and active request counters.
func (m *Metrics) IncRequests() {
	m.requestsTotal.Add(1)
	m.activeRequests.Add(1)
}

// DecRequests decrements the active request counter (call in defer after serve).
func (m *Metrics) DecRequests() {
	m.activeRequests.Add(-1)
}

// AddRequestDuration records the duration of a completed request in nanoseconds.
func (m *Metrics) AddRequestDuration(d int64) {
	m.requestDurationN.Add(d)
}

// IncErrors increments the error counter.
func (m *Metrics) IncErrors() {
	m.errorsTotal.Add(1)
}

// SetWSConnections sets the current WebSocket connection count.
func (m *Metrics) SetWSConnections(n int64) {
	m.wsConnections.Store(n)
}

func (m *Metrics) SetDaemonControls(n int64) { m.daemonControls.Store(n) }
func (m *Metrics) IncDaemonConnects()        { m.daemonConnects.Add(1) }
func (m *Metrics) IncRemoteRunsQueued()      { m.remoteRunsQueued.Add(1) }
func (m *Metrics) IncRemoteRunAccepts()      { m.remoteRunAccepts.Add(1) }
func (m *Metrics) IncRemoteEventDupes()      { m.remoteEventDupes.Add(1) }
func (m *Metrics) IncRemoteRPCTimeout()      { m.remoteRPCTimeout.Add(1) }

// Handler returns an HTTP handler that serves metrics in Prometheus text format.
// It can be registered as GET /metrics with no authentication required.
func (m *Metrics) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")

		uptime := time.Since(m.startTime).Truncate(time.Second).Seconds()
		total := m.requestsTotal.Load()
		errors := m.errorsTotal.Load()

		fmt.Fprintf(w, "# HELP solo_build_info Build metadata\n")
		fmt.Fprintf(w, "# TYPE solo_build_info gauge\n")
		fmt.Fprintf(w, "solo_build_info{version=\"0.1.0\",revision=\"unknown\"} 1\n")

		fmt.Fprintf(w, "# HELP solo_uptime_seconds Server uptime in seconds\n")
		fmt.Fprintf(w, "# TYPE solo_uptime_seconds gauge\n")
		fmt.Fprintf(w, "solo_uptime_seconds %0.0f\n", uptime)

		fmt.Fprintf(w, "# HELP solo_requests_total Total number of HTTP requests received\n")
		fmt.Fprintf(w, "# TYPE solo_requests_total counter\n")
		fmt.Fprintf(w, "solo_requests_total %d\n", total)

		fmt.Fprintf(w, "# HELP solo_requests_active Current number of active HTTP requests\n")
		fmt.Fprintf(w, "# TYPE solo_requests_active gauge\n")
		fmt.Fprintf(w, "solo_requests_active %d\n", m.activeRequests.Load())

		fmt.Fprintf(w, "# HELP solo_errors_total Total number of internal server errors\n")
		fmt.Fprintf(w, "# TYPE solo_errors_total counter\n")
		fmt.Fprintf(w, "solo_errors_total %d\n", errors)

		fmt.Fprintf(w, "# HELP solo_ws_connections Current number of active WebSocket connections\n")
		fmt.Fprintf(w, "# TYPE solo_ws_connections gauge\n")
		fmt.Fprintf(w, "solo_ws_connections %d\n", m.wsConnections.Load())

		fmt.Fprintf(w, "# HELP solo_daemon_control_connections Current authenticated Daemon control connections\n")
		fmt.Fprintf(w, "# TYPE solo_daemon_control_connections gauge\n")
		fmt.Fprintf(w, "solo_daemon_control_connections %d\n", m.daemonControls.Load())
		fmt.Fprintf(w, "# HELP solo_daemon_connects_total Successful Daemon control handshakes\n")
		fmt.Fprintf(w, "# TYPE solo_daemon_connects_total counter\n")
		fmt.Fprintf(w, "solo_daemon_connects_total %d\n", m.daemonConnects.Load())
		fmt.Fprintf(w, "# HELP solo_remote_runs_queued_total Durable remote Runs queued\n")
		fmt.Fprintf(w, "# TYPE solo_remote_runs_queued_total counter\n")
		fmt.Fprintf(w, "solo_remote_runs_queued_total %d\n", m.remoteRunsQueued.Load())
		fmt.Fprintf(w, "# HELP solo_remote_run_accepts_total Remote Run accepts\n")
		fmt.Fprintf(w, "# TYPE solo_remote_run_accepts_total counter\n")
		fmt.Fprintf(w, "solo_remote_run_accepts_total %d\n", m.remoteRunAccepts.Load())
		fmt.Fprintf(w, "# HELP solo_remote_event_duplicates_total Idempotently ignored remote Run event replays\n")
		fmt.Fprintf(w, "# TYPE solo_remote_event_duplicates_total counter\n")
		fmt.Fprintf(w, "solo_remote_event_duplicates_total %d\n", m.remoteEventDupes.Load())
		fmt.Fprintf(w, "# HELP solo_remote_rpc_timeouts_total Timed out Daemon reverse RPC calls\n")
		fmt.Fprintf(w, "# TYPE solo_remote_rpc_timeouts_total counter\n")
		fmt.Fprintf(w, "solo_remote_rpc_timeouts_total %d\n", m.remoteRPCTimeout.Load())

		fmt.Fprintf(w, "# HELP solo_request_duration_avg_ms Average request duration in milliseconds\n")
		fmt.Fprintf(w, "# TYPE solo_request_duration_avg_ms gauge\n")
		if total > 0 {
			avg := float64(m.requestDurationN.Load()) / float64(total) / 1e6
			fmt.Fprintf(w, "solo_request_duration_avg_ms %0.2f\n", avg)
		} else {
			fmt.Fprintf(w, "solo_request_duration_avg_ms 0\n")
		}
	}
}
