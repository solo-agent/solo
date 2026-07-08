package handler

import (
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/solo-ai/solo/internal/server/service"
)

// AllowedMIMETypes is the set of permitted MIME types for uploads.
var AllowedMIMETypes = map[string]bool{
	// Images
	"image/jpeg":    true,
	"image/png":     true,
	"image/gif":     true,
	"image/webp":    true,
	"image/svg+xml": true,
	// Documents
	"application/pdf":    true,
	"text/plain":         true,
	"text/csv":           true,
	"text/markdown":      true,
	"application/rtf":    true,
	"application/msword": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
	"application/vnd.ms-excel": true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"application/vnd.ms-powerpoint":                                             true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	"application/vnd.oasis.opendocument.text":                                   true,
	"application/vnd.oasis.opendocument.spreadsheet":                            true,
	"application/vnd.oasis.opendocument.presentation":                           true,
	// Common
	"application/json":             true,
	"application/zip":              true,
	"application/x-zip-compressed": true,
	"application/gzip":             true,
	"application/x-gzip":           true,
	"application/x-tar":            true,
	"application/x-7z-compressed":  true,
	"application/vnd.rar":          true,
	"application/x-rar-compressed": true,
}

// MaxAllowedMIMETypes is the cap on the number of distinct types in the
// whitelist (prevents accidental unbounded growth).
const MaxAllowedMIMETypes = 40

// MaxUploadSize is the maximum upload size (50 MB).
const MaxUploadSize = 50 << 20

// AttachmentHandler handles file upload and attachment serving.
type AttachmentHandler struct {
	pool      *pgxpool.Pool
	uploadDir string
}

// NewAttachmentHandler creates a new AttachmentHandler.
func NewAttachmentHandler(pool *pgxpool.Pool, uploadDir string) *AttachmentHandler {
	return &AttachmentHandler{pool: pool, uploadDir: uploadDir}
}

// AttachmentResponse is returned after a successful upload.
type AttachmentResponse struct {
	ID           string `json:"id"`
	Filename     string `json:"filename"`
	MimeType     string `json:"mime_type"`
	Size         int64  `json:"size"`
	URL          string `json:"url"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// Upload handles POST /api/v1/attachments/upload
func (h *AttachmentHandler) Upload(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Limit request body to MaxUploadSize
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadSize)

	if err := r.ParseMultipartForm(MaxUploadSize); err != nil {
		slog.Warn("attachment upload: parse multipart form failed", "error", err)
		writeError(w, http.StatusBadRequest, "file too large or invalid form data")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		slog.Warn("attachment upload: missing file field", "error", err)
		writeError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()

	// Validate MIME type. Some browsers send application/octet-stream for
	// office/archive uploads, so fall back to extension detection when the
	// multipart header is not specific enough.
	mimeType := normalizeMIMEType(header.Header.Get("Content-Type"), header.Filename)
	if !isAllowedMIMEType(mimeType) {
		slog.Warn("attachment upload: disallowed MIME type",
			"filename", header.Filename,
			"mime_type", mimeType,
		)
		writeError(w, http.StatusUnsupportedMediaType,
			fmt.Sprintf("file type %q is not allowed", mimeType))
		return
	}

	// Validate filename (prevent path traversal)
	safeFilename := filepath.Base(header.Filename)
	if safeFilename == "." || safeFilename == ".." || safeFilename == "" {
		writeError(w, http.StatusBadRequest, "invalid filename")
		return
	}

	// Generate unique filename: <uuid>-<original_filename>
	id := uuid.New().String()
	uniqueName := id + "-" + safeFilename

	// Store in date-based subdirectory
	dateDir := time.Now().Format("2006-01")
	saveDir := filepath.Join(h.uploadDir, dateDir)
	if err := os.MkdirAll(saveDir, 0750); err != nil {
		slog.Error("attachment upload: failed to create upload directory",
			"dir", saveDir, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to store file")
		return
	}

	savePath := filepath.Join(saveDir, uniqueName)

	// Save file to disk
	dst, err := os.Create(savePath)
	if err != nil {
		slog.Error("attachment upload: failed to create file",
			"path", savePath, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to store file")
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		slog.Error("attachment upload: failed to write file",
			"path", savePath, "error", err)
		os.Remove(savePath) // clean up partial file
		writeError(w, http.StatusInternalServerError, "failed to write file")
		return
	}

	// Persist metadata to database
	now := time.Now()
	relPath := filepath.Join(dateDir, uniqueName)
	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO attachments (id, user_id, filename, mime_type, size, storage_path, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, userID, safeFilename, mimeType, written, relPath, now,
	)
	if err != nil {
		slog.Error("attachment upload: failed to persist metadata",
			"id", id, "error", err)
		os.Remove(savePath) // clean up file if DB write fails
		writeError(w, http.StatusInternalServerError, "failed to store attachment metadata")
		return
	}

	// Generate thumbnail for raster image uploads (not SVG).
	var thumbnailURL string
	if isRasterImage(mimeType) {
		thumbPath, thumbErr := service.GenerateThumbnail(savePath, 400)
		if thumbErr != nil {
			slog.Warn("attachment upload: thumbnail generation failed",
				"id", id, "mime_type", mimeType, "error", thumbErr)
		} else {
			// Compute relative path for DB storage.
			thumbRel, relErr := filepath.Rel(h.uploadDir, thumbPath)
			if relErr != nil {
				slog.Warn("attachment upload: failed to compute relative thumbnail path",
					"id", id, "thumb_path", thumbPath, "error", relErr)
			} else {
				_, dbErr := h.pool.Exec(r.Context(),
					`UPDATE attachments SET thumbnail_path = $1 WHERE id = $2`,
					thumbRel, id)
				if dbErr != nil {
					slog.Warn("attachment upload: failed to persist thumbnail path",
						"id", id, "error", dbErr)
				} else {
					thumbnailURL = "/api/v1/attachments/" + id + "/thumbnail"
				}
			}
		}
	}

	resp := AttachmentResponse{
		ID:           id,
		Filename:     safeFilename,
		MimeType:     mimeType,
		Size:         written,
		URL:          "/api/v1/attachments/" + id,
		ThumbnailURL: thumbnailURL,
		CreatedAt:    now.Format(time.RFC3339),
	}

	slog.Info("attachment uploaded",
		"id", id,
		"filename", safeFilename,
		"mime_type", mimeType,
		"size", written,
		"user_id", userID,
	)

	writeJSON(w, http.StatusCreated, resp)
}

// Serve handles GET /api/v1/attachments/{attachmentID}
func (h *AttachmentHandler) Serve(w http.ResponseWriter, r *http.Request) {
	// Read the attachment metadata from DB
	var storagePath, mimeType, filename string
	err := h.pool.QueryRow(r.Context(),
		`SELECT storage_path, mime_type, filename FROM attachments WHERE id = $1`,
		r.PathValue("attachmentID"),
	).Scan(&storagePath, &mimeType, &filename)
	if err != nil {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}

	fullPath := filepath.Join(h.uploadDir, storagePath)
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))
	http.ServeFile(w, r, fullPath)
}

// ServeThumbnail handles GET /api/v1/attachments/{attachmentID}/thumbnail
func (h *AttachmentHandler) ServeThumbnail(w http.ResponseWriter, r *http.Request) {
	var thumbnailPath, mimeType string
	err := h.pool.QueryRow(r.Context(),
		`SELECT thumbnail_path, mime_type FROM attachments WHERE id = $1`,
		r.PathValue("attachmentID"),
	).Scan(&thumbnailPath, &mimeType)
	if err != nil || thumbnailPath == "" {
		writeError(w, http.StatusNotFound, "thumbnail not found")
		return
	}

	fullPath := filepath.Join(h.uploadDir, thumbnailPath)
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeFile(w, r, fullPath)
}

// isRasterImage returns true for raster image MIME types that support
// thumbnail generation. SVG is excluded because it's vector-based.
func isRasterImage(mimeType string) bool {
	switch mimeType {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

// isAllowedMIMEType checks whether a MIME type is in the whitelist.
// The whitelist is capped at MaxAllowedMIMETypes entries to prevent
// accidental unbounded growth.
func isAllowedMIMEType(mimeType string) bool {
	if len(AllowedMIMETypes) > MaxAllowedMIMETypes {
		slog.Warn("attachment MIME whitelist exceeded max entries, denying all", "entries", len(AllowedMIMETypes))
		return false
	}
	return AllowedMIMETypes[mimeType]
}

func normalizeMIMEType(raw, filename string) string {
	mimeType := strings.TrimSpace(strings.ToLower(raw))
	if mimeType != "" {
		if parsed, _, err := mime.ParseMediaType(mimeType); err == nil {
			mimeType = parsed
		}
	}

	detected := detectMIMEType(filename)
	if mimeType == "" || mimeType == "application/octet-stream" {
		return detected
	}
	if !isAllowedMIMEType(mimeType) && detected != "application/octet-stream" && isAllowedMIMEType(detected) {
		return detected
	}
	return mimeType
}

// detectMIMEType infers a MIME type from a filename extension.
func detectMIMEType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	case ".txt":
		return "text/plain"
	case ".csv":
		return "text/csv"
	case ".md":
		return "text/markdown"
	case ".rtf":
		return "application/rtf"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xls":
		return "application/vnd.ms-excel"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".ppt":
		return "application/vnd.ms-powerpoint"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".odt":
		return "application/vnd.oasis.opendocument.text"
	case ".ods":
		return "application/vnd.oasis.opendocument.spreadsheet"
	case ".odp":
		return "application/vnd.oasis.opendocument.presentation"
	case ".json":
		return "application/json"
	case ".zip":
		return "application/zip"
	case ".gz":
		return "application/gzip"
	case ".tar":
		return "application/x-tar"
	case ".7z":
		return "application/x-7z-compressed"
	case ".rar":
		return "application/vnd.rar"
	default:
		return "application/octet-stream"
	}
}

// Ensure multipart is imported (used by Upload).
var _ = multipart.ErrMessageTooLarge
