package wallet

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

type topUpRequest struct {
	AmountCents int64 `json:"amount_cents"`
}

func (h *Handler) GetMyWallet(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	balance, err := h.svc.GetBalance(r.Context(), user.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to fetch balance")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"balance_cents": balance})
}

func (h *Handler) TopUp(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req topUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.AmountCents <= 0 {
		httpx.Error(w, http.StatusBadRequest, "amount_cents must be positive")
		return
	}

	newBalance, err := h.svc.Credit(r.Context(), user.ID, req.AmountCents, "wallet_topup", map[string]interface{}{"source": "manual"})
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"balance_cents": newBalance})
}

func (h *Handler) ListMyTransactions(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	limit := 25
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}

	transactions, err := h.svc.ListTransactions(r.Context(), user.ID, limit)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list transactions")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"transactions": transactions})
}
