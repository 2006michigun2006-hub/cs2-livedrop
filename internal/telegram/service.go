package telegram

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type MiniAppUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type VerifiedData struct {
	User     MiniAppUser
	AuthDate int64
}

func VerifyInitData(initData, botToken string) (VerifiedData, error) {
	if strings.TrimSpace(botToken) == "" {
		return VerifiedData{}, errors.New("telegram bot token not configured")
	}

	values, err := url.ParseQuery(initData)
	if err != nil {
		return VerifiedData{}, fmt.Errorf("invalid init_data: %w", err)
	}

	hash := values.Get("hash")
	if hash == "" {
		return VerifiedData{}, errors.New("missing hash")
	}
	values.Del("hash")

	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+values.Get(k))
	}
	dataCheckString := strings.Join(parts, "\n")

	secretMAC := hmac.New(sha256.New, []byte("WebAppData"))
	secretMAC.Write([]byte(botToken))
	secretKey := secretMAC.Sum(nil)

	checkMAC := hmac.New(sha256.New, secretKey)
	checkMAC.Write([]byte(dataCheckString))
	expectedHash := hex.EncodeToString(checkMAC.Sum(nil))

	if !hmac.Equal([]byte(strings.ToLower(expectedHash)), []byte(strings.ToLower(hash))) {
		return VerifiedData{}, errors.New("init_data hash mismatch")
	}

	authDateRaw := values.Get("auth_date")
	authDate, err := strconv.ParseInt(authDateRaw, 10, 64)
	if err != nil {
		return VerifiedData{}, errors.New("invalid auth_date")
	}
	if time.Since(time.Unix(authDate, 0)) > 24*time.Hour {
		return VerifiedData{}, errors.New("stale init_data")
	}

	userRaw := values.Get("user")
	if userRaw == "" {
		return VerifiedData{}, errors.New("missing user payload")
	}

	var user MiniAppUser
	if err := json.Unmarshal([]byte(userRaw), &user); err != nil {
		return VerifiedData{}, errors.New("invalid user payload")
	}
	if user.ID == 0 {
		return VerifiedData{}, errors.New("invalid user id")
	}

	return VerifiedData{User: user, AuthDate: authDate}, nil
}
