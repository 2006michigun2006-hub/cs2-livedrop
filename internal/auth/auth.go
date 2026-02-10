package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type contextKey string

const userContextKey contextKey = "authenticatedUser"

const (
	RoleViewer   = "viewer"
	RoleStreamer = "streamer"
	RoleAdmin    = "admin"
)

type User struct {
	ID               int64     `json:"id"`
	Username         string    `json:"username"`
	Email            string    `json:"email,omitempty"`
	SteamID          string    `json:"steam_id,omitempty"`
	TelegramID       string    `json:"telegram_id,omitempty"`
	TelegramUsername string    `json:"telegram_username,omitempty"`
	Role             string    `json:"role"`
	BalanceCents     int64     `json:"balance_cents"`
	CreatedAt        time.Time `json:"created_at"`
}

type Service struct {
	db        *pgxpool.Pool
	jwtSecret []byte
	baseURL   string
}

func NewService(db *pgxpool.Pool, jwtSecret, baseURL string) *Service {
	return &Service{db: db, jwtSecret: []byte(jwtSecret), baseURL: strings.TrimRight(baseURL, "/")}
}

func IsValidRole(role string) bool {
	return role == RoleViewer || role == RoleStreamer || role == RoleAdmin
}

func (s *Service) Register(ctx context.Context, username, email, password string) (User, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(strings.ToLower(email))
	if len(username) < 3 || len(password) < 8 || !strings.Contains(email, "@") {
		return User{}, errors.New("username/email/password does not meet minimum requirements")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}

	role, err := s.defaultRole(ctx)
	if err != nil {
		return User{}, err
	}

	var user User
	err = s.db.QueryRow(ctx, `
INSERT INTO users (username, email, password_hash, role)
VALUES ($1, $2, $3, $4)
RETURNING id, username, email, COALESCE(steam_id, ''), COALESCE(telegram_id, ''), COALESCE(telegram_username, ''), role, balance_cents, created_at
`, username, email, string(hash), role).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.SteamID,
		&user.TelegramID,
		&user.TelegramUsername,
		&user.Role,
		&user.BalanceCents,
		&user.CreatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return User{}, errors.New("username or email already exists")
		}
		return User{}, err
	}

	return user, nil
}

func (s *Service) Login(ctx context.Context, identity, password string) (User, error) {
	identity = strings.TrimSpace(strings.ToLower(identity))
	if identity == "" || password == "" {
		return User{}, errors.New("missing credentials")
	}

	var user User
	var passwordHash string
	err := s.db.QueryRow(ctx, `
SELECT id, username, COALESCE(email, ''), COALESCE(steam_id, ''), COALESCE(telegram_id, ''), COALESCE(telegram_username, ''), role, balance_cents, created_at, COALESCE(password_hash, '')
FROM users
WHERE LOWER(email) = $1 OR LOWER(username) = $1
`, identity).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.SteamID,
		&user.TelegramID,
		&user.TelegramUsername,
		&user.Role,
		&user.BalanceCents,
		&user.CreatedAt,
		&passwordHash,
	)
	if err != nil {
		return User{}, errors.New("invalid credentials")
	}

	if passwordHash == "" || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		return User{}, errors.New("invalid credentials")
	}

	return user, nil
}

func (s *Service) UpsertSteamUser(ctx context.Context, steamID string) (User, error) {
	if steamID == "" {
		return User{}, errors.New("missing steam id")
	}

	role, err := s.defaultRole(ctx)
	if err != nil {
		return User{}, err
	}

	suffix := steamID
	if len(steamID) > 6 {
		suffix = steamID[len(steamID)-6:]
	}
	defaultUsername := fmt.Sprintf("steam_%s", suffix)

	var user User
	err = s.db.QueryRow(ctx, `
INSERT INTO users (username, steam_id, role)
VALUES ($1, $2, $3)
ON CONFLICT (steam_id)
DO UPDATE SET steam_id = EXCLUDED.steam_id
RETURNING id, username, COALESCE(email, ''), COALESCE(steam_id, ''), COALESCE(telegram_id, ''), COALESCE(telegram_username, ''), role, balance_cents, created_at
`, defaultUsername, steamID, role).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.SteamID,
		&user.TelegramID,
		&user.TelegramUsername,
		&user.Role,
		&user.BalanceCents,
		&user.CreatedAt,
	)
	if err != nil {
		return User{}, err
	}

	return user, nil
}

func (s *Service) UpsertTelegramUser(ctx context.Context, telegramID, telegramUsername string) (User, error) {
	telegramID = strings.TrimSpace(telegramID)
	telegramUsername = strings.TrimSpace(telegramUsername)
	if telegramID == "" {
		return User{}, errors.New("missing telegram id")
	}

	role, err := s.defaultRole(ctx)
	if err != nil {
		return User{}, err
	}

	username := telegramUsername
	if username == "" {
		suffix := telegramID
		if len(telegramID) > 6 {
			suffix = telegramID[len(telegramID)-6:]
		}
		username = "tg_" + suffix
	}
	email := fmt.Sprintf("%s@telegram.local", telegramID)

	var user User
	err = s.db.QueryRow(ctx, `
INSERT INTO users (username, email, telegram_id, telegram_username, role)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (telegram_id)
DO UPDATE SET telegram_username = EXCLUDED.telegram_username
RETURNING id, username, COALESCE(email, ''), COALESCE(steam_id, ''), COALESCE(telegram_id, ''), COALESCE(telegram_username, ''), role, balance_cents, created_at
`, username, email, telegramID, telegramUsername, role).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.SteamID,
		&user.TelegramID,
		&user.TelegramUsername,
		&user.Role,
		&user.BalanceCents,
		&user.CreatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return User{}, errors.New("telegram account conflicts with existing username/email")
		}
		return User{}, err
	}

	return user, nil
}

func (s *Service) GetByID(ctx context.Context, userID int64) (User, error) {
	var user User
	err := s.db.QueryRow(ctx, `
SELECT id, username, COALESCE(email, ''), COALESCE(steam_id, ''), COALESCE(telegram_id, ''), COALESCE(telegram_username, ''), role, balance_cents, created_at
FROM users
WHERE id = $1
`, userID).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.SteamID,
		&user.TelegramID,
		&user.TelegramUsername,
		&user.Role,
		&user.BalanceCents,
		&user.CreatedAt,
	)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Service) SetRole(ctx context.Context, userID int64, role string) (User, error) {
	role = strings.ToLower(strings.TrimSpace(role))
	if !IsValidRole(role) {
		return User{}, errors.New("invalid role")
	}

	var user User
	err := s.db.QueryRow(ctx, `
UPDATE users
SET role = $1
WHERE id = $2
RETURNING id, username, COALESCE(email, ''), COALESCE(steam_id, ''), COALESCE(telegram_id, ''), COALESCE(telegram_username, ''), role, balance_cents, created_at
`, role, userID).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.SteamID,
		&user.TelegramID,
		&user.TelegramUsername,
		&user.Role,
		&user.BalanceCents,
		&user.CreatedAt,
	)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Service) PromoteToStreamer(ctx context.Context, userID int64) (User, error) {
	current, err := s.GetByID(ctx, userID)
	if err != nil {
		return User{}, err
	}
	if current.Role == RoleAdmin || current.Role == RoleStreamer {
		return current, nil
	}
	return s.SetRole(ctx, userID, RoleStreamer)
}

func (s *Service) IssueToken(user User) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(user.ID, 10),
		Issuer:    s.baseURL,
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *Service) ParseToken(tokenString string) (int64, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return 0, err
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return 0, errors.New("invalid token")
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return 0, err
	}

	return userID, nil
}

func (s *Service) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			if cookie, err := r.Cookie(jwtCookieName); err == nil {
				token = strings.TrimSpace(cookie.Value)
			}
		}
		if token == "" {
			http.Error(w, "missing auth token", http.StatusUnauthorized)
			return
		}

		userID, err := s.ParseToken(token)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		user, err := s.GetByID(r.Context(), userID)
		if err != nil {
			http.Error(w, "user not found", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Service) RequireRoles(allowedRoles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedRoles))
	for _, role := range allowedRoles {
		allowed[strings.ToLower(strings.TrimSpace(role))] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := UserFromContext(r.Context())
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if _, ok := allowed[user.Role]; !ok {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func UserFromContext(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(userContextKey).(User)
	return user, ok
}

func bearerToken(header string) string {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func RequestID(ctx context.Context) string {
	return middleware.GetReqID(ctx)
}

func (s *Service) defaultRole(ctx context.Context) (string, error) {
	var count int64
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return "", err
	}
	if count == 0 {
		return RoleAdmin, nil
	}
	return RoleViewer, nil
}
