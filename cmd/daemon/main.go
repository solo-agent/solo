package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/db"
	"github.com/solo-ai/solo/internal/server/middleware"
	"github.com/solo-ai/solo/pkg/agent"
	"github.com/solo-ai/solo/pkg/config"
	"github.com/solo-ai/solo/pkg/llm"
	"github.com/solo-ai/solo/pkg/skillloader"
)

type healthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Version   string `json:"version"`
}

var (
	startTime        = time.Now()
	daemonID         string
	serverURL        string
	internalToken    string
	llmProvider      llm.Provider
	dbPool           *pgxpool.Pool
	machineLock      *agent.MachineLock
	taskMgr          *taskManager
		daemonH          *daemonHandler
	workspaceMgr     *agent.WorkspaceManager
)

func main() {
	_ = config.LoadDotenv()
	cfg := config.Load()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	daemonID = cfg.DaemonID
	serverURL = cfg.ServerURL
	internalToken = cfg.JWTSecret // Use JWT secret as internal token for dev

	port := os.Getenv("DAEMON_PORT")
	if port == "" {
		port = "8081"
	}

	// Connect to database (for persisting agent responses)
	ctx := context.Background()
	var err error
	dbPool, err = db.NewPool(ctx, cfg.DBURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	// Initialize LLM provider
	apiKey := cfg.LLMAPIKey
	if apiKey == "" {
		apiKey = os.Getenv("LLM_API_KEY")
	}
	if apiKey == "" {
		slog.Warn("LLM_API_KEY not set — LLM calls will fail")
	}
	providerType := cfg.LLMProvider
	if providerType == "" {
		providerType = os.Getenv("LLM_PROVIDER")
		if providerType == "" {
			providerType = "anthropic"
		}
	}
	llmProvider = llm.NewProvider(providerType, apiKey)
	slog.Info("LLM provider initialized", "provider", providerType)

	// Acquire machine lock to prevent duplicate daemon processes
	machineLock, err = agent.AcquireLock("", serverURL)
	if err != nil {
		slog.Error("failed to acquire machine lock — another daemon may be running", "error", err)
		os.Exit(1)
	}
	slog.Info("machine lock acquired", "pid", machineLock.PID, "token", machineLock.Token)

	// Create task manager
	taskMgr = newTaskManager()

	// Create handler
	h := newDaemonHandler(dbPool, taskMgr, llmProvider, serverURL, internalToken)
	daemonH = h

	// v1.4: Initialize persistent agent session managers for all registered types.
	for _, meta := range agent.GlobalRegistry().ListMeta() {
		psBackend, err := agent.NewPersistentBackend(meta.Type)
		if err != nil {
			continue // not all types support persistent sessions
		}
		workspaceMgr = agent.NewWorkspaceManager("")
		memoryMgr := agent.NewMemoryManager("")
		sessionMgr := agent.NewAgentSessionManager(psBackend, workspaceMgr, memoryMgr, slog.Default())
		h.SetSessionManager(meta.Type, sessionMgr)
		slog.Info("persistent agent session manager initialized", "provider", meta.Type)
		defer sessionMgr.CloseAll()
	}

	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.CORS())
	r.Use(middleware.Logging(nil))
	r.Use(chimiddleware.Recoverer)

	// Health check (public)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(healthResponse{
			Status:    "ok",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Version:   "0.3.0",
		})
	})

	// Internal daemon endpoints (called by server)
	r.Route("/internal/daemon/tasks/{taskID}", func(r chi.Router) {
		r.Get("/events", h.TaskEvents)   // SSE event stream
		r.Post("/cancel", h.CancelTask)   // Cancel running task
	})
	r.Route("/internal/daemon", func(r chi.Router) {
		r.Post("/run", h.Run)           // Server dispatches agent tasks here
		r.Post("/proxy", h.ProxyRequest) // Agent-to-server proxy
		r.Route("/workspace", func(r chi.Router) {
			r.Get("/list", h.HandleWorkspaceList)
			r.Get("/read", h.HandleWorkspaceRead)
		})
		r.Get("/skills", h.HandleSkillsList)
			r.Route("/worktree", func(r chi.Router) {
				r.Post("/create", h.HandleWorktreeCreate)
				r.Post("/cleanup", h.HandleWorktreeCleanup)
			})
		r.Post("/agents/{agentID}/cleanup", h.CleanupAgent) // server-initiated hard cleanup
	})

	// SSE requires long-lived connections — no write timeout.
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // 0 = no timeout (needed for SSE streaming)
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("daemon server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("daemon server error", "error", err)
			os.Exit(1)
		}
	}()

	// Register with server on startup
	if err := registerWithServer(ctx); err != nil {
		slog.Error("failed to register with server", "error", err)
		// Non-fatal: daemon can still operate standalone
	} else {
		// Start heartbeat after successful registration
		go heartbeatLoop(ctx)
	}

	<-ctx.Done()
	slog.Info("daemon server shutting down")

	// Unregister on shutdown
	unregisterFromServer()

	// Release machine lock
	if machineLock != nil {
		if err := machineLock.Release(); err != nil {
			slog.Warn("failed to release machine lock", "error", err)
		} else {
			slog.Info("machine lock released")
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("daemon shutdown error", "error", err)
		os.Exit(1)
	}
	slog.Info("daemon server stopped")
}

// registerWithServer sends a registration request to the server.
func registerWithServer(ctx context.Context) error {
	if serverURL == "" {
		return fmt.Errorf("DAEMON_SERVER_URL not set")
	}

	host := getOutboundIP()
	// When the server is on localhost, register with 127.0.0.1 so the
	// server always reaches the daemon via loopback — avoids firewall /
	// NAT issues with the outbound IP.
	if strings.Contains(serverURL, "localhost") || strings.Contains(serverURL, "127.0.0.1") {
		host = "127.0.0.1"
	}
	portStr := os.Getenv("DAEMON_PORT")
	port := 8081
	if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
		port = p
	}

	req := daemonRegisterPayload{
		DaemonID:      daemonID,
		Host:          host,
		Port:          port,
		Version:       "0.3.0",
		Capabilities:  []string{"llm"},
		MaxConcurrent: 10,
		CurrentLoad:   0,
		AgentTypes:    registeredAgentTypes(),
		SystemInfo:    collectSystemInfo(),
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal register request: %w", err)
	}

	url := serverURL + "/internal/daemon/register"
	resp, err := sendInternalRequest(ctx, http.MethodPost, url, "application/json", payload)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var regResp daemonRegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&regResp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	slog.Info("registered with server",
		"daemon_id", daemonID,
		"host", host,
		"port", port,
		"heartbeat_interval", regResp.HeartbeatInterval,
	)

	return nil
}

// unregisterFromServer sends a deregistration request (best-effort).
func unregisterFromServer() {
	if serverURL == "" {
		return
	}

	req := map[string]string{"daemon_id": daemonID}
	payload, _ := json.Marshal(req)

	url := serverURL + "/internal/daemon/unregister"
	resp, err := sendInternalRequest(context.Background(), http.MethodPost, url, "application/json", payload)
	if err != nil {
		slog.Warn("failed to unregister from server", "error", err)
		return
	}
	resp.Body.Close()
	slog.Info("unregistered from server")
}

// sendInternalRequest sends an HTTP request with the internal auth header.
func sendInternalRequest(ctx context.Context, method, url, contentType string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	if internalToken != "" {
		req.Header.Set("Authorization", "Internal-Token "+internalToken)
	}
	return http.DefaultClient.Do(req)
}

// heartbeatLoop sends periodic heartbeats to the server.
func heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sendHeartbeat()
		case <-ctx.Done():
			return
		}
	}
}

// sendHeartbeat sends a single heartbeat to the server.
func sendHeartbeat() {
	if serverURL == "" {
		return
	}

	req := daemonHeartbeatPayload{
		DaemonID:    daemonID,
		Load:        0,
		MaxLoad:     10,
		UptimeSec:   int64(time.Since(startTime).Seconds()),
		ActiveTasks: taskMgr.ListActiveTasks(),
		AgentIDs:    daemonH.activeSessionAgentIDs(),
		SystemInfo:  collectSystemInfo(),
		// Skills served on-demand via /internal/daemon/skills
	}

	payload, err := json.Marshal(req)
	if err != nil {
		slog.Error("failed to marshal heartbeat", "error", err)
		return
	}

	url := serverURL + "/internal/daemon/heartbeat"
	resp, err := sendInternalRequest(context.Background(), http.MethodPost, url, "application/json", payload)
	if err != nil {
		slog.Warn("heartbeat failed, attempting re-registration", "error", err)
		if regErr := registerWithServer(context.Background()); regErr != nil {
			slog.Error("re-registration after heartbeat failure also failed", "error", regErr)
		}
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		slog.Warn("heartbeat: daemon unknown to server, re-registering")
		if regErr := registerWithServer(context.Background()); regErr != nil {
			slog.Error("re-registration failed", "error", regErr)
		}
	}

	slog.Debug("heartbeat sent", "daemon_id", daemonID)
}

// getOutboundIP returns the preferred outbound IP address.
func getOutboundIP() string {
	conn, err := net.DialTimeout("udp", "8.8.8.8:80", 5*time.Second)
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

// --- System info collected at daemon startup ---

// SystemInfo carries OS, hostname and local IP reported to the server.
type SystemInfo struct {
	OS       string `json:"os"`
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
}

// collectSystemInfo gathers OS, hostname, and primary local IP at startup.
func collectSystemInfo() SystemInfo {
	hostname, _ := os.Hostname()
	return SystemInfo{
		OS:       runtime.GOOS,
		Hostname: hostname,
		IP:       getOutboundIP(),
	}
}

// --- Request/Response types for daemon-server communication ---

type daemonRegisterPayload struct {
	DaemonID      string     `json:"daemon_id"`
	Host          string     `json:"host"`
	Port          int        `json:"port"`
	Version       string     `json:"version"`
	Capabilities  []string   `json:"capabilities"`
	MaxConcurrent int        `json:"max_concurrent"`
	CurrentLoad   int32      `json:"current_load"`
	AgentTypes    []string   `json:"agent_types"`
	SystemInfo    SystemInfo `json:"system_info"`
}

type daemonRegisterResponse struct {
	Status            string `json:"status"`
	HeartbeatInterval int    `json:"heartbeat_interval"`
}

type daemonHeartbeatPayload struct {
	DaemonID    string                                  `json:"daemon_id"`
	Load        int32                                   `json:"load"`
	MaxLoad     int                                     `json:"max_load"`
	UptimeSec   int64                                   `json:"uptime_seconds"`
	ActiveTasks []string                                `json:"active_tasks"`
	AgentIDs    []string                                `json:"agent_ids"`
	SystemInfo  SystemInfo                              `json:"system_info"`
	// Skills are served on-demand via /internal/daemon/skills, not in heartbeat.
}

// ---- Skill path resolution ------------------------------------------------
// Each provider may discover skills from multiple directories (native paths,
// shared agent-ecosystem paths, and a universal fallback). See skill-system
// design docs for the full priority table.
//
// Universal fallback paths (all providers):
//   Global:    ~/.agents/skills/
//   Workspace: <ws>/.agents/skills/

// agentGlobalRoots returns all global skill directories for a provider.
// Each path has its own kind — no aliasing, no merging.
func agentGlobalRoots(provider, home string) []skillloader.SkillRoot {
	switch provider {
	case "claude", "local":
		return []skillloader.SkillRoot{
			{Path: filepath.Join(home, ".claude", "skills"), Kind: "claude", Priority: 60},
		}
	case "codex":
		codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME"))
		if codexHome == "" {
			codexHome = filepath.Join(home, ".codex")
		}
		return []skillloader.SkillRoot{
			{Path: filepath.Join(codexHome, "skills"), Kind: "codex", Priority: 35},
			{Path: filepath.Join(home, ".agents", "skills"), Kind: "agents", Priority: 25},
		}
	case "opencode":
		return []skillloader.SkillRoot{
			{Path: filepath.Join(home, ".config", "opencode", "skills"), Kind: "opencode", Priority: 35},
			{Path: filepath.Join(home, ".claude", "skills"), Kind: "claude", Priority: 30},
			{Path: filepath.Join(home, ".agents", "skills"), Kind: "agents", Priority: 25},
		}
	case "openclaw":
		return []skillloader.SkillRoot{
			{Path: filepath.Join(home, ".openclaw", "skills"), Kind: "openclaw", Priority: 35},
			{Path: filepath.Join(home, ".agents", "skills"), Kind: "agents", Priority: 25},
		}
	case "copilot":
		return []skillloader.SkillRoot{
			{Path: filepath.Join(home, ".copilot", "skills"), Kind: "copilot", Priority: 35},
			{Path: filepath.Join(home, ".agents", "skills"), Kind: "agents", Priority: 25},
		}
	case "cursor":
		return []skillloader.SkillRoot{
			{Path: filepath.Join(home, ".cursor", "skills"), Kind: "cursor", Priority: 35},
			{Path: filepath.Join(home, ".agents", "skills"), Kind: "agents", Priority: 25},
		}
	case "kiro":
		return []skillloader.SkillRoot{
			{Path: filepath.Join(home, ".kiro", "skills"), Kind: "kiro", Priority: 35},
			{Path: filepath.Join(home, ".agents", "skills"), Kind: "agents", Priority: 25},
		}
	case "hermes":
		return []skillloader.SkillRoot{
			{Path: filepath.Join(home, ".hermes", "skills"), Kind: "hermes", Priority: 35},
		}
	case "pi":
		return []skillloader.SkillRoot{
			{Path: filepath.Join(home, ".pi", "agent", "skills"), Kind: "pi", Priority: 35},
			{Path: filepath.Join(home, ".agents", "skills"), Kind: "agents", Priority: 25},
		}
	default:
		return nil
	}
}

// agentWorkspaceRoots returns all workspace-relative skill directories for a
// provider. Kind is prefixed with "ws-" so the frontend can separate sections.
func agentWorkspaceRoots(provider, wsDir string) []skillloader.SkillRoot {
	switch provider {
	case "claude", "local":
		return []skillloader.SkillRoot{
			{Path: filepath.Join(wsDir, ".claude", "skills"), Kind: "ws-claude", Priority: 100},
		}
	case "codex":
		return []skillloader.SkillRoot{
			{Path: filepath.Join(wsDir, ".codex", "skills"), Kind: "ws-codex", Priority: 100},
		}
	case "opencode":
		return []skillloader.SkillRoot{
			{Path: filepath.Join(wsDir, ".opencode", "skills"), Kind: "ws-opencode", Priority: 100},
			{Path: filepath.Join(wsDir, ".claude", "skills"), Kind: "ws-claude", Priority: 90},
			{Path: filepath.Join(wsDir, ".agents", "skills"), Kind: "ws-opencode", Priority: 80},
		}
	case "copilot":
		return []skillloader.SkillRoot{
			{Path: filepath.Join(wsDir, ".github", "copilot", "skills"), Kind: "ws-copilot", Priority: 100},
		}
	case "cursor":
		return []skillloader.SkillRoot{
			{Path: filepath.Join(wsDir, ".cursor", "skills"), Kind: "ws-cursor", Priority: 100},
		}
	case "kiro":
		return []skillloader.SkillRoot{
			{Path: filepath.Join(wsDir, ".kiro", "skills"), Kind: "ws-kiro", Priority: 100},
		}
	case "openclaw":
		return []skillloader.SkillRoot{
			{Path: filepath.Join(wsDir, "skills"), Kind: "ws-openclaw", Priority: 100},
			{Path: filepath.Join(wsDir, ".agents", "skills"), Kind: "ws-openclaw", Priority: 80},
		}
	case "hermes":
		return []skillloader.SkillRoot{
			{Path: filepath.Join(wsDir, ".hermes", "skills"), Kind: "ws-hermes", Priority: 100},
		}
	case "pi":
		return []skillloader.SkillRoot{
			{Path: filepath.Join(wsDir, ".pi", "skills"), Kind: "ws-pi", Priority: 100},
		}
	default:
		return nil
	}
}


// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writeJSON encode error", "error", err)
	}
}

// registeredAgentTypes returns all agent types from the global registry.
func registeredAgentTypes() []string {
	metas := agent.GlobalRegistry().ListMeta()
	types := make([]string, 0, len(metas))
	for _, m := range metas {
		types = append(types, m.Type)
	}
	return types
}
