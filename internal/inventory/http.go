package inventory

import (
	"net/http"
	"strconv"

	"github.com/2006michigun2006-hub/cs2-livedrop/internal/auth"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/httpx"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) ListMine(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}

	items, err := h.svc.ListByUser(r.Context(), user.ID, limit)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list inventory")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

func (h *Handler) OpenCase(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	itemID, err := strconv.ParseInt(chi.URLParam(r, "itemID"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid item id")
		return
	}

	openedCase, droppedSkin, err := h.svc.OpenCase(r.Context(), user.ID, itemID)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"opened_case": openedCase,
		"drop":        droppedSkin,
	})
}

func (h *Handler) SellItem(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	itemID, err := strconv.ParseInt(chi.URLParam(r, "itemID"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid item id")
		return
	}

	item, newBalance, err := h.svc.SellItem(r.Context(), user.ID, itemID)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"item":           item,
		"balance_cents":  newBalance,
		"credited_cents": item.PriceCents,
		"message":        "item sold",
	})
}
