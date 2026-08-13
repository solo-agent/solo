package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"os"
	"os/signal"
	"strconv"
	"strings"
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
	if err := validateProductionConfig(cfg); err != nil {
		slog.Error("invalid production configuration", "error", err)
		os.Exit(1)
	}

	// Initialize structured JSON logger
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	slog.Info("starting solo server", "port", cfg.Port)

	// Connect to database
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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
	dm.SetAgentService(agentSvc)

	// Set agent service on hub (was nil during creation due to circular dependency)
	hub.SetAgentService(agentSvc)

	// Start session cleanup goroutine (every 5 minutes)
	go sessionCleanupLoop(pool)

	// Start agent run watchdogs for ACK/progress visibility.
	go agentSvc.StartAgentRunWatchdogLoop(context.Background())

	// Start computer offline checker (every 30 seconds)
	go startOfflineChecker(pool)

	// Create router with all dependencies
	router := server.NewRouter(ctx, pool, hub, dm, agentSvc)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
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
	cancel()

	slog.Info("shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}

func validateProductionConfig(cfg *config.Config) error {
	if os.Getenv("APP_ENV") != "production" {
		return nil
	}
	secret := strings.TrimSpace(cfg.JWTSecret)
	if len(secret) < 32 || strings.Contains(strings.ToLower(secret), "change") || strings.Contains(strings.ToLower(secret), "replace") {
		return fmt.Errorf("JWT_SECRET must be a non-default secret of at least 32 characters")
	}
	databaseURL, configured := os.LookupEnv("DATABASE_URL")
	if !configured || strings.TrimSpace(databaseURL) == "" || strings.TrimSpace(cfg.DBURL) == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	origins := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if origins == "" || strings.Contains(origins, "*") {
		return fmt.Errorf("CORS_ALLOWED_ORIGINS must list explicit origins")
	}
	if strings.TrimSpace(cfg.PublicURL) == "" || !strings.HasPrefix(cfg.PublicURL, "https://") {
		return fmt.Errorf("PUBLIC_URL must be an https URL")
	}
	switch cfg.AuthMailTransport {
	case "smtp":
		if cfg.SMTPHost == "" || cfg.SMTPFrom == "" {
			return fmt.Errorf("SMTP_HOST and SMTP_FROM are required")
		}
		if _, err := mail.ParseAddress(cfg.SMTPFrom); err != nil {
			return fmt.Errorf("SMTP_FROM must be a valid email address")
		}
		port, err := strconv.Atoi(cfg.SMTPPort)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("SMTP_PORT must be a valid port")
		}
		if cfg.SMTPTLS != "starttls" && cfg.SMTPTLS != "implicit" {
			return fmt.Errorf("SMTP_TLS must be starttls or implicit")
		}
	case "tencent_ses":
		if cfg.TencentCloudSecretID == "" || cfg.TencentCloudSecretKey == "" {
			return fmt.Errorf("TENCENTCLOUD_SECRET_ID and TENCENTCLOUD_SECRET_KEY are required")
		}
		if cfg.TencentSESRegion != "ap-guangzhou" && cfg.TencentSESRegion != "ap-hongkong" {
			return fmt.Errorf("TENCENT_SES_REGION must be ap-guangzhou or ap-hongkong")
		}
		if _, err := mail.ParseAddress(cfg.TencentSESFrom); err != nil {
			return fmt.Errorf("TENCENT_SES_FROM must be a valid email address")
		}
		if cfg.TencentSESTemplateID < 1 {
			return fmt.Errorf("TENCENT_SES_TEMPLATE_ID is required")
		}
	default:
		return fmt.Errorf("AUTH_MAIL_TRANSPORT must be smtp or tencent_ses")
	}
	return nil
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
		if _, err := pool.Exec(context.Background(), `DELETE FROM auth_email_challenges WHERE expires_at < NOW() - interval '1 day'`); err != nil {
			slog.Error("email challenge cleanup failed", "error", err)
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
