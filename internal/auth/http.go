package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/2006michigun2006-hub/cs2-livedrop/internal/httpx"
	"github.com/go-chi/chi/v5"
)

const steamOpenIDURL = "https://steamcommunity.com/openid/login"

const (
	jwtCookieName        = "livedrop_jwt"
	steamInviteCookie    = "livedrop_steam_invite"
	steamCookieMaxAgeSec = 10 * 60
)

type Handler struct {
	svc          *Service
	inviteJoiner InviteJoiner
}

type InviteJoiner interface {
	JoinInvite(ctx context.Context, inviteCode string, userID int64) error
}

func NewHandler(svc *Service, inviteJoiner InviteJoiner) *Handler {
	return &Handler{svc: svc, inviteJoiner: inviteJoiner}
}

type authRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Identity string `json:"identity"`
	Password string `json:"password"`
}

type setRoleRequest struct {
	Role string `json:"role"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json body")
		return
	}

	user, err := h.svc.Register(r.Context(), req.Username, req.Email, req.Password)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	token, err := h.svc.IssueToken(user)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not issue token")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]interface{}{"token": token, "user": user})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json body")
		return
	}

	identity := req.Identity
	if identity == "" {
		identity = req.Email
	}

	user, err := h.svc.Login(r.Context(), identity, req.Password)
	if err != nil {
		httpx.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	token, err := h.svc.IssueToken(user)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not issue token")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"token": token, "user": user})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"user": user})
}

func (h *Handler) SetUserRole(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid user id")
		return
	}

	var req setRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json body")
		return
	}

	updatedUser, err := h.svc.SetRole(r.Context(), userID, req.Role)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"user": updatedUser})
}

func (h *Handler) BecomeStreamer(w http.ResponseWriter, r *http.Request) {
	user, ok := UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	updatedUser, err := h.svc.PromoteToStreamer(r.Context(), user.ID)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"user": updatedUser})
}

func (h *Handler) SteamLogin(w http.ResponseWriter, r *http.Request) {
	inviteCode := strings.TrimSpace(r.URL.Query().Get("invite"))
	setInviteCookie(w, inviteCode, h.isSecureCookie())
	returnTo := fmt.Sprintf("%s/api/auth/steam/callback", h.svc.baseURL)
	values := url.Values{}
	values.Set("openid.ns", "http://specs.openid.net/auth/2.0")
	values.Set("openid.mode", "checkid_setup")
	values.Set("openid.return_to", returnTo)
	values.Set("openid.realm", h.svc.baseURL)
	values.Set("openid.identity", "http://specs.openid.net/auth/2.0/identifier_select")
	values.Set("openid.claimed_id", "http://specs.openid.net/auth/2.0/identifier_select")

	http.Redirect(w, r, steamOpenIDURL+"?"+values.Encode(), http.StatusFound)
}

func (h *Handler) SteamCallback(w http.ResponseWriter, r *http.Request) {
	inviteCode := strings.TrimSpace(r.URL.Query().Get("invite"))
	if inviteCode == "" {
		if c, err := r.Cookie(steamInviteCookie); err == nil {
			inviteCode = strings.TrimSpace(c.Value)
		}
	}
	clearInviteCookie(w, h.isSecureCookie())

	steamID, err := verifySteamLogin(r)
	if err != nil {
		httpx.Error(w, http.StatusUnauthorized, "steam auth failed: "+err.Error())
		return
	}

	user, err := h.svc.UpsertSteamUser(r.Context(), steamID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "steam user creation failed")
		return
	}

	token, err := h.svc.IssueToken(user)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	setJWTCookie(w, token, h.isSecureCookie())

	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		joined := false
		if inviteCode != "" && h.inviteJoiner != nil {
			if err := h.inviteJoiner.JoinInvite(r.Context(), inviteCode, user.ID); err == nil {
				joined = true
			} else {
				log.Printf("steam callback invite join failed: invite=%s user_id=%d err=%v", inviteCode, user.ID, err)
			}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{"token": token, "user": user, "invite_joined": joined})
		return
	}
	redirectURL := "/simulator.html?token=" + url.QueryEscape(token)
	if inviteCode != "" {
		inviteJoined := "0"
		if h.inviteJoiner != nil {
			if err := h.inviteJoiner.JoinInvite(r.Context(), inviteCode, user.ID); err == nil {
				inviteJoined = "1"
			} else {
				log.Printf("steam callback invite join failed: invite=%s user_id=%d err=%v", inviteCode, user.ID, err)
			}
		}
		redirectURL = "/invite/" + url.PathEscape(inviteCode) + "?token=" + url.QueryEscape(token) + "&invite_joined=" + inviteJoined
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	clearJWTCookie(w, h.isSecureCookie())
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) isSecureCookie() bool {
	return strings.HasPrefix(strings.ToLower(h.svc.baseURL), "https://")
}

func setJWTCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     jwtCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int((24 * time.Hour).Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearJWTCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     jwtCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func setInviteCookie(w http.ResponseWriter, inviteCode string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     steamInviteCookie,
		Value:    inviteCode,
		Path:     "/",
		MaxAge:   steamCookieMaxAgeSec,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearInviteCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     steamInviteCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func verifySteamLogin(r *http.Request) (string, error) {
	query := r.URL.Query()
	claimedID := query.Get("openid.claimed_id")
	if claimedID == "" {
		return "", errors.New("missing claimed id")
	}

	values := url.Values{}
	for key, entries := range query {
		if !strings.HasPrefix(key, "openid.") {
			continue
		}
		for _, value := range entries {
			values.Add(key, value)
		}
	}
	values.Set("openid.mode", "check_authentication")

	resp, err := http.PostForm(steamOpenIDURL, values)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if !strings.Contains(string(body), "is_valid:true") {
		return "", errors.New("steam validation rejected")
	}

	parts := strings.Split(claimedID, "/")
	steamID := parts[len(parts)-1]
	if steamID == "" {
		return "", errors.New("invalid steam id")
	}
	return steamID, nil
}
