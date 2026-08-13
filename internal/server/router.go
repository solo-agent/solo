package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

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
func NewRouter(ctx context.Context, pool *pgxpool.Pool, hub *ws.Hub, dm *service.DaemonManager, agentSvc *service.AgentService) chi.Router {
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
	workspaceRoot := defaultAgentWorkspaceRoot()
	relationshipMD := service.NewRelationshipsMDGenerator(pool, workspaceRoot)
	taskSvc := service.NewTaskService(pool)
	automationSvc := service.NewAutomationService(pool, taskSvc, hub, agentSvc)
	sendDedupe := service.NewSendDedupe(1000, 5*time.Minute)
	taskSvc.SetAgentNotifier(service.NewAgentNotifier(pool, hub, agentSvc))
	relationshipSvc := service.NewAgentRelationshipService(pool, relationshipMD)
	templateSvc := service.NewTemplateService(pool, relationshipMD)
	teamFormationSvc := service.NewTeamFormationService(pool, relationshipMD, hub, templateSvc)
	computerSvc := service.NewComputerService(pool)
	inboxSvc := service.NewInboxService(pool)
	artifactRoot := os.Getenv("ARTIFACTS_DIR")
	if artifactRoot == "" {
		if home, err := os.UserHomeDir(); err == nil {
			artifactRoot = filepath.Join(home, ".solo", "artifacts")
		} else {
			artifactRoot = filepath.Join(".", ".solo", "artifacts")
		}
	}
	artifactSvc := service.NewArtifactService(pool, artifactRoot)
	if agentSvc != nil {
		artifactSvc.SetAgentArtifactRequester(agentSvc.TriggerAgentForArtifact)
	}
	taskSvc.SetArtifactGenerator(func(ctx context.Context, taskID, userID string) (string, error) {
		artifact, err := artifactSvc.GenerateLatest(ctx, taskID, userID)
		if err != nil {
			return "", err
		}
		if artifact.Summary == "pending" {
			return "pending", nil
		}
		return "available", nil
	})

	// Initialize handlers
	authHandler := handler.NewAuthHandler(pool, agentSvc)
	channelHandler := handler.NewChannelHandler(pool, dm, templateSvc)
	memberHandler := handler.NewMemberHandler(pool, agentSvc, dm)
	messageHandler := handler.NewMessageHandler(pool, hub, agentSvc, taskSvc, sendDedupe)
	agentHandler := handler.NewAgentHandler(pool, dm, hub, agentSvc)
	agentRunHandler := handler.NewAgentRunHandler(pool, dm)
	budgetHandler := handler.NewBudgetHandler(service.NewBudgetService(pool))
	dashboardHandler := handler.NewDashboardHandler(pool)
	threadHandler := handler.NewThreadHandler(pool, hub, agentSvc, sendDedupe)
	thinkingHandler := handler.NewThinkingHandler(pool, hub, agentSvc)
	dmHandler := handler.NewDMHandler(pool, hub, agentSvc, taskSvc, sendDedupe)
	daemonHandler := handler.NewDaemonHandler(dm, agentSvc, computerSvc)
	daemonControlHandler := handler.NewDaemonControlHandler(computerSvc, dm)
	mentionSvc := service.NewMentionService(pool)
	taskHandler := handler.NewTaskHandler(pool, hub, agentSvc, taskSvc, mentionSvc)
	automationHandler := handler.NewAutomationHandler(automationSvc)
	relationshipHandler := handler.NewAgentRelationshipHandler(relationshipSvc)
	templateHandler := handler.NewTemplateHandler(templateSvc)
	teamFormationHandler := handler.NewTeamFormationHandler(teamFormationSvc)
	searchHandler := handler.NewSearchHandler(pool)
	computerHandler := handler.NewComputerHandler(computerSvc, dm, pool)
	inboxHandler := handler.NewInboxHandler(inboxSvc)
	artifactHandler := handler.NewArtifactHandler(artifactSvc)
	artifactHandler.SetTaskBroadcaster(taskSvc, hub)
	onboardingHandler := handler.NewOnboardingHandler(pool, agentSvc)
	workspaceHandler := handler.NewWorkspaceHandler(pool, dm)
	go automationSvc.Start(ctx)

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

	// Remote Daemon enrollment is intentionally outside user JWT auth. The
	// one-time token is the credential and this endpoint has its own strict rate limit.
	r.With(middleware.RateLimiter(5.0/60.0, 5), chimw.AllowContentType("application/json")).
		Post("/internal/v1/daemon/enroll", daemonControlHandler.Enroll)
	r.Group(func(r chi.Router) {
		r.Use(middleware.ComputerAuth(computerSvc))
		r.Get("/internal/v1/daemon/connect", daemonControlHandler.Connect)
		r.Get("/internal/v1/daemon/runs/pending", daemonControlHandler.PendingRuns)
		r.Post("/internal/v1/daemon/runs/{runID}/accept", daemonControlHandler.AcceptRun)
		r.Post("/internal/v1/daemon/runs/{runID}/events", daemonControlHandler.AppendRunEvent)
	})

	// Local compatibility transport is never exposed by a production Server.
	if os.Getenv("APP_ENV") != "production" {
		r.Route("/internal/daemon", func(r chi.Router) {
			r.Use(middleware.InternalAuth())

			r.Post("/register", daemonHandler.Register)
			r.Post("/heartbeat", daemonHandler.Heartbeat)
			r.Post("/unregister", daemonHandler.Unregister)

			r.Route("/tasks/{taskID}", func(r chi.Router) {
				r.Post("/complete", daemonHandler.TaskComplete)
				r.Post("/error", daemonHandler.TaskError)
			})
		})
	}

	// Attachment bodies enforce owner/channel authorization in the handler.
	r.Get("/api/v1/attachments/{attachmentID}", attachmentHandler.Serve)
	r.Get("/api/v1/attachments/{attachmentID}/thumbnail", attachmentHandler.ServeThumbnail)

	// ---- Public auth routes (rate-limited: 10 req/min) ----
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Use(middleware.RateLimiter(10.0/60.0, 10))

		r.Get("/config", authHandler.PublicConfig)
		r.Post("/register", authHandler.Register)
		r.Post("/register/verify", authHandler.VerifyRegistration)
		r.Post("/password/forgot", authHandler.ForgotPassword)
		r.Post("/password/reset", authHandler.ResetPassword)
		r.Post("/login", authHandler.Login)
		r.Post("/refresh", authHandler.Refresh)
	})

	// Scoped, read-only Guest links use their own revocable credentials and
	// never enter the authenticated user/Agent middleware path.
	r.Route("/api/v1/guest", func(r chi.Router) {
		r.Use(middleware.RateLimiter(60.0/60.0, 30))
		r.Get("/embed", workspaceHandler.GuestEmbed)
		r.Get("/channels/{channelID}/messages", workspaceHandler.GuestMessages)
	})

	// ---- Protected routes (rate-limited: 100 req/s) ----
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(pool))
		r.Use(middleware.RateLimiter(100, 100))
		r.Use(middleware.WorkspaceScope(pool))

		// Auth logout requires authentication
		r.Post("/api/v1/auth/logout", authHandler.Logout)

		// User routes
		r.Get("/api/v1/users/me", authHandler.CurrentUser)
		r.Patch("/api/v1/users/me", authHandler.UpdateCurrentUser)
		r.Route("/api/v1/workspaces", func(r chi.Router) {
			r.Get("/", workspaceHandler.List)
			r.Post("/", workspaceHandler.Create)
			r.Route("/{workspaceID}", func(r chi.Router) {
				r.Patch("/", workspaceHandler.Update)
				r.Delete("/", workspaceHandler.Delete)
				r.Route("/members", func(r chi.Router) {
					r.Get("/", workspaceHandler.Members)
					r.Post("/", workspaceHandler.AddMember)
					r.Patch("/{userID}", workspaceHandler.UpdateMember)
					r.Delete("/{userID}", workspaceHandler.RemoveMember)
				})
				r.Route("/invitations", func(r chi.Router) {
					r.Get("/", workspaceHandler.Invitations)
					r.Delete("/{invitationID}", workspaceHandler.DeleteInvitation)
				})
				r.Route("/join-rules", func(r chi.Router) {
					r.Get("/", workspaceHandler.JoinRules)
					r.Post("/", workspaceHandler.AddJoinRule)
					r.Delete("/{ruleID}", workspaceHandler.DeleteJoinRule)
				})
				r.Route("/embed", func(r chi.Router) {
					r.Get("/", workspaceHandler.EmbedSettings)
					r.Put("/", workspaceHandler.UpdateEmbedSettings)
					r.Post("/tokens", workspaceHandler.CreateGuestToken)
					r.Delete("/tokens/{tokenID}", workspaceHandler.RevokeGuestToken)
				})
			})
		})
		r.Get("/api/v1/users/me/budget", budgetHandler.GetCurrentUser)
		r.Put("/api/v1/users/me/budget", budgetHandler.SaveCurrentUser)
		// Channel routes
		r.Get("/api/v1/server/info", channelHandler.ServerInfo)

		r.Get("/api/v1/messages/check", messageHandler.Check)
		r.Post("/api/v1/channels/join", channelHandler.JoinByTarget)

		r.Route("/api/v1/channels", func(r chi.Router) {
			r.Get("/", channelHandler.List)
			r.Post("/", channelHandler.Create)
			r.Get("/lucy", channelHandler.GetLucy)

			r.Route("/{channelID}", func(r chi.Router) {
				r.Get("/", channelHandler.Get)
				r.Patch("/", channelHandler.Update)
				r.Delete("/", channelHandler.Delete)
				r.Post("/template", channelHandler.ApplyTemplate)

				// Ordinary Agents are created and listed only inside their home Channel.
				r.Route("/agents", func(r chi.Router) {
					r.Get("/", agentHandler.List)
					r.Post("/", agentHandler.Create)
				})

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
						r.Post("/submit", taskHandler.Submit)
						r.Post("/accept", taskHandler.Accept)
						r.Post("/reject", taskHandler.Reject)
						r.Post("/close", taskHandler.Close)
						r.Post("/reopen", taskHandler.Reopen)
					})
				})

				// Channel automation routes.
				r.Route("/automations", func(r chi.Router) {
					r.Get("/", automationHandler.List)
					r.Post("/", automationHandler.Create)

					r.Route("/{automationID}", func(r chi.Router) {
						r.Get("/", automationHandler.Get)
						r.Patch("/", automationHandler.Update)
						r.Delete("/", automationHandler.Delete)
						r.Post("/run", automationHandler.RunNow)
						r.Get("/runs", automationHandler.ListRuns)
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

				// Thinking mode: one node-scoped conversation graph per channel.
				r.Route("/thinking", func(r chi.Router) {
					r.Get("/", thinkingHandler.Get)
					r.Post("/", thinkingHandler.Ensure)
					r.Post("/nodes/{nodeID}/children", thinkingHandler.CreateChild)
					r.Post("/nodes/{nodeID}/handoff/retry", thinkingHandler.RetryForkHandoff)
					r.Post("/nodes/{nodeID}/handoff/refresh", thinkingHandler.RefreshCheckpoint)
					r.Post("/nodes/{nodeID}/return", thinkingHandler.ReturnNode)
				})
			})
		})

		// Agent routes
		r.Route("/api/v1/agents", func(r chi.Router) {
			r.Get("/", agentHandler.ListWorkspace)
			r.Route("/{agentID}", func(r chi.Router) {
				r.Get("/", agentHandler.Get)
				r.Patch("/", agentHandler.Update)
				r.Delete("/", agentHandler.Delete)

				// Agent workspace files (v1.5)
				r.Get("/workspace", agentHandler.Workspace)
				r.Get("/runs", agentRunHandler.AgentRuns)
				r.Get("/sessions", agentRunHandler.AgentSessions)
				r.Get("/tasks", agentRunHandler.AgentTasks)
				r.Get("/budget", budgetHandler.GetAgent)
				r.Put("/budget", budgetHandler.SaveAgent)

				// Agent skill catalog (proxied to daemon).
				r.Get("/skills", agentHandler.AgentSkills)

				r.Get("/relationships", relationshipHandler.ListByAgent)
			})
		})

		r.Route("/api/v1/agent-relationships", func(r chi.Router) {
			r.Get("/", relationshipHandler.List)
			r.Post("/", relationshipHandler.Create)

			r.Route("/{relationshipID}", func(r chi.Router) {
				r.Patch("/", relationshipHandler.Update)
				r.Delete("/", relationshipHandler.Delete)
			})
		})

		r.Route("/api/v1/templates", func(r chi.Router) {
			r.Get("/", templateHandler.List)
			r.Get("/{templateID}", templateHandler.Get)
		})

		// Lucy-only declarative auto-team provisioning.
		r.Post("/api/v1/team-formations", teamFormationHandler.Form)

		// Agent backends metadata (registered backend adapters)
		r.Get("/api/v1/agent-backends", agentHandler.AgentBackends)
		r.Get("/api/v1/agent-backends/detect", agentHandler.AgentBackendsDetect)

		// Onboarding wizard
		r.Post("/api/v1/onboarding/create-lucy", onboardingHandler.CreateLucy)

		r.Get("/api/v1/agent-runs", agentRunHandler.Recent)
		r.Get("/api/v1/agent-runs/active", agentRunHandler.Active)
		r.Get("/api/v1/agent-runs/{runID}", agentRunHandler.Get)
		r.Get("/api/v1/agent-runs/{runID}/events", agentRunHandler.Events)
		r.Get("/api/v1/agent-runs/{runID}/transcript", agentRunHandler.Transcript)
		r.Get("/api/v1/agent-sessions/{sessionID}/timeline", agentRunHandler.SessionTimeline)
		r.Get("/api/v1/dashboard/live", dashboardHandler.Live)
		r.Get("/api/v1/dashboard/insight", dashboardHandler.Insight)

		// Global tasks routes (all channels)
		r.Get("/api/v1/tasks", taskHandler.ListAll)
		r.Post("/api/v1/tasks", taskHandler.CreateGlobal)
		r.Get("/api/v1/tasks/{taskID}", taskHandler.GetGlobal)
		r.Get("/api/v1/tasks/{taskID}/runs", agentRunHandler.TaskRuns)
		r.Get("/api/v1/tasks/{taskID}/agent-timeline", agentRunHandler.TaskTimeline)
		r.Patch("/api/v1/tasks/{taskID}", taskHandler.UpdateGlobal)
		r.Delete("/api/v1/tasks/{taskID}", taskHandler.DeleteGlobal)
		r.Post("/api/v1/tasks/{taskID}/accept", taskHandler.AcceptGlobal)
		r.Post("/api/v1/tasks/{taskID}/reject", taskHandler.RejectGlobal)
		r.Post("/api/v1/tasks/{taskID}/close", taskHandler.CloseGlobal)
		r.Post("/api/v1/tasks/{taskID}/reopen", taskHandler.ReopenGlobal)
		r.Post("/api/v1/tasks/{taskID}/artifact", artifactHandler.GenerateLatest)
		r.Post("/api/v1/tasks/{taskID}/artifact/finalize", artifactHandler.Finalize)
		r.Post("/api/v1/tasks/{taskID}/artifact/publish", artifactHandler.Publish)
		r.Get("/api/v1/tasks/{taskID}/artifact/latest", artifactHandler.Latest)
		r.Get("/api/v1/tasks/{taskID}/artifacts", artifactHandler.List)
		r.Get("/api/v1/artifacts/{artifactID}/meta", artifactHandler.Meta)
		r.Get("/api/v1/artifacts/{artifactID}", artifactHandler.Serve)

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
				r.Post("/claim", computerHandler.Claim)
				r.Post("/enrollment", computerHandler.CreateEnrollment)
				r.Post("/credential/revoke", computerHandler.RevokeCredential)

				// Computer agents (v1.5)
				r.Get("/agents", computerHandler.ListAgents)
			})
		})

		// Inbox routes (v1.5)
		r.Route("/api/v1/inbox", func(r chi.Router) {
			r.Get("/", inboxHandler.List)
			r.Get("/unread-count", inboxHandler.UnreadCount)
			r.Post("/mark-all-read", inboxHandler.MarkAllRead)
			r.Post("/clear-all", inboxHandler.ClearAll)
			r.Post("/{messageId}/mark-read", inboxHandler.MarkRead)
		})

		// Thread read-status routes (P25-02-B)
		r.Route("/api/v1/threads", func(r chi.Router) {
			r.Post("/{threadID}/mark-read", threadHandler.MarkThreadRead)
			r.Post("/unfollow", threadHandler.UnfollowThread)
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

func defaultAgentWorkspaceRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "agents")
	}
	return filepath.Join(home, ".solo", "agents")
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
