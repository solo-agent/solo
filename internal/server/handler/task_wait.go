package handler

import (
	"encoding/json"
	"net/http"

	"github.com/solo-ai/solo/internal/server/service"
	"github.com/solo-ai/solo/internal/server/ws"
)

func (h *TaskHandler) Waits(w http.ResponseWriter, r *http.Request) {
	actor, task, ok := h.deliveryTask(w, r)
	if !ok {
		return
	}
	data, err := h.svc.ListTaskWaits(r.Context(), task.ChannelID, task.ID, actor)
	if err != nil {
		h.writeTaskLifecycleError(w, err)
		return
	}
	writeJSON(w, 200, data)
}

func (h *TaskHandler) Wait(w http.ResponseWriter, r *http.Request) {
	actor, task, ok := h.deliveryTask(w, r)
	if !ok {
		return
	}
	var req service.TaskWaitRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, 400, "invalid wait request")
		return
	}
	id, err := h.svc.WaitTask(r.Context(), task.ChannelID, task.ID, actor, req)
	if err != nil {
		h.writeTaskLifecycleError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"id": id})
	h.broadcastWaitTask(r, task, actor)
}

func (h *TaskHandler) ResolveWait(w http.ResponseWriter, r *http.Request) {
	actor, task, ok := h.deliveryTask(w, r)
	if !ok {
		return
	}
	var req service.TaskWaitResolution
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		writeError(w, 400, "invalid wait resolution")
		return
	}
	if err := h.svc.ResolveTaskWait(r.Context(), task.ChannelID, task.ID, actor, req); err != nil {
		h.writeTaskLifecycleError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
	h.broadcastWaitTask(r, task, actor)
}

func (h *TaskHandler) broadcastWaitTask(r *http.Request, task *service.Task, actor string) {
	if h.hub == nil {
		return
	}
	updated, err := h.svc.GetTask(r.Context(), task.ChannelID, task.ID, actor)
	if err == nil {
		h.hub.BroadcastToChannel(task.ChannelID, ws.Envelope(ws.EventTaskUpdated, toTaskResponse(updated)))
	}
}
