package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/solo-ai/solo/internal/server/service"
)

// SkillHandler is the HTTP layer for skill operations.
type SkillHandler struct {
	svc *service.SkillService
}

// NewSkillHandler creates a SkillHandler.
func NewSkillHandler(svc *service.SkillService) *SkillHandler {
	return &SkillHandler{svc: svc}
}

// --- Response DTOs ---

type skillSummaryDTO struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	SourcePath  string    `json:"source_path"`
	SourceKind  string    `json:"source_kind"`
	BodyHash    string    `json:"body_hash"`
	DiscoveredAt time.Time `json:"discovered_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type skillFileDTO struct {
	ID        string    `json:"id"`
	SkillID   string    `json:"skill_id"`
	Path      string    `json:"path"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type skillDetailDTO struct {
	skillSummaryDTO
	Body  string         `json:"body"`
	Files []skillFileDTO `json:"files"`
}

// --- Request DTOs ---

type setAgentSkillsRequest struct {
	SkillIDs []string `json:"skill_ids"`
}

// --- Handlers ---

// ListSkills handles GET /api/v1/skills
func (h *SkillHandler) ListSkills(w http.ResponseWriter, r *http.Request) {
	uid, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	_ = uid // auth gate; everyone sees the same global list (single-tenant)
	skills, err := h.svc.ListAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list skills")
		return
	}
	out := make([]skillSummaryDTO, len(skills))
	for i, s := range skills {
		out[i] = toSummaryDTO(s)
	}
	writeJSON(w, http.StatusOK, out)
}

// GetSkill handles GET /api/v1/skills/{id}
func (h *SkillHandler) GetSkill(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUserID(r); !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	id := chi.URLParam(r, "id")
	sk, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		// ErrSkillNotFound (or pgx.ErrNoRows) -> 404; other errors -> 500.
		if errors.Is(err, service.ErrSkillNotFound) || isNotFound(err) {
			writeError(w, http.StatusNotFound, "skill not found")
			return
		}
		slog.Error("failed to get skill", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get skill")
		return
	}
	dto := skillDetailDTO{
		skillSummaryDTO: toSummaryDTO(sk.SkillSummary),
		Body:            sk.Body,
	}
	for _, f := range sk.Files {
		dto.Files = append(dto.Files, skillFileDTO{
			ID: f.ID, Path: f.Path, Content: f.Content,
			CreatedAt: f.CreatedAt, UpdatedAt: f.UpdatedAt,
		})
	}
	writeJSON(w, http.StatusOK, dto)
}

// RescanSkills handles POST /api/v1/skills/rescan
func (h *SkillHandler) RescanSkills(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUserID(r); !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	res, err := h.svc.Rescan(r.Context())
	if err != nil {
		// Per spec: return 200 + ok:false on rescan failure (server can keep
		// serving even if disk is unwalkable).
		writeJSON(w, http.StatusOK, service.RescanResult{
			OK: false, Error: err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// ListAgentSkills handles GET /api/v1/agents/{agentID}/skills
func (h *SkillHandler) ListAgentSkills(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUserID(r); !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	agentID := chi.URLParam(r, "agentID")
	skills, err := h.svc.ListByAgent(r.Context(), agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list agent skills")
		return
	}
	out := make([]skillSummaryDTO, len(skills))
	for i, s := range skills {
		out[i] = toSummaryDTO(s)
	}
	writeJSON(w, http.StatusOK, out)
}

// SetAgentSkills handles PUT /api/v1/agents/{agentID}/skills (full replace)
func (h *SkillHandler) SetAgentSkills(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUserID(r); !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	agentID := chi.URLParam(r, "agentID")
	var req setAgentSkillsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	skills, err := h.svc.SetAgentSkills(r.Context(), agentID, req.SkillIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]skillSummaryDTO, len(skills))
	for i, s := range skills {
		out[i] = toSummaryDTO(s)
	}
	writeJSON(w, http.StatusOK, out)
}

func toSummaryDTO(s service.SkillSummary) skillSummaryDTO {
	return skillSummaryDTO{
		ID:          s.ID,
		Name:        s.Name,
		Description: s.Description,
		SourcePath:  s.SourcePath,
		SourceKind:  s.SourceKind,
		BodyHash:    s.BodyHash,
		DiscoveredAt: s.DiscoveredAt,
		UpdatedAt:    s.UpdatedAt,
	}
}
