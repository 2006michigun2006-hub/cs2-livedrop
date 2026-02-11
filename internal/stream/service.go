package stream

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/2006michigun2006-hub/cs2-livedrop/internal/inventory"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/lottery"
	"github.com/jackc/pgx/v5/pgxpool"
	qrcode "github.com/skip2/go-qrcode"
)

type BotSender interface {
	SendMessage(ctx context.Context, chatID, text string) error
}

type Service struct {
	db          *pgxpool.Pool
	lottery     *lottery.Service
	inventory   *inventory.Service
	bot         BotSender
	baseURL     string
	botUsername string
}

type Session struct {
	ID             int64      `json:"id"`
	StreamerID     int64      `json:"streamer_id"`
	Title          string     `json:"title"`
	InviteCode     string     `json:"invite_code"`
	TelegramChatID string     `json:"telegram_chat_id,omitempty"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
}

type GiveawayRule struct {
	ID              int64     `json:"id"`
	StreamSessionID int64     `json:"stream_session_id"`
	TriggerType     string    `json:"trigger_type"`
	PrizeType       string    `json:"prize_type"`
	PrizeName       string    `json:"prize_name"`
	PrizeCents      int64     `json:"prize_cents"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
}

type EventPreset struct {
	TriggerType string `json:"trigger_type"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type StartResult struct {
	Session          Session `json:"session"`
	InviteURL        string  `json:"invite_url"`
	SteamInviteURL   string  `json:"steam_invite_url"`
	TelegramDeepLink string  `json:"telegram_deeplink,omitempty"`
	QRCodePNGBase64  string  `json:"qr_code_png_base64"`
}

func NewService(db *pgxpool.Pool, lottery *lottery.Service, inventory *inventory.Service, bot BotSender, baseURL, botUsername string) *Service {
	return &Service{db: db, lottery: lottery, inventory: inventory, bot: bot, baseURL: strings.TrimRight(baseURL, "/"), botUsername: strings.TrimPrefix(botUsername, "@")}
}

func (s *Service) StartSession(ctx context.Context, streamerID int64, title, telegramChatID string, sendToChat bool) (StartResult, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "LiveDrop Session"
	}

	inviteCode, err := generateInviteCode(12)
	if err != nil {
		return StartResult{}, err
	}

	var session Session
	err = s.db.QueryRow(ctx, `
INSERT INTO stream_sessions (streamer_id, title, invite_code, telegram_chat_id)
VALUES ($1, $2, $3, $4)
RETURNING id, streamer_id, title, invite_code, COALESCE(telegram_chat_id, ''), status, created_at, ended_at
`, streamerID, title, inviteCode, nullIfEmpty(telegramChatID)).Scan(
		&session.ID,
		&session.StreamerID,
		&session.Title,
		&session.InviteCode,
		&session.TelegramChatID,
		&session.Status,
		&session.CreatedAt,
		&session.EndedAt,
	)
	if err != nil {
		return StartResult{}, err
	}

	result := s.buildStartResult(session)

	if sendToChat && s.bot != nil && session.TelegramChatID != "" {
		message := fmt.Sprintf("%s is live. Join giveaway pool: %s\nSteam quick join: %s", session.Title, result.InviteURL, result.SteamInviteURL)
		_ = s.bot.SendMessage(ctx, session.TelegramChatID, message)
	}

	return result, nil
}

func (s *Service) buildStartResult(session Session) StartResult {
	inviteURL := fmt.Sprintf("%s/invite/%s", s.baseURL, session.InviteCode)
	steamInviteURL := fmt.Sprintf("%s/api/auth/steam/login?invite=%s", s.baseURL, session.InviteCode)
	qrText := inviteURL
	png, _ := qrcode.Encode(qrText, qrcode.Medium, 256)
	qrBase64 := base64.StdEncoding.EncodeToString(png)

	deepLink := ""
	if s.botUsername != "" {
		deepLink = fmt.Sprintf("https://t.me/%s?start=invite_%s", s.botUsername, session.InviteCode)
	}

	return StartResult{
		Session:          session,
		InviteURL:        inviteURL,
		SteamInviteURL:   steamInviteURL,
		TelegramDeepLink: deepLink,
		QRCodePNGBase64:  qrBase64,
	}
}

func (s *Service) EndSession(ctx context.Context, streamerID, sessionID int64) (Session, error) {
	var session Session
	err := s.db.QueryRow(ctx, `
UPDATE stream_sessions
SET status = 'ended', ended_at = NOW()
WHERE id = $1 AND streamer_id = $2 AND status = 'active'
RETURNING id, streamer_id, title, invite_code, COALESCE(telegram_chat_id, ''), status, created_at, ended_at
`, sessionID, streamerID).Scan(
		&session.ID,
		&session.StreamerID,
		&session.Title,
		&session.InviteCode,
		&session.TelegramChatID,
		&session.Status,
		&session.CreatedAt,
		&session.EndedAt,
	)
	if err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Service) GetActiveByStreamer(ctx context.Context, streamerID int64) (Session, error) {
	var session Session
	err := s.db.QueryRow(ctx, `
SELECT id, streamer_id, title, invite_code, COALESCE(telegram_chat_id, ''), status, created_at, ended_at
FROM stream_sessions
WHERE streamer_id = $1 AND status = 'active'
ORDER BY created_at DESC
LIMIT 1
`, streamerID).Scan(
		&session.ID,
		&session.StreamerID,
		&session.Title,
		&session.InviteCode,
		&session.TelegramChatID,
		&session.Status,
		&session.CreatedAt,
		&session.EndedAt,
	)
	return session, err
}

func (s *Service) JoinByInvite(ctx context.Context, inviteCode string, userID int64) (Session, error) {
	inviteCode = strings.TrimSpace(inviteCode)
	if inviteCode == "" {
		return Session{}, errors.New("invite code is required")
	}

	var session Session
	err := s.db.QueryRow(ctx, `
SELECT id, streamer_id, title, invite_code, COALESCE(telegram_chat_id, ''), status, created_at, ended_at
FROM stream_sessions
WHERE invite_code = $1 AND status = 'active'
`, inviteCode).Scan(
		&session.ID,
		&session.StreamerID,
		&session.Title,
		&session.InviteCode,
		&session.TelegramChatID,
		&session.Status,
		&session.CreatedAt,
		&session.EndedAt,
	)
	if err != nil {
		return Session{}, err
	}

	_, err = s.db.Exec(ctx, `
INSERT INTO stream_participants (stream_session_id, user_id)
VALUES ($1, $2)
ON CONFLICT (stream_session_id, user_id) DO NOTHING
`, session.ID, userID)
	if err != nil {
		return Session{}, err
	}

	_ = s.lottery.RecordActivity(ctx, userID, 2)
	return session, nil
}

func (s *Service) JoinInvite(ctx context.Context, inviteCode string, userID int64) error {
	_, err := s.JoinByInvite(ctx, inviteCode, userID)
	return err
}

func (s *Service) ListParticipants(ctx context.Context, sessionID int64) ([]int64, error) {
	rows, err := s.db.Query(ctx, `
SELECT user_id
FROM stream_participants
WHERE stream_session_id = $1
ORDER BY joined_at DESC
`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

func (s *Service) AddGiveawayRule(ctx context.Context, streamerID, sessionID int64, triggerType, prizeType, prizeName string, prizeCents int64) (GiveawayRule, error) {
	if triggerType == "" || prizeName == "" {
		return GiveawayRule{}, errors.New("trigger_type and prize_name are required")
	}
	prizeType = strings.ToLower(strings.TrimSpace(prizeType))
	if prizeType != "skin" && prizeType != "case" {
		return GiveawayRule{}, errors.New("prize_type must be skin or case")
	}
	if prizeCents < 0 {
		return GiveawayRule{}, errors.New("prize_cents cannot be negative")
	}

	var owner int64
	if err := s.db.QueryRow(ctx, `SELECT streamer_id FROM stream_sessions WHERE id = $1`, sessionID).Scan(&owner); err != nil {
		return GiveawayRule{}, err
	}
	if owner != streamerID {
		return GiveawayRule{}, errors.New("not your stream session")
	}

	var rule GiveawayRule
	err := s.db.QueryRow(ctx, `
INSERT INTO giveaway_rules (stream_session_id, trigger_type, prize_type, prize_name, prize_cents)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, stream_session_id, trigger_type, prize_type, prize_name, prize_cents, enabled, created_at
`, sessionID, strings.ToLower(strings.TrimSpace(triggerType)), prizeType, strings.TrimSpace(prizeName), prizeCents).Scan(
		&rule.ID,
		&rule.StreamSessionID,
		&rule.TriggerType,
		&rule.PrizeType,
		&rule.PrizeName,
		&rule.PrizeCents,
		&rule.Enabled,
		&rule.CreatedAt,
	)
	return rule, err
}

func (s *Service) ListGiveawayRules(ctx context.Context, sessionID int64) ([]GiveawayRule, error) {
	rows, err := s.db.Query(ctx, `
SELECT id, stream_session_id, trigger_type, prize_type, prize_name, prize_cents, enabled, created_at
FROM giveaway_rules
WHERE stream_session_id = $1
ORDER BY created_at DESC
`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]GiveawayRule, 0)
	for rows.Next() {
		var rule GiveawayRule
		if err := rows.Scan(&rule.ID, &rule.StreamSessionID, &rule.TriggerType, &rule.PrizeType, &rule.PrizeName, &rule.PrizeCents, &rule.Enabled, &rule.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, rule)
	}
	return result, rows.Err()
}

func (s *Service) ListEventPresets() []EventPreset {
	return []EventPreset{
		{TriggerType: "ace", Label: "Ace", Description: "Streamer kills all 5 enemies in the round."},
		{TriggerType: "headshot", Label: "Headshot", Description: "Streamer lands one or more headshots in a round."},
		{TriggerType: "bomb_plant", Label: "Bomb Plant", Description: "Streamer side successfully plants the bomb."},
		{TriggerType: "round_win", Label: "Round Win", Description: "Streamer's team wins a round."},
		{TriggerType: "kill", Label: "Kill", Description: "Streamer gets a kill event."},
		{TriggerType: "death", Label: "Death", Description: "Streamer dies."},
	}
}

func (s *Service) UpdateGiveawayRule(ctx context.Context, streamerID, sessionID, ruleID int64, triggerType, prizeType, prizeName string, prizeCents int64, enabled bool) (GiveawayRule, error) {
	if triggerType == "" || prizeName == "" {
		return GiveawayRule{}, errors.New("trigger_type and prize_name are required")
	}
	prizeType = strings.ToLower(strings.TrimSpace(prizeType))
	if prizeType != "skin" && prizeType != "case" {
		return GiveawayRule{}, errors.New("prize_type must be skin or case")
	}

	var owner int64
	if err := s.db.QueryRow(ctx, `SELECT streamer_id FROM stream_sessions WHERE id = $1`, sessionID).Scan(&owner); err != nil {
		return GiveawayRule{}, err
	}
	if owner != streamerID {
		return GiveawayRule{}, errors.New("not your stream session")
	}

	var rule GiveawayRule
	err := s.db.QueryRow(ctx, `
UPDATE giveaway_rules
SET trigger_type = $1, prize_type = $2, prize_name = $3, prize_cents = $4, enabled = $5
WHERE id = $6 AND stream_session_id = $7
RETURNING id, stream_session_id, trigger_type, prize_type, prize_name, prize_cents, enabled, created_at
`, strings.ToLower(strings.TrimSpace(triggerType)), prizeType, strings.TrimSpace(prizeName), prizeCents, enabled, ruleID, sessionID).Scan(
		&rule.ID,
		&rule.StreamSessionID,
		&rule.TriggerType,
		&rule.PrizeType,
		&rule.PrizeName,
		&rule.PrizeCents,
		&rule.Enabled,
		&rule.CreatedAt,
	)
	return rule, err
}

func (s *Service) DeleteGiveawayRule(ctx context.Context, streamerID, sessionID, ruleID int64) error {
	var owner int64
	if err := s.db.QueryRow(ctx, `SELECT streamer_id FROM stream_sessions WHERE id = $1`, sessionID).Scan(&owner); err != nil {
		return err
	}
	if owner != streamerID {
		return errors.New("not your stream session")
	}

	result, err := s.db.Exec(ctx, `DELETE FROM giveaway_rules WHERE id = $1 AND stream_session_id = $2`, ruleID, sessionID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errors.New("rule not found")
	}
	return nil
}

func (s *Service) HandleGameEvent(ctx context.Context, streamerID int64, eventType string, triggerEventID *int64) ([]lottery.Round, error) {
	session, err := s.GetActiveByStreamer(ctx, streamerID)
	if err != nil {
		return nil, nil
	}

	rows, err := s.db.Query(ctx, `
SELECT id, stream_session_id, trigger_type, prize_type, prize_name, prize_cents, enabled, created_at
FROM giveaway_rules
WHERE stream_session_id = $1 AND enabled = TRUE AND trigger_type = $2
`, session.ID, strings.ToLower(strings.TrimSpace(eventType)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := make([]GiveawayRule, 0)
	for rows.Next() {
		var rule GiveawayRule
		if err := rows.Scan(&rule.ID, &rule.StreamSessionID, &rule.TriggerType, &rule.PrizeType, &rule.PrizeName, &rule.PrizeCents, &rule.Enabled, &rule.CreatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	if len(rules) == 0 {
		return nil, nil
	}

	participants, err := s.ListParticipants(ctx, session.ID)
	if err != nil || len(participants) == 0 {
		return nil, err
	}

	streamID := session.ID
	triggered := make([]lottery.Round, 0)
	for _, rule := range rules {
		round, err := s.lottery.TriggerForUsers(ctx, rule.TriggerType, triggerEventID, &streamID, rule.PrizeCents, participants, map[string]interface{}{
			"prize_type": rule.PrizeType,
			"prize_name": rule.PrizeName,
			"rule_id":    rule.ID,
		})
		if err == nil && round != nil {
			triggered = append(triggered, *round)
			if s.inventory != nil && round.WinnerUserID != nil {
				_, _ = s.inventory.GrantItem(ctx, *round.WinnerUserID, rule.PrizeType, rule.PrizeName, "restricted", "stream_giveaway", map[string]interface{}{
					"stream_session_id": session.ID,
					"rule_id":           rule.ID,
					"trigger_type":      rule.TriggerType,
					"price_cents":       rule.PrizeCents,
				})
			}
		}
	}

	return triggered, nil
}

func generateInviteCode(length int) (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		result[i] = alphabet[n.Int64()]
	}
	return string(result), nil
}

func nullIfEmpty(v string) interface{} {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return strings.TrimSpace(v)
}
