package events

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/2006michigun2006-hub/cs2-livedrop/internal/auth"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

type createEventRequest struct {
	Source    string          `json:"source"`
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json body")
		return
	}

	event, err := h.svc.Create(r.Context(), &user.ID, req.Source, req.EventType, req.Payload)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]interface{}{"event": event})
}

func (h *Handler) ListRecent(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r.URL.Query().Get("limit"))
	events, err := h.svc.ListRecent(r.Context(), limit)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not fetch events")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"events": events})
}

func (h *Handler) ListMine(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	limit := parseLimit(r.URL.Query().Get("limit"))
	events, err := h.svc.ListByUser(r.Context(), user.ID, limit)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not fetch events")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"events": events})
}

func parseLimit(raw string) int {
	if raw == "" {
		return 25
	}
	limit, err := strconv.Atoi(raw)
	if err != nil {
		return 25
	}
	return limit
}
