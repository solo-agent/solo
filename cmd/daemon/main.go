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
		AgentSkills: collectAgentSkills(daemonH.agentProviders()),
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
	AgentSkills map[string][]skillloader.DiscoveredSkill `json:"agent_skills,omitempty"`
}

// workspaceSkillDir returns the relative path inside the agent workspace CWD
// where each provider discovers project-level skills.
func workspaceSkillDir(provider string) string {
	switch provider {
	case "claude", "local":
		return ".claude/skills"
	case "codex":
		return ".codex/skills"
	case "opencode":
		return ".opencode/skills"
	case "copilot":
		return ".github/copilot/skills"
	case "cursor":
		return ".cursor/skills"
	case "kiro":
		return ".kiro/skills"
	default:
		return ""
	}
}

// skillRootForProvider maps a backend provider type to the global skill
// directory that CLI agent natively discovers. Paths follow upstream
// conventions documented by each provider. Keep in sync with:
//
//	OpenCode: https://opencode.ai/docs/skills
//	Copilot:  https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-skills
//	Cursor:   https://forum.cursor.com/t/cursor-doesnt-know-new-skills-arens-saved/158507
func skillRootForProvider(provider, home string) *skillloader.SkillRoot {
	switch provider {
	case "claude", "local":
		return &skillloader.SkillRoot{Path: filepath.Join(home, ".claude", "skills"), Kind: "claude", Priority: 60}
	case "codex":
		codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME"))
		if codexHome == "" {
			codexHome = filepath.Join(home, ".codex")
		}
		return &skillloader.SkillRoot{Path: filepath.Join(codexHome, "skills"), Kind: "codex", Priority: 35}
	case "opencode":
		return &skillloader.SkillRoot{Path: filepath.Join(home, ".config", "opencode", "skills"), Kind: "opencode", Priority: 35}
	case "copilot":
		return &skillloader.SkillRoot{Path: filepath.Join(home, ".copilot", "skills"), Kind: "copilot", Priority: 35}
	case "cursor":
		return &skillloader.SkillRoot{Path: filepath.Join(home, ".cursor", "skills"), Kind: "cursor", Priority: 35}
	case "kiro":
		return &skillloader.SkillRoot{Path: filepath.Join(home, ".kiro", "skills"), Kind: "kiro", Priority: 35}
	default:
		return nil
	}
}

// collectAgentSkills scans each agent's global and workspace skill directories.
// Returns per-agent discovered skills. Global skills from the same provider are
// shared across agents of that provider; workspace skills are agent-specific.
// Workspace skills win on name collision (added after global, first-wins).
func collectAgentSkills(agentProviders map[string]string) map[string][]skillloader.DiscoveredSkill {
	if len(agentProviders) == 0 {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		slog.Warn("skill scan: no home dir", "error", err)
		return nil
	}

	// Phase 1: scan global roots per provider (avoid duplicate scans).
	type providerScan struct {
		root    *skillloader.SkillRoot
		agents  []string
	}
	scanned := make(map[string]*providerScan)
	for agentID, provider := range agentProviders {
		if scanned[provider] == nil {
			scanned[provider] = &providerScan{
				root:   skillRootForProvider(provider, home),
				agents: nil,
			}
		}
		scanned[provider].agents = append(scanned[provider].agents, agentID)
	}

	out := make(map[string][]skillloader.DiscoveredSkill)

	// Assign global skills to agents.
	for provider, ps := range scanned {
		if ps.root == nil {
			continue
		}
		discovered, err := skillloader.ScanRoots(home, []skillloader.SkillRoot{*ps.root})
		if err != nil {
			slog.Warn("skill global scan failed", "provider", provider, "path", ps.root.Path, "error", err)
			continue
		}
		flat := make([]skillloader.DiscoveredSkill, 0, len(discovered))
		for _, ds := range discovered {
			flat = append(flat, ds)
		}
		for _, agentID := range ps.agents {
			out[agentID] = append(out[agentID], flat...)
		}
		slog.Debug("skill global scan",
			"provider", provider, "path", ps.root.Path,
			"count", len(flat), "agents", len(ps.agents),
		)
	}

	// Phase 2: scan each agent's workspace dir for project-level skills.
	if workspaceMgr != nil {
		for agentID, provider := range agentProviders {
			relDir := workspaceSkillDir(provider)
			if relDir == "" {
				continue
			}
			wsDir := workspaceMgr.WorkspaceDir(agentID)
			wsRoot := &skillloader.SkillRoot{
				Path:     filepath.Join(wsDir, relDir),
				Kind:     "workspace",
				Priority: 100, // workspace skills override globals
			}
			discovered, err := skillloader.ScanRoots(wsDir, []skillloader.SkillRoot{*wsRoot})
			if err != nil {
				slog.Debug("skill workspace scan failed",
					"agent_id", agentID, "path", wsRoot.Path, "error", err,
				)
				continue
			}
			flat := make([]skillloader.DiscoveredSkill, 0, len(discovered))
			for _, ds := range discovered {
				flat = append(flat, ds)
			}
			if len(flat) > 0 {
				out[agentID] = append(out[agentID], flat...)
				slog.Debug("skill workspace scan",
					"agent_id", agentID, "path", wsRoot.Path, "count", len(flat),
				)
			}
		}
	}

	return out
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
