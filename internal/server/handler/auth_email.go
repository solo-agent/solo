package handler

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/solo-ai/solo/internal/auth"
	"github.com/solo-ai/solo/internal/server/service"
	"golang.org/x/crypto/bcrypt"
)

const (
	challengeRegister      = "register"
	challengePasswordReset = "password_reset"
	challengeTTL           = 10 * time.Minute
	challengeCooldown      = time.Minute
	challengeMaxAttempts   = 5
)

var errEmailCooldown = errors.New("email challenge cooldown")

type verifyRegistrationRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type resetPasswordRequest struct {
	Email       string `json:"email"`
	Code        string `json:"code"`
	NewPassword string `json:"new_password"`
}

type emailChallenge struct {
	ID           string
	CodeHash     string
	DisplayName  *string
	PasswordHash *string
	Attempts     int
	ExpiresAt    time.Time
}

func (h *AuthHandler) PublicConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{
		"signup_available":   h.cfg.AllowSignup || len(h.cfg.AllowedEmails) > 0 || len(h.cfg.AllowedEmailDomains) > 0,
		"email_verification": !localRegistrationAutoVerify(r),
	})
}

func localRegistrationAutoVerify(r *http.Request) bool {
	enabled, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("SOLO_DEV_AUTO_VERIFY_LOCAL")))
	if err != nil || !enabled || os.Getenv("APP_ENV") == "production" {
		return false
	}
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !isLoopbackHost(remoteHost) || !isLoopbackHost(r.Host) {
		return false
	}
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		parsed, err := url.Parse(origin)
		if err != nil || !isLoopbackHost(parsed.Host) {
			return false
		}
	}
	for _, header := range []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Real-IP"} {
		for _, value := range strings.Split(r.Header.Get(header), ",") {
			if value = strings.TrimSpace(value); value != "" && !isLoopbackHost(value) {
				return false
			}
		}
	}
	// A standardized Forwarded header is intentionally not trusted for this
	// development-only capability. Local direct browser/API requests do not set it.
	return strings.TrimSpace(r.Header.Get("Forwarded")) == ""
}

func isLoopbackHost(raw string) bool {
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	} else {
		host = strings.Trim(host, "[]")
	}
	return host == "localhost" || (net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback())
}

func normalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || len(email) > 255 {
		return "", errors.New("invalid email format")
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || !strings.Contains(email, "@") {
		return "", errors.New("invalid email format")
	}
	return email, nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	if len([]byte(password)) > 72 {
		return errors.New("password must be at most 72 bytes")
	}
	return nil
}

func validateRegistration(req RegisterRequest) (string, string, error) {
	email, err := normalizeEmail(req.Email)
	if err != nil {
		return "", "", err
	}
	if err := validatePassword(req.Password); err != nil {
		return "", "", err
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = email[:strings.LastIndex(email, "@")]
	}
	if utf8.RuneCountInString(displayName) > 100 {
		return "", "", errors.New("display name must be at most 100 characters")
	}
	return email, displayName, nil
}

func (h *AuthHandler) signupAllowed(email string) bool {
	for _, allowed := range h.cfg.AllowedEmails {
		if email == allowed {
			return true
		}
	}
	domain := email[strings.LastIndex(email, "@")+1:]
	for _, allowed := range h.cfg.AllowedEmailDomains {
		if domain == strings.TrimPrefix(allowed, "@") {
			return true
		}
	}
	if len(h.cfg.AllowedEmails) > 0 || len(h.cfg.AllowedEmailDomains) > 0 {
		return false
	}
	return h.cfg.AllowSignup
}

func verificationCode() (string, error) {
	if os.Getenv("APP_ENV") != "production" {
		if fixed := strings.TrimSpace(os.Getenv("SOLO_DEV_AUTH_CODE")); len(fixed) == 6 {
			if _, ok := new(big.Int).SetString(fixed, 10); ok {
				return fixed, nil
			}
		}
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func codeHash(secret, email, purpose, code string) string {
	digest := hmac.New(sha256.New, []byte(secret))
	digest.Write([]byte(email))
	digest.Write([]byte{0})
	digest.Write([]byte(purpose))
	digest.Write([]byte{0})
	digest.Write([]byte(code))
	return hex.EncodeToString(digest.Sum(nil))
}

func (h *AuthHandler) createEmailChallenge(ctx context.Context, email, purpose, displayName, passwordHash string) (string, string, error) {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback(ctx)
	lockKey := email + "|" + purpose
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return "", "", err
	}
	var latest time.Time
	err = tx.QueryRow(ctx, `SELECT created_at FROM auth_email_challenges WHERE email = $1 AND purpose = $2 ORDER BY created_at DESC LIMIT 1`, email, purpose).Scan(&latest)
	if err == nil && time.Since(latest) < challengeCooldown {
		return "", "", errEmailCooldown
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", "", err
	}
	code, err := verificationCode()
	if err != nil {
		return "", "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE auth_email_challenges SET used_at = now() WHERE email = $1 AND purpose = $2 AND used_at IS NULL`, email, purpose); err != nil {
		return "", "", err
	}
	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO auth_email_challenges (email, purpose, code_hash, display_name, password_hash, expires_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6)
		RETURNING id`, email, purpose, codeHash(h.cfg.JWTSecret, email, purpose, code), displayName, passwordHash, time.Now().Add(challengeTTL)).Scan(&id)
	if err != nil {
		return "", "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", err
	}
	return id, code, nil
}

func (h *AuthHandler) invalidateEmailChallenge(ctx context.Context, id string) {
	_, _ = h.pool.Exec(ctx, `UPDATE auth_email_challenges SET used_at = now() WHERE id = $1`, id)
}

func (h *AuthHandler) lockEmailChallenge(ctx context.Context, tx pgx.Tx, email, purpose, code string) (emailChallenge, error) {
	var challenge emailChallenge
	err := tx.QueryRow(ctx, `
		SELECT id, code_hash, display_name, password_hash, attempts, expires_at
		FROM auth_email_challenges
		WHERE email = $1 AND purpose = $2 AND used_at IS NULL
		ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, email, purpose).
		Scan(&challenge.ID, &challenge.CodeHash, &challenge.DisplayName, &challenge.PasswordHash, &challenge.Attempts, &challenge.ExpiresAt)
	if err != nil {
		return challenge, errors.New("invalid or expired verification code")
	}
	actual := codeHash(h.cfg.JWTSecret, email, purpose, strings.TrimSpace(code))
	valid := time.Now().Before(challenge.ExpiresAt) && challenge.Attempts < challengeMaxAttempts && subtle.ConstantTimeCompare([]byte(actual), []byte(challenge.CodeHash)) == 1
	if !valid {
		_, _ = tx.Exec(ctx, `UPDATE auth_email_challenges SET attempts = attempts + 1 WHERE id = $1`, challenge.ID)
		return challenge, errors.New("invalid or expired verification code")
	}
	return challenge, nil
}

func (h *AuthHandler) VerifyRegistration(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var req verifyRegistrationRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email, err := normalizeEmail(req.Email)
	if err != nil || len(strings.TrimSpace(req.Code)) != 6 {
		writeError(w, http.StatusBadRequest, "invalid or expired verification code")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to verify email")
		return
	}
	defer tx.Rollback(r.Context())
	challenge, err := h.lockEmailChallenge(r.Context(), tx, email, challengeRegister, req.Code)
	if err != nil {
		_ = tx.Commit(r.Context())
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if challenge.DisplayName == nil || challenge.PasswordHash == nil {
		writeError(w, http.StatusBadRequest, "invalid or expired verification code")
		return
	}
	var userID string
	var createdAt time.Time
	err = tx.QueryRow(r.Context(), `
		INSERT INTO users (email, display_name, password_hash, email_verified_at)
		VALUES ($1, $2, $3, now()) RETURNING id, created_at`, email, *challenge.DisplayName, *challenge.PasswordHash).Scan(&userID, &createdAt)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE auth_email_challenges SET used_at = now() WHERE id = $1`, challenge.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to verify email")
		return
	}
	if _, err := tx.Exec(r.Context(), `
			INSERT INTO workspace_members (workspace_id, user_id, role)
			VALUES ('00000000-0000-0000-0000-000000000001', $1, $2)`, userID, service.PublicWorkspaceRole(email)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to join public Workspace")
		return
	}
	if _, err := tx.Exec(r.Context(), `
			INSERT INTO channels (id, workspace_id, name, description, type, created_by)
			SELECT '00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001',
			       'general', 'Public lobby for everyone on Solo', 'channel', $1
			 WHERE NOT EXISTS (
			       SELECT 1 FROM channels
			        WHERE workspace_id = '00000000-0000-0000-0000-000000000001'
			          AND name = 'general' AND type = 'channel' AND is_archived = false
			 )
			ON CONFLICT (id) DO NOTHING`, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to initialize public Workspace")
		return
	}
	if _, err := tx.Exec(r.Context(), `
			INSERT INTO channel_members (channel_id, member_type, member_id, role)
			SELECT id, 'user', $1, 'member'
			  FROM channels
			 WHERE workspace_id = '00000000-0000-0000-0000-000000000001'
			   AND type = 'channel' AND is_archived = false
			ON CONFLICT DO NOTHING`, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to join public Channel")
		return
	}
	if _, err := tx.Exec(r.Context(), `
			WITH accepted_invitations AS (
				UPDATE workspace_invitations
				   SET accepted_by = $1, accepted_at = now()
				 WHERE lower(email) = lower($2) AND accepted_at IS NULL AND expires_at > now()
				 RETURNING workspace_id, role
			), membership_sources AS (
				SELECT ai.workspace_id, ai.role FROM accepted_invitations ai
				JOIN workspaces w ON w.id=ai.workspace_id AND w.deleted_at IS NULL
				UNION ALL
				SELECT r.workspace_id, r.role FROM workspace_join_rules r
				JOIN workspaces w ON w.id=r.workspace_id AND w.deleted_at IS NULL
				 WHERE (rule_type='email' AND lower(value)=lower($2))
				    OR (rule_type='domain' AND lower(value)=split_part(lower($2),'@',2))
			), eligible AS (
				SELECT workspace_id,
				       CASE WHEN bool_or(role='admin') THEN 'admin' ELSE 'member' END AS role
				  FROM membership_sources
				 GROUP BY workspace_id
			), joined AS (
				INSERT INTO workspace_members (workspace_id, user_id, role)
				SELECT workspace_id, $1, role FROM eligible
				ON CONFLICT (workspace_id, user_id) DO UPDATE SET role=EXCLUDED.role
				RETURNING workspace_id
			)
			INSERT INTO channel_members (channel_id, member_type, member_id, role)
			SELECT c.id, 'user', $1, 'member'
			  FROM joined j
			  JOIN channels c ON c.workspace_id=j.workspace_id
			                 AND c.type='channel' AND c.is_archived=false
			ON CONFLICT DO NOTHING`, userID, email); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to accept Workspace invitations")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE users SET onboarding_completed_at=now()
		 WHERE id=$1 AND EXISTS (
			SELECT 1 FROM workspace_members wm
			JOIN workspaces w ON w.id=wm.workspace_id
			WHERE wm.user_id=$1 AND w.is_default=false AND w.deleted_at IS NULL
		)`, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to finish invited registration")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to verify email")
		return
	}
	response, err := h.newUserAuthResponse(r.Context(), userID, email, *challenge.DisplayName, createdAt, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	slog.Info("user registered", "user_id", userID, "email", email)
	writeJSON(w, http.StatusCreated, response)
}

func (h *AuthHandler) newUserAuthResponse(ctx context.Context, userID, email, displayName string, createdAt time.Time, workspaceID string) (AuthResponse, error) {
	accessToken, err := auth.GenerateAccessToken(userID, email, displayName)
	if err != nil {
		return AuthResponse{}, err
	}
	refreshToken, err := auth.GenerateRefreshToken()
	if err != nil {
		return AuthResponse{}, err
	}
	if _, err := h.pool.Exec(ctx, `INSERT INTO sessions (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`, userID, auth.HashToken(refreshToken), time.Now().Add(auth.RefreshTokenDuration)); err != nil {
		return AuthResponse{}, err
	}
	var channelID string
	if workspaceID != "" {
		channelID = h.bootstrapOnboarding(ctx, userID, displayName, email, workspaceID)
	}
	return AuthResponse{
		AccessToken: accessToken, RefreshToken: refreshToken, ExpiresIn: int64(auth.AccessTokenDuration.Seconds()), OnboardingChannelID: channelID, WorkspaceID: workspaceID,
		User: UserResponse{ID: userID, Email: email, DisplayName: displayName, Role: "member", CreatedAt: createdAt.Format(time.RFC3339)},
	}, nil
}

func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var req forgotPasswordRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email, emailErr := normalizeEmail(req.Email)
	generic := map[string]any{"message": "if the account exists, a reset code has been sent", "expires_in": 600, "retry_after": 60}
	if emailErr != nil {
		writeJSON(w, http.StatusAccepted, generic)
		return
	}
	var exists bool
	if err := h.pool.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM users WHERE email = $1 AND is_active = true AND email_verified_at IS NOT NULL)`, email).Scan(&exists); err != nil || !exists {
		writeJSON(w, http.StatusAccepted, generic)
		return
	}
	id, code, err := h.createEmailChallenge(r.Context(), email, challengePasswordReset, "", "")
	if errors.Is(err, errEmailCooldown) {
		writeJSON(w, http.StatusAccepted, generic)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to request password reset")
		return
	}
	if err := h.mail.SendCode(r.Context(), email, code, challengePasswordReset); err != nil {
		h.invalidateEmailChallenge(r.Context(), id)
		slog.Error("failed to send password reset code", "error", err)
		writeError(w, http.StatusServiceUnavailable, "email delivery is temporarily unavailable")
		return
	}
	writeJSON(w, http.StatusAccepted, generic)
}

func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 12<<10)
	var req resetPasswordRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email, err := normalizeEmail(req.Email)
	if err != nil || len(strings.TrimSpace(req.Code)) != 6 {
		writeError(w, http.StatusBadRequest, "invalid or expired verification code")
		return
	}
	if err := validatePassword(req.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset password")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset password")
		return
	}
	defer tx.Rollback(r.Context())
	challenge, err := h.lockEmailChallenge(r.Context(), tx, email, challengePasswordReset, req.Code)
	if err != nil {
		_ = tx.Commit(r.Context())
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := tx.Exec(r.Context(), `UPDATE users SET password_hash = $1, updated_at = now() WHERE email = $2 AND is_active = true AND email_verified_at IS NOT NULL`, string(passwordHash), email)
	if err != nil || result.RowsAffected() != 1 {
		writeError(w, http.StatusBadRequest, "invalid or expired verification code")
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE auth_email_challenges SET used_at = now() WHERE id = $1`, challenge.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset password")
		return
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM sessions WHERE user_id = (SELECT id FROM users WHERE email = $1)`, email); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset password")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset password")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
