package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/server/handler"
	"github.com/solo-ai/solo/internal/server/middleware"
	"github.com/solo-ai/solo/internal/server/service"
	"github.com/solo-ai/solo/internal/server/ws"
	"github.com/solo-ai/solo/pkg/metrics"
)

const (
	// maxMessageBodyBytes is the maximum request body size (100 KB) for
	// message creation and update endpoints.
	maxMessageBodyBytes = 100 * 1024 // 100 KB
)

// NewRouter creates the fully-configured Chi router with all middleware and routes.
// It accepts the DB pool, WebSocket hub, daemon manager, and agent service.
func NewRouter(pool *pgxpool.Pool, hub *ws.Hub, dm *service.DaemonManager, agentSvc *service.AgentService) chi.Router {
	r := chi.NewRouter()

	// ---- Global middleware (applied to all routes) ----
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.Logging(nil))
	r.Use(chimw.Recoverer)
	r.Use(middleware.CORS())

	// Security headers on all responses
	r.Use(middleware.SecurityHeaders())

	// ---- Metrics + health endpoints (no auth, no rate limit) ----
	r.Get("/metrics", metrics.Global.Handler())
	r.Get("/healthz", livenessHandler())
	r.Get("/readyz", readinessHandler(pool, dm))

	// Initialize services
	taskSvc := service.NewTaskService(pool)
	computerSvc := service.NewComputerService(pool)

	// Initialize handlers
	authHandler := handler.NewAuthHandler(pool)
	channelHandler := handler.NewChannelHandler(pool)
	memberHandler := handler.NewMemberHandler(pool, agentSvc)
	messageHandler := handler.NewMessageHandler(pool, hub, agentSvc, taskSvc)
	agentHandler := handler.NewAgentHandler(pool)
	threadHandler := handler.NewThreadHandler(pool, hub, agentSvc)
	dmHandler := handler.NewDMHandler(pool, hub, agentSvc, taskSvc)
	daemonHandler := handler.NewDaemonHandler(dm, agentSvc, computerSvc)
	mentionSvc := service.NewMentionService(pool)
	taskHandler := handler.NewTaskHandler(pool, hub, agentSvc, taskSvc, mentionSvc)
	searchHandler := handler.NewSearchHandler(pool)
	computerHandler := handler.NewComputerHandler(computerSvc)

	// Attachment handler
	uploadDir := os.Getenv("ATTACHMENTS_DIR")
	if uploadDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			uploadDir = filepath.Join(home, ".solo", "attachments")
		} else {
			uploadDir = filepath.Join(".", "attachments")
		}
	}
	attachmentHandler := handler.NewAttachmentHandler(pool, uploadDir)

	// ---- Internal daemon routes (no JWT auth, no rate limit) ----
	r.Route("/internal/daemon", func(r chi.Router) {
		r.Use(middleware.InternalAuth())

		r.Post("/register", daemonHandler.Register)
		r.Post("/heartbeat", daemonHandler.Heartbeat)
		r.Post("/unregister", daemonHandler.Unregister)

		// Task callbacks from daemon
		r.Route("/tasks/{taskID}", func(r chi.Router) {
			r.Post("/complete", daemonHandler.TaskComplete)
			r.Post("/error", daemonHandler.TaskError)
		})
	})

	// Public attachment serving (no auth needed for inline images)
	r.Get("/api/v1/attachments/{attachmentID}", attachmentHandler.Serve)
	r.Get("/api/v1/attachments/{attachmentID}/thumbnail", attachmentHandler.ServeThumbnail)

	// ---- Public auth routes (rate-limited: 10 req/min) ----
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Use(middleware.RateLimiter(10.0/60.0, 10))

		r.Post("/register", authHandler.Register)
		r.Post("/login", authHandler.Login)
		r.Post("/refresh", authHandler.Refresh)
	})

	// ---- Protected routes (rate-limited: 100 req/s) ----
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth())
		r.Use(middleware.RateLimiter(100, 100))

		// Auth logout requires authentication
		r.Post("/api/v1/auth/logout", authHandler.Logout)

		// User routes
		r.Get("/api/v1/users/me", func(w http.ResponseWriter, r *http.Request) {
			uid := r.Header.Get("X-User-ID")
			email := r.Header.Get("X-User-Email")
			name := r.Header.Get("X-User-Name")
			writeJSON(w, http.StatusOK, map[string]string{
				"id":           uid,
				"email":        email,
				"display_name": name,
			})
		})

		// Channel routes
		r.Get("/api/v1/server/info", channelHandler.ServerInfo)

		r.Post("/api/v1/channels/join", channelHandler.JoinByTarget)

		r.Route("/api/v1/channels", func(r chi.Router) {
			r.Get("/", channelHandler.List)
			r.Post("/", channelHandler.Create)

			r.Route("/{channelID}", func(r chi.Router) {
				r.Get("/", channelHandler.Get)
				r.Patch("/", channelHandler.Update)
				r.Delete("/", channelHandler.Delete)

				// Channel member management routes
				r.Route("/members", func(r chi.Router) {
					r.Get("/", memberHandler.ListMembers)
					r.Post("/", memberHandler.AddMember)
					r.Delete("/{memberID}", memberHandler.RemoveMember)
				})

				// Task routes (SOLO-122-B)
				r.Route("/tasks", func(r chi.Router) {
					r.Get("/", taskHandler.List)
					r.Post("/", taskHandler.Create)

					r.Route("/{taskID}", func(r chi.Router) {
						r.Get("/", taskHandler.Get)
						r.Patch("/", taskHandler.Update)
						r.Delete("/", taskHandler.Delete)

						// Claim / Unclaim (Phase 1)
						r.Post("/claim", taskHandler.Claim)
						r.Delete("/claim", taskHandler.Unclaim)
					})
				})

				// Channel messages routes (with body size limit)
				r.Route("/messages", func(r chi.Router) {
					r.Use(middleware.MaxBodySize(maxMessageBodyBytes))

					r.Get("/", messageHandler.List)
					r.Post("/", messageHandler.Create)

					// Message edit/delete (W3-02-BE)
					r.Patch("/{messageID}", messageHandler.Update)
					r.Delete("/{messageID}", messageHandler.Delete)

					// Convert message to task (Phase 1)
					r.Post("/{messageID}/convert-to-task", taskHandler.ConvertToTask)

					// Thread routes (nested under messages)
					r.Route("/{messageID}/thread", func(r chi.Router) {
						r.Post("/", threadHandler.CreateThreadReply)
						r.Get("/", threadHandler.ListThreadMessages)
					})
				})
			})
		})

		// Agent routes
		r.Route("/api/v1/agents", func(r chi.Router) {
			r.Get("/", agentHandler.List)
			r.Post("/", agentHandler.Create)

			r.Route("/{agentID}", func(r chi.Router) {
				r.Get("/", agentHandler.Get)
				r.Patch("/", agentHandler.Update)
				r.Delete("/", agentHandler.Delete)
			})
		})

		// Agent backends metadata (registered backend adapters)
		r.Get("/api/v1/agent-backends", agentHandler.AgentBackends)
		r.Get("/api/v1/agent-backends/detect", agentHandler.AgentBackendsDetect)

		// Global tasks routes (all channels)
		r.Route("/api/v1/tasks", func(r chi.Router) {
			r.Get("/", taskHandler.ListAll)
			r.Post("/", taskHandler.CreateGlobal)
			r.Route("/{taskID}", func(r chi.Router) {
				r.Get("/", taskHandler.GetGlobal)
				r.Patch("/", taskHandler.UpdateGlobal)
				r.Delete("/", taskHandler.DeleteGlobal)
			})
		})

		// DM routes (SOLO-55-B, SOLO-56-B)
		r.Route("/api/v1/dm", func(r chi.Router) {
			r.Get("/", dmHandler.ListDMs)
			r.Post("/", dmHandler.CreateOrGetDM)

			r.Route("/{dmID}", func(r chi.Router) {
				r.Get("/", dmHandler.GetDM)

				r.Route("/messages", func(r chi.Router) {
					r.Use(middleware.MaxBodySize(maxMessageBodyBytes))
					r.Get("/", dmHandler.ListMessages)
					r.Post("/", dmHandler.SendMessage)

					// DM message edit/delete (W3-03-BE)
					r.Patch("/{messageID}", dmHandler.UpdateMessage)
					r.Delete("/{messageID}", dmHandler.DeleteMessage)

					// Convert message to task (asTask)
					r.Post("/{messageID}/convert-to-task", dmHandler.ConvertMessageToTask)
				})
				// DM task routes (v1.2 Phase 2)
				r.Route("/tasks", func(r chi.Router) {
					r.Get("/", dmHandler.ListTasks)
					r.Post("/", dmHandler.CreateTask)

					r.Route("/{taskID}", func(r chi.Router) {
						r.Get("/", dmHandler.GetTask)
						r.Patch("/", dmHandler.UpdateTask)
						r.Delete("/", dmHandler.DeleteTask)

						r.Post("/claim", dmHandler.ClaimTask)
						r.Delete("/claim", dmHandler.UnclaimTask)
					})
				})
			})
		})
		// Search route (SOLO-234-B)
		r.Get("/api/v1/search", searchHandler.Search)

		// Computer routes (SOLO-241-B)
		r.Route("/api/v1/computers", func(r chi.Router) {
			r.Get("/", computerHandler.List)
			r.Post("/", computerHandler.Create)

			r.Route("/{computerID}", func(r chi.Router) {
				r.Get("/", computerHandler.Get)
				r.Patch("/", computerHandler.Update)
				r.Delete("/", computerHandler.Delete)
			})
		})

		// Thread read-status routes (P25-02-B)
		r.Route("/api/v1/threads", func(r chi.Router) {
			r.Post("/{threadID}/mark-read", threadHandler.MarkThreadRead)
		})

		// Attachment routes (SOLO-243-B)
		r.Post("/api/v1/attachments/upload", attachmentHandler.Upload)
	})

	// WebSocket endpoint (authenticates via ?token query param — browser
	// WebSocket API cannot set custom headers, so it must be outside the
	// auth middleware group).
	r.Get("/api/v1/ws", hub.ServeWS)

	return r
}

// livenessHandler responds 200 OK to indicate the server process is alive.
// This is the k8s liveness probe endpoint. It does not check dependencies.
func livenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"alive"}`))
	}
}

// readinessHandler responds 200 OK when the server is ready to serve traffic.
// This is the k8s readiness probe endpoint. It checks:
//   - Database connectivity (ping)
//   - Daemon manager status (at least one online daemon is optional)
//
// Returns 503 Service Unavailable if any essential dependency is down.
func readinessHandler(pool *pgxpool.Pool, dm *service.DaemonManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check database connectivity
		err := pool.Ping(r.Context())
		if err != nil {
			slog.Error("readiness check: database ping failed", "error", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "unhealthy",
				"reason": "database unreachable",
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writeJSON encode error", "error", err)
	}
}
