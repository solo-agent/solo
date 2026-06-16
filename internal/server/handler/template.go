package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/solo-ai/solo/internal/server/service"
)

type TemplateHandler struct {
	svc  *service.TemplateService
	pool *pgxpool.Pool
}

func NewTemplateHandler(svc *service.TemplateService, pool *pgxpool.Pool) *TemplateHandler {
	return &TemplateHandler{svc: svc, pool: pool}
}

// List handles GET /api/v1/templates
func (h *TemplateHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, name, description, category, icon
		  FROM agent_templates
		 ORDER BY name ASC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	type tmpl struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Category    string `json:"category"`
		Icon        string `json:"icon"`
	}
	out := []tmpl{}
	for rows.Next() {
		var t tmpl
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Category, &t.Icon); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, t)
	}
	writeJSON(w, http.StatusOK, out)
}

// Apply handles POST /api/v1/templates/{id}/apply
func (h *TemplateHandler) Apply(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		OwnerID       string `json:"owner_id"`
		ModelProvider string `json:"model_provider,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "bad json")
		return
	}
	result, err := h.svc.Apply(r.Context(), id, body.OwnerID, body.ModelProvider)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
