package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/internal/auth"
	"github.com/solo-ai/solo/internal/i18n"
	"github.com/solo-ai/solo/internal/server/service"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler handles authentication-related HTTP requests.
type AuthHandler struct {
	pool *pgxpool.Pool
	svc  *service.ChannelService
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(pool *pgxpool.Pool) *AuthHandler {
	return &AuthHandler{pool: pool, svc: service.NewChannelService(pool)}
}

// --- Request/Response types ---

type RegisterRequest struct {
	Email           string `json:"email"`
	Password        string `json:"password"`
	DisplayName     string `json:"display_name,omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type AuthResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int64        `json:"expires_in"`
	User         UserResponse `json:"user"`
}

type UserResponse struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	CreatedAt   string `json:"created_at"`
}

// Register handles POST /api/v1/auth/register
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	password := req.Password
	displayName := strings.TrimSpace(req.DisplayName)

	// Validate input
	if email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, "invalid email format")
		return
	}
	if len(password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if displayName == "" {
		// Default display name to email local part
		displayName = email[:strings.Index(email, "@")]
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		slog.Error("failed to hash password", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to register user")
		return
	}

	// Insert user
	var userID string
	var createdAt time.Time
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO users (email, display_name, password_hash)
		 VALUES ($1, $2, $3)
		 RETURNING id, created_at`,
		email, displayName, string(hashedPassword),
	).Scan(&userID, &createdAt)

	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		slog.Error("failed to create user", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to register user")
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
		// Non-fatal: user is created, they can retry login
	}

	slog.Info("user registered", "user_id", userID, "email", email)

	// Auto-create welcome channel for new user (best-effort)
	h.createWelcomeChannel(r.Context(), userID, displayName)

	writeJSON(w, http.StatusCreated, AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(auth.AccessTokenDuration.Seconds()),
		User: UserResponse{
			ID:          userID,
			Email:       email,
			DisplayName: displayName,
			Role:        "member",
			CreatedAt:   createdAt.Format(time.RFC3339),
		},
	})
}

// Login handles POST /api/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
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
	var createdAt time.Time
	err := h.pool.QueryRow(r.Context(),
		`SELECT id, display_name, password_hash, role, created_at
		 FROM users WHERE email = $1 AND is_active = true`,
		email,
	).Scan(&userID, &displayName, &passwordHash, &role, &createdAt)

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

	// Delete old session (single-use refresh token)
	_, _ = h.pool.Exec(r.Context(),
		`DELETE FROM sessions WHERE token_hash = $1`, tokenHash)

	// Issue new tokens
	newAccessToken, err := auth.GenerateAccessToken(userID, email, displayName)
	if err != nil {
		slog.Error("failed to generate access token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	newRefreshToken, err := auth.GenerateRefreshToken()
	if err != nil {
		slog.Error("failed to generate refresh token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	// Store new session
	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO sessions (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		userID, auth.HashToken(newRefreshToken), time.Now().Add(auth.RefreshTokenDuration),
	)
	if err != nil {
		slog.Error("failed to store new session", "error", err)
	}

	writeJSON(w, http.StatusOK, AuthResponse{
		AccessToken:  newAccessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    int64(auth.AccessTokenDuration.Seconds()),
	})
}

// createWelcomeChannel creates a "welcome" channel for a newly registered user.
// This is best-effort — failures are logged but not returned to the client.
func (h *AuthHandler) createWelcomeChannel(ctx context.Context, userID, displayName string) {
	// Use user-specific suffix to avoid unique constraint collision across users.
	shortID := userID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	channelName := "welcome-" + shortID
	channelDesc := i18n.Active.DefaultChannelDesc
	if displayName == "" {
		displayName = i18n.Active.DefaultDisplayName
	}

	_, err := h.svc.CreateChannel(ctx, channelName, channelDesc, "channel", userID)
	if err != nil {
		slog.Warn("failed to create welcome channel for new user",
			"user_id", userID,
			"error", err,
		)
	}
}
