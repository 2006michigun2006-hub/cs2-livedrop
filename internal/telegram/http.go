package telegram

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/2006michigun2006-hub/cs2-livedrop/internal/auth"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/httpx"
)

type Handler struct {
	authService *auth.Service
	botToken    string
}

func NewHandler(authService *auth.Service, botToken string) *Handler {
	return &Handler{authService: authService, botToken: botToken}
}

type loginRequest struct {
	InitData string `json:"init_data"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json body")
		return
	}

	verified, err := VerifyInitData(req.InitData, h.botToken)
	if err != nil {
		httpx.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	user, err := h.authService.UpsertTelegramUser(r.Context(), strconv.FormatInt(verified.User.ID, 10), verified.User.Username)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	token, err := h.authService.IssueToken(user)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not issue token")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"token": token,
		"user":  user,
	})
}
