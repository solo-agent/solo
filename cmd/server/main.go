package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/internal/db"
	server "github.com/solo-ai/solo/internal/server"
	"github.com/solo-ai/solo/internal/server/service"
	"github.com/solo-ai/solo/internal/server/ws"
	"github.com/solo-ai/solo/pkg/config"
)

func main() {
	_ = config.LoadDotenv()
	cfg := config.Load()

	// Initialize structured JSON logger
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	slog.Info("starting solo server", "port", cfg.Port)

	// Connect to database
	ctx := context.Background()
	pool, err := db.NewPool(ctx, cfg.DBURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Create WebSocket Hub (agentSvc is set below after it's created)
	hub := ws.NewHub(pool, nil)
	go hub.Run()

	// Create DaemonManager (tracks daemon instances)
	dm := service.NewDaemonManager(pool, hub)
	dm.Start()
	defer dm.Stop()

	// Create AgentService (triggers agent auto-response, manages agent status)
	mentionSvc := service.NewMentionService(pool)
	agentSvc := service.NewAgentService(pool, dm, hub, mentionSvc)

	// Set agent service on hub (was nil during creation due to circular dependency)
	hub.SetAgentService(agentSvc)

	// Start session cleanup goroutine (every 5 minutes)
	go sessionCleanupLoop(pool)

	// Start computer offline checker (every 30 seconds)
	go startOfflineChecker(pool)

	// Create router with all dependencies
	router := server.NewRouter(pool, hub, dm, agentSvc)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	// Start server in goroutine
	go func() {
		slog.Info("server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}

// sessionCleanupLoop periodically removes expired sessions from the database.
// Runs every 5 minutes to prevent the sessions table from accumulating stale rows.
func sessionCleanupLoop(pool *pgxpool.Pool) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		result, err := pool.Exec(context.Background(),
			`DELETE FROM sessions WHERE expires_at < NOW()`)
		if err != nil {
			slog.Error("session cleanup failed", "error", err)
			continue
		}
		if n := result.RowsAffected(); n > 0 {
			slog.Info("expired sessions cleaned up", "count", n)
		}
	}
}

// startOfflineChecker periodically marks computers as offline when their
// heartbeat has not been received for 60+ seconds. Runs every 30 seconds.
func startOfflineChecker(pool *pgxpool.Pool) {
	computerSvc := service.NewComputerService(pool)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if _, err := computerSvc.MarkOffline(context.Background()); err != nil {
			slog.Error("offline computer check failed", "error", err)
		}
	}
}
