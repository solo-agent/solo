package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/solo-ai/solo/internal/server/service"
)

type TemplateHandler struct {
	svc *service.TemplateService
}

func NewTemplateHandler(svc *service.TemplateService) *TemplateHandler {
	return &TemplateHandler{svc: svc}
}

func (h *TemplateHandler) List(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUserID(r); !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	templates, err := h.svc.List(r.Context(), r.URL.Query().Get("lang"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, templates)
}

func (h *TemplateHandler) Get(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUserID(r); !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	template, err := h.svc.Get(r.Context(), chi.URLParam(r, "templateID"), r.URL.Query().Get("lang"))
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "template not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load template")
		return
	}
	writeJSON(w, http.StatusOK, template)
}
