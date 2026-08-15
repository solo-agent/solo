package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/internal/auth"
	"github.com/solo-ai/solo/internal/authmail"
	"github.com/solo-ai/solo/internal/server/onboarding"
	"github.com/solo-ai/solo/internal/server/service"
	"github.com/solo-ai/solo/pkg/config"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler handles authentication-related HTTP requests.
type AuthHandler struct {
	pool     *pgxpool.Pool
	svc      *service.ChannelService
	agentSvc *service.AgentService // optional: nil in tests
	cfg      *config.Config
	mail     *authmail.Sender
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(pool *pgxpool.Pool, agentSvc *service.AgentService) *AuthHandler {
	cfg := config.Load()
	return &AuthHandler{pool: pool, svc: service.NewChannelService(pool), agentSvc: agentSvc, cfg: cfg, mail: authmail.NewSender(cfg)}
}

// --- Request/Response types ---

type RegisterRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name,omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type AuthResponse struct {
	AccessToken         string       `json:"access_token"`
	RefreshToken        string       `json:"refresh_token"`
	ExpiresIn           int64        `json:"expires_in"`
	User                UserResponse `json:"user"`
	OnboardingChannelID string       `json:"onboarding_channel_id,omitempty"`
	WorkspaceID         string       `json:"workspace_id,omitempty"`
}

type UserResponse struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
	Role        string  `json:"role"`
	CreatedAt   string  `json:"created_at"`
}

// Register validates a pending account and sends its email verification code.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	email, displayName, err := validateRegistration(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var exists bool
	if err := h.pool.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM users WHERE email = $1)`, email).Scan(&exists); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to register user")
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "email already registered")
		return
	}
	if !h.signupAllowed(email) {
		writeError(w, http.StatusForbidden, "registration is not available for this email")
		return
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to register user")
		return
	}
	challengeID, code, err := h.createEmailChallenge(r.Context(), email, challengeRegister, displayName, string(hashedPassword))
	if errors.Is(err, errEmailCooldown) {
		writeError(w, http.StatusTooManyRequests, "please wait before requesting another code")
		return
	}
	if err != nil {
		slog.Error("failed to create registration challenge", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to send verification code")
		return
	}
	if localRegistrationAutoVerify(r) {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"message": "registration ready", "email": email, "expires_in": 600, "retry_after": 60,
			"email_verification": false, "verification_code": code,
		})
		return
	}
	if err := h.mail.SendCode(r.Context(), email, code, challengeRegister); err != nil {
		h.invalidateEmailChallenge(r.Context(), challengeID)
		slog.Error("failed to send registration code", "error", err)
		writeError(w, http.StatusServiceUnavailable, "email delivery is temporarily unavailable")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"message": "verification code sent", "email": email, "expires_in": 600, "retry_after": 60,
		"email_verification": true,
	})
}

// Login handles POST /api/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	password := req.Password

	if email == "" || password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	// Query user
	var userID, displayName, passwordHash, role string
	var avatarURL *string
	var createdAt time.Time
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, display_name, password_hash, role, created_at, avatar_url
		 FROM users WHERE email = $1 AND is_active = true AND email_verified_at IS NOT NULL`,
		email,
	).Scan(&userID, &displayName, &passwordHash, &role, &createdAt, &avatarURL)

	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		slog.Error("failed to query user", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	// Issue tokens
	accessToken, err := auth.GenerateAccessToken(userID, email, displayName)
	if err != nil {
		slog.Error("failed to generate access token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	refreshToken, err := auth.GenerateRefreshToken()
	if err != nil {
		slog.Error("failed to generate refresh token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	// Store refresh token session
	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO sessions (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		userID, auth.HashToken(refreshToken), time.Now().Add(auth.RefreshTokenDuration),
	)
	if err != nil {
		slog.Error("failed to store session", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	slog.Info("user logged in", "user_id", userID, "email", email)

	writeJSON(w, http.StatusOK, AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(auth.AccessTokenDuration.Seconds()),
		User: UserResponse{
			ID:          userID,
			Email:       email,
			DisplayName: displayName,
			AvatarURL:   avatarURL,
			Role:        role,
			CreatedAt:   createdAt.Format(time.RFC3339),
		},
	})
}

// Logout handles POST /api/v1/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Delete all sessions for this user
	_, err := h.pool.Exec(r.Context(),
		`DELETE FROM sessions WHERE user_id = $1`, userID)
	if err != nil {
		slog.Error("failed to delete sessions", "error", err)
	}

	slog.Info("user logged out", "user_id", userID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

// Refresh handles POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	refreshToken := strings.TrimSpace(req.RefreshToken)
	if refreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	tokenHash := auth.HashToken(refreshToken)

	// Find and validate session
	var userID, email, displayName string
	var expiresAt time.Time
	err := h.pool.QueryRow(r.Context(),
		`SELECT s.user_id, u.email, u.display_name, s.expires_at
		 FROM sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = $1 AND u.is_active = true`,
		tokenHash,
	).Scan(&userID, &email, &displayName, &expiresAt)

	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusUnauthorized, "invalid or expired refresh token")
			return
		}
		slog.Error("failed to query session", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if time.Now().After(expiresAt) {
		// Clean up expired session
		_, _ = h.pool.Exec(r.Context(),
			`DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
		writeError(w, http.StatusUnauthorized, "refresh token expired")
		return
	}

	// Keep the refresh token stable for the lifetime of this database session.
	// Rotating it here made concurrent browser requests race: the first refresh
	// deleted the token while another tab/request was still using it, which
	// forced an otherwise valid user back to the login page.
	newAccessToken, err := auth.GenerateAccessToken(userID, email, displayName)
	if err != nil {
		slog.Error("failed to generate access token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	writeJSON(w, http.StatusOK, AuthResponse{
		AccessToken:  newAccessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(auth.AccessTokenDuration.Seconds()),
	})
}

func onboardingUniqueName(base, userID string) string {
	suffix := userID
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	name := strings.TrimSpace(base)
	if len([]rune(name)) > 91 {
		name = string([]rune(name)[:91])
	}
	return name + "-" + suffix
}

// bootstrapOnboarding creates the pinned Lucy Channel for a newly registered user
// and inserts a wizard welcome message. Lucy agent creation is deferred to the
// onboarding wizard (POST /api/v1/onboarding/create-lucy) so the user can select
// a runtime first.
// Returns the Channel ID for clients that need to initialize Lucy.
// All failures are logged but not returned — registration succeeds regardless.
func (h *AuthHandler) bootstrapOnboarding(ctx context.Context, userID, displayName, email, workspaceID string) string {
	channelName := onboarding.OnboardingChannelName(displayName)
	channelDesc := "Your pinned Channel with Lucy, Solo's steward."

	// Step 1: Create the pinned Lucy Channel.
	channelID, err := h.svc.CreateChannelInWorkspace(ctx, channelName, channelDesc, "lucy", userID, workspaceID)
	if errors.Is(err, service.ErrChannelNameExists) {
		channelName = onboardingUniqueName(channelName, userID)
		channelID, err = h.svc.CreateChannelInWorkspace(ctx, channelName, channelDesc, "lucy", userID, workspaceID)
	}
	if err != nil {
		slog.Warn("onboarding: failed to create channel",
			"user_id", userID, "channel_name", channelName, "error", err)
		return ""
	}

	// Step 1b: Preserve the per-user #all Channel. New Agents are not auto-added.
	h.ensureAllChannel(ctx, userID, displayName, workspaceID)

	// Step 2: Insert a wizard welcome message.
	msgID := uuid.New().String()
	welcomeMsg := onboarding.WizardWelcomePrompt(displayName)
	now := time.Now()
	_, err = h.pool.Exec(ctx,
		`INSERT INTO messages (id, channel_id, sender_type, sender_id, content, content_type, created_at, updated_at)
		 VALUES ($1, $2, 'system', '00000000-0000-0000-0000-000000000000', $3, 'system', $4, $4)`,
		msgID, channelID, welcomeMsg, now,
	)
	if err != nil {
		slog.Warn("onboarding: failed to insert welcome message",
			"channel_id", channelID, "error", err)
		return channelID
	}

	slog.Info("onboarding: channel created, awaiting wizard",
		"user_id", userID, "channel_id", channelID, "channel_name", channelName)

	return channelID
}

// ensureAllChannel preserves the current per-user #all-{name} Channel.
// New Channel-scoped Agents and Lucy are intentionally not auto-added.
func (h *AuthHandler) ensureAllChannel(ctx context.Context, userID, displayName, workspaceID string) {
	channelName := "all-" + onboarding.SanitizeDisplayName(displayName)
	_, err := h.svc.CreateChannelInWorkspace(ctx, channelName, "All your agents and members", "channel", userID, workspaceID)
	if errors.Is(err, service.ErrChannelNameExists) {
		channelName = onboardingUniqueName(channelName, userID)
		_, err = h.svc.CreateChannelInWorkspace(ctx, channelName, "All your agents and members", "channel", userID, workspaceID)
	}
	if err != nil {
		slog.Debug("onboarding: #all channel exists", "user_id", userID, "channel", channelName, "error", err)
	} else {
		slog.Info("onboarding: #all channel created", "user_id", userID, "channel", channelName)
	}
}
