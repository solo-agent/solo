package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	userAvatarPresetPrefix  = "dicebear:pixel-art:"
	userAvatarPresetCount   = 12
	maxUserAvatarUploadSize = 5 << 20
)

type UpdateUserRequest struct {
	DisplayName *string `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

// CurrentUser returns mutable profile data from the database rather than the JWT snapshot.
func (h *AuthHandler) CurrentUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	user, err := h.currentUser(r.Context(), userID)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		slog.Error("failed to load current user", "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load user")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// UpdateCurrentUser updates the current user's display name and/or avatar.
func (h *AuthHandler) UpdateCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DisplayName == nil && req.AvatarURL == nil {
		writeError(w, http.StatusBadRequest, "no profile changes provided")
		return
	}

	displayName := ""
	if req.DisplayName != nil {
		displayName = strings.TrimSpace(*req.DisplayName)
		if displayName == "" {
			writeError(w, http.StatusBadRequest, "display_name is required")
			return
		}
		if utf8.RuneCountInString(displayName) > 50 {
			writeError(w, http.StatusBadRequest, "display_name cannot exceed 50 characters")
			return
		}
	}

	avatarURL := ""
	if req.AvatarURL != nil {
		avatarURL = strings.TrimSpace(*req.AvatarURL)
		if err := h.validateUserAvatar(r.Context(), userID, avatarURL); err != nil {
			if _, ok := err.(validationError); ok {
				writeError(w, http.StatusBadRequest, err.Error())
			} else {
				slog.Error("failed to validate user avatar", "user_id", userID, "error", err)
				writeError(w, http.StatusInternalServerError, "failed to validate avatar")
			}
			return
		}
	}

	var user UserResponse
	var createdAt time.Time
	err := h.pool.QueryRow(r.Context(),
		`UPDATE users
		    SET display_name = CASE WHEN $2 THEN $3 ELSE display_name END,
		        avatar_url = CASE WHEN $4 THEN NULLIF($5, '') ELSE avatar_url END,
		        updated_at = now()
		  WHERE id = $1 AND is_active = true
		  RETURNING id, email, display_name, avatar_url, role, created_at`,
		userID, req.DisplayName != nil, displayName, req.AvatarURL != nil, avatarURL,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.AvatarURL, &user.Role, &createdAt)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		slog.Error("failed to update current user", "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}
	user.CreatedAt = createdAt.Format(time.RFC3339)
	writeJSON(w, http.StatusOK, user)
}

func (h *AuthHandler) currentUser(ctx context.Context, userID string) (*UserResponse, error) {
	var user UserResponse
	var createdAt time.Time
	err := h.pool.QueryRow(ctx,
		`SELECT id, email, display_name, avatar_url, role, created_at
		   FROM users
		  WHERE id = $1 AND is_active = true`,
		userID,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.AvatarURL, &user.Role, &createdAt)
	if err != nil {
		return nil, err
	}
	user.CreatedAt = createdAt.Format(time.RFC3339)
	return &user, nil
}

func (h *AuthHandler) validateUserAvatar(ctx context.Context, userID, avatarURL string) error {
	if avatarURL == "" {
		return nil
	}
	if isUserAvatarPreset(avatarURL) {
		return nil
	}

	attachmentID, ok := userAvatarAttachmentID(avatarURL)
	if !ok {
		return newValidationError("avatar_url must be a Solo avatar preset or an uploaded image")
	}

	var valid bool
	err := h.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1
			  FROM attachments
			 WHERE id = $1
			   AND user_id = $2
			   AND mime_type IN ('image/jpeg', 'image/png', 'image/webp')
			   AND size <= $3
		)`,
		attachmentID, userID, maxUserAvatarUploadSize,
	).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return newValidationError("avatar image must be your JPEG, PNG, or WebP upload under 5 MB")
	}
	return nil
}

func isUserAvatarPreset(value string) bool {
	if !strings.HasPrefix(value, userAvatarPresetPrefix) {
		return false
	}
	index, err := strconv.Atoi(strings.TrimPrefix(value, userAvatarPresetPrefix))
	return err == nil && index >= 0 && index < userAvatarPresetCount
}

func userAvatarAttachmentID(value string) (string, bool) {
	const prefix = "/api/v1/attachments/"
	if !strings.HasPrefix(value, prefix) {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(value, prefix), "/")
	if len(parts) == 2 && parts[1] != "thumbnail" {
		return "", false
	}
	if len(parts) < 1 || len(parts) > 2 {
		return "", false
	}
	id, err := uuid.Parse(parts[0])
	if err != nil {
		return "", false
	}
	return id.String(), true
}

type validationError string

func (e validationError) Error() string { return string(e) }

func newValidationError(message string) error { return validationError(message) }
