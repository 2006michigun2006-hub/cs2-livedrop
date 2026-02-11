package lottery

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

type joinRequest struct {
	ScoreDelta int64 `json:"score_delta"`
}

func (h *Handler) Join(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req joinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json body")
		return
	}

	if err := h.svc.Join(r.Context(), user.ID, req.ScoreDelta); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to join lottery")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"joined": true})
}

func (h *Handler) ListRounds(w http.ResponseWriter, r *http.Request) {
	limit := 25
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}

	rounds, err := h.svc.ListRounds(r.Context(), limit)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list rounds")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"rounds": rounds})
}
