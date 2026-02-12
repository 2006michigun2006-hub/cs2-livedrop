package cases

import (
	"encoding/json"
	"math"
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

type createCaseRequest struct {
	StreamSessionID     *int64  `json:"stream_session_id"`
	Title               string  `json:"title"`
	Description         string  `json:"description"`
	RewardItemType      string  `json:"reward_item_type"`
	RewardItemName      string  `json:"reward_item_name"`
	TargetAmountCents   int64   `json:"target_amount_cents"`
	TargetAmountDollars float64 `json:"target_amount_dollars"`
}

type contributeRequest struct {
	AmountCents   int64   `json:"amount_cents"`
	AmountDollars float64 `json:"amount_dollars"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createCaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.TargetAmountCents <= 0 && req.TargetAmountDollars > 0 {
		req.TargetAmountCents = int64(math.Round(req.TargetAmountDollars * 100))
	}

	c, err := h.svc.Create(r.Context(), user.ID, req.StreamSessionID, req.Title, req.Description, req.RewardItemType, req.RewardItemName, req.TargetAmountCents)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]interface{}{"case": c})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	limit := 25
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}

	cases, err := h.svc.List(r.Context(), limit)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list cases")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"cases": cases})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	caseID, err := strconv.ParseInt(chi.URLParam(r, "caseID"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid case id")
		return
	}

	var req createCaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.TargetAmountCents <= 0 && req.TargetAmountDollars > 0 {
		req.TargetAmountCents = int64(math.Round(req.TargetAmountDollars * 100))
	}

	c, err := h.svc.Update(r.Context(), caseID, user.ID, req.Title, req.Description, req.RewardItemType, req.RewardItemName, req.TargetAmountCents)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"case": c})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	caseID, err := strconv.ParseInt(chi.URLParam(r, "caseID"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid case id")
		return
	}

	if err := h.svc.Delete(r.Context(), caseID, user.ID); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"deleted": true})
}

func (h *Handler) Contribute(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	caseID, err := strconv.ParseInt(chi.URLParam(r, "caseID"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid case id")
		return
	}

	var req contributeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.AmountCents <= 0 && req.AmountDollars > 0 {
		req.AmountCents = int64(math.Round(req.AmountDollars * 100))
	}

	c, round, rewardItem, err := h.svc.Contribute(r.Context(), caseID, user.ID, req.AmountCents)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"case": c, "lottery_round": round, "reward_item": rewardItem})
}

func (h *Handler) CampaignByInvite(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	inviteCode := chi.URLParam(r, "inviteCode")
	view, err := h.svc.GetCampaignByInvite(r.Context(), inviteCode, user.ID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "campaign not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"campaign": view})
}
