package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/solo-ai/solo/internal/server/service"
)

type LarkHandler struct{ svc *service.LarkService }

func NewLarkHandler(svc *service.LarkService) *LarkHandler { return &LarkHandler{svc: svc} }

type saveLarkBindingRequest struct {
	ChannelID         string `json:"channel_id"`
	AgentID           string `json:"agent_id"`
	Platform          string `json:"platform"`
	AppID             string `json:"app_id"`
	AppSecret         string `json:"app_secret"`
	VerificationToken string `json:"verification_token"`
}

type larkBindingResponse struct {
	*service.LarkBinding
	CallbackURL string `json:"callback_url,omitempty"`
}

func (h *LarkHandler) Get(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.authorizeAdmin(w, r)
	if !ok {
		return
	}
	_ = userID
	binding, err := h.svc.GetBinding(r.Context(), workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load Lark connection")
		return
	}
	response := larkBindingResponse{LarkBinding: binding}
	if binding.ConnectionMode == "callback" {
		response.CallbackURL = callbackURL(r, binding.ID, h.svc.CallbackSignature(binding.ID))
	}
	writeJSON(w, http.StatusOK, response)
}

type startLarkRegistrationRequest struct {
	ChannelID string `json:"channel_id"`
	AgentID   string `json:"agent_id"`
	Platform  string `json:"platform"`
}

func (h *LarkHandler) StartRegistration(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.authorizeAdmin(w, r)
	if !ok {
		return
	}
	var req startLarkRegistrationRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil || strings.TrimSpace(req.ChannelID) == "" || strings.TrimSpace(req.AgentID) == "" {
		writeError(w, http.StatusBadRequest, "channel and Agent are required")
		return
	}
	session, err := h.svc.StartRegistration(r.Context(), service.StartLarkRegistrationInput{
		WorkspaceID: workspaceID, Platform: req.Platform, ChannelID: req.ChannelID,
		AgentID: req.AgentID, UserID: userID,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

func (h *LarkHandler) RegistrationStatus(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.authorizeAdmin(w, r)
	if !ok {
		return
	}
	session, err := h.svc.RegistrationStatus(workspaceID, chi.URLParam(r, "sessionID"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (h *LarkHandler) Save(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.authorizeAdmin(w, r)
	if !ok {
		return
	}
	var req saveLarkBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.ChannelID) == "" || strings.TrimSpace(req.AgentID) == "" || strings.TrimSpace(req.AppID) == "" {
		writeError(w, http.StatusBadRequest, "channel, Agent and App ID are required")
		return
	}
	binding, err := h.svc.SaveBinding(r.Context(), service.SaveLarkBindingInput{
		WorkspaceID: workspaceID, ChannelID: req.ChannelID, AgentID: req.AgentID,
		Platform: req.Platform, AppID: req.AppID, AppSecret: req.AppSecret,
		VerificationToken: req.VerificationToken, UserID: userID,
	})
	if err != nil {
		status := http.StatusBadRequest
		if service.IsUniqueViolation(err) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, larkBindingResponse{LarkBinding: binding, CallbackURL: callbackURL(r, binding.ID, h.svc.CallbackSignature(binding.ID))})
}

func (h *LarkHandler) Delete(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.authorizeAdmin(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteBinding(r.Context(), workspaceID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to disconnect Lark")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *LarkHandler) Retry(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.authorizeAdmin(w, r)
	if !ok {
		return
	}
	binding, err := h.svc.GetBinding(r.Context(), workspaceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Lark connection not found")
		return
	}
	count, err := h.svc.RetryFailed(r.Context(), binding.ID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "retry failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"retried": count})
}

func (h *LarkHandler) Events(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
	var payload struct {
		Challenge string `json:"challenge"`
		Token     string `json:"token"`
		Header    struct {
			EventID string `json:"event_id"`
			Token   string `json:"token"`
		} `json:"header"`
		Event struct {
			Sender struct {
				SenderID struct {
					OpenID string `json:"open_id"`
				} `json:"sender_id"`
			} `json:"sender"`
			Message struct {
				MessageID   string `json:"message_id"`
				ChatID      string `json:"chat_id"`
				ChatType    string `json:"chat_type"`
				MessageType string `json:"message_type"`
				Content     string `json:"content"`
				Mentions    []struct {
					Key string `json:"key"`
				} `json:"mentions"`
			} `json:"message"`
		} `json:"event"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid event")
		return
	}
	token := payload.Token
	if token == "" {
		token = payload.Header.Token
	}
	binding, err := h.svc.VerifyCallback(r.Context(), chi.URLParam(r, "bindingID"), chi.URLParam(r, "signature"), token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid callback")
		return
	}
	if payload.Challenge != "" {
		writeJSON(w, http.StatusOK, map[string]string{"challenge": payload.Challenge})
		return
	}
	if payload.Event.Message.MessageType != "text" {
		writeJSON(w, http.StatusOK, map[string]bool{"accepted": true})
		return
	}
	var content struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(payload.Event.Message.Content), &content); err != nil {
		writeError(w, http.StatusBadRequest, "invalid message content")
		return
	}
	for _, mention := range payload.Event.Message.Mentions {
		content.Text = strings.ReplaceAll(content.Text, mention.Key, "")
	}
	err = h.svc.HandleInbound(r.Context(), binding, service.LarkInboundEvent{
		EventID: payload.Header.EventID, MessageID: payload.Event.Message.MessageID,
		ChatID: payload.Event.Message.ChatID, ChatType: payload.Event.Message.ChatType,
		SenderOpenID: payload.Event.Sender.SenderID.OpenID, Text: content.Text,
		Mentioned: len(payload.Event.Message.Mentions) > 0,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"accepted": true})
}

func (h *LarkHandler) authorizeAdmin(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	userID, ok := requireUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return "", "", false
	}
	workspaceID := chi.URLParam(r, "workspaceID")
	if !h.svc.IsWorkspaceAdmin(r.Context(), workspaceID, userID) {
		writeError(w, http.StatusForbidden, "Workspace admin access required")
		return "", "", false
	}
	return workspaceID, userID, true
}

func callbackURL(r *http.Request, bindingID, signature string) string {
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host + "/api/v1/lark/events/" + bindingID + "/" + signature
}
