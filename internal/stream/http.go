package stream

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

type startRequest struct {
	Title          string `json:"title"`
	TelegramChatID string `json:"telegram_chat_id"`
	SendToChat     bool   `json:"send_to_chat"`
}

type giveawayRuleRequest struct {
	TriggerType string `json:"trigger_type"`
	PrizeType   string `json:"prize_type"`
	PrizeName   string `json:"prize_name"`
	PrizeCents  int64  `json:"prize_cents"`
	Enabled     bool   `json:"enabled"`
}

func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req startRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json body")
		return
	}

	result, err := h.svc.StartSession(r.Context(), user.ID, req.Title, req.TelegramChatID, req.SendToChat)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, result)
}

func (h *Handler) End(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	sessionID, err := strconv.ParseInt(chi.URLParam(r, "sessionID"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid session id")
		return
	}

	session, err := h.svc.EndSession(r.Context(), user.ID, sessionID)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"session": session})
}

func (h *Handler) ActiveMine(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	session, err := h.svc.GetActiveByStreamer(r.Context(), user.ID)
	if err != nil {
		httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"session": nil})
		return
	}
	result := h.svc.buildStartResult(session)
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) JoinInviteAuthenticated(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	inviteCode := chi.URLParam(r, "inviteCode")
	session, err := h.svc.JoinByInvite(r.Context(), inviteCode, user.ID)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"session": session, "joined": true})
}

func (h *Handler) ListParticipants(w http.ResponseWriter, r *http.Request) {
	sessionID, err := strconv.ParseInt(chi.URLParam(r, "sessionID"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid session id")
		return
	}
	participants, err := h.svc.ListParticipants(r.Context(), sessionID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list participants")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"user_ids": participants, "count": len(participants)})
}

func (h *Handler) AddGiveawayRule(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	sessionID, err := strconv.ParseInt(chi.URLParam(r, "sessionID"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid session id")
		return
	}

	var req giveawayRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json body")
		return
	}

	rule, err := h.svc.AddGiveawayRule(r.Context(), user.ID, sessionID, req.TriggerType, req.PrizeType, req.PrizeName, req.PrizeCents)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]interface{}{"rule": rule})
}

func (h *Handler) ListGiveawayRules(w http.ResponseWriter, r *http.Request) {
	sessionID, err := strconv.ParseInt(chi.URLParam(r, "sessionID"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid session id")
		return
	}
	rules, err := h.svc.ListGiveawayRules(r.Context(), sessionID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to list giveaway rules")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"rules": rules})
}

func (h *Handler) ListEventPresets(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"presets": h.svc.ListEventPresets()})
}

func (h *Handler) UpdateGiveawayRule(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	sessionID, err := strconv.ParseInt(chi.URLParam(r, "sessionID"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid session id")
		return
	}
	ruleID, err := strconv.ParseInt(chi.URLParam(r, "ruleID"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid rule id")
		return
	}

	var req giveawayRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json body")
		return
	}

	rule, err := h.svc.UpdateGiveawayRule(r.Context(), user.ID, sessionID, ruleID, req.TriggerType, req.PrizeType, req.PrizeName, req.PrizeCents, req.Enabled)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"rule": rule})
}

func (h *Handler) DeleteGiveawayRule(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	sessionID, err := strconv.ParseInt(chi.URLParam(r, "sessionID"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid session id")
		return
	}
	ruleID, err := strconv.ParseInt(chi.URLParam(r, "ruleID"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid rule id")
		return
	}

	if err := h.svc.DeleteGiveawayRule(r.Context(), user.ID, sessionID, ruleID); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"deleted": true})
}

func (h *Handler) InviteLanding(w http.ResponseWriter, r *http.Request) {
	inviteCode := chi.URLParam(r, "inviteCode")
	query := r.URL.Query()
	target := fmt.Sprintf("/simulator.html?invite=%s", inviteCode)
	if token := query.Get("token"); token != "" {
		target += "&token=" + url.QueryEscape(token)
	}
	if joined := query.Get("invite_joined"); joined != "" {
		target += "&invite_joined=" + joined
	}
	http.Redirect(w, r, target, http.StatusFound)
}
