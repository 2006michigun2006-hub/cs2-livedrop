package events

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Event struct {
	ID        int64           `json:"id"`
	UserID    *int64          `json:"user_id,omitempty"`
	Source    string          `json:"source"`
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) Create(ctx context.Context, userID *int64, source, eventType string, payload json.RawMessage) (Event, error) {
	source = strings.TrimSpace(source)
	eventType = strings.TrimSpace(eventType)
	if source == "" || eventType == "" {
		return Event{}, errors.New("source and event_type are required")
	}
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}

	var event Event
	err := s.db.QueryRow(ctx, `
INSERT INTO events (user_id, source, event_type, payload)
VALUES ($1, $2, $3, $4)
RETURNING id, user_id, source, event_type, payload, created_at
`, userID, source, eventType, payload).Scan(&event.ID, &event.UserID, &event.Source, &event.EventType, &event.Payload, &event.CreatedAt)
	if err != nil {
		return Event{}, err
	}

	return event, nil
}

func (s *Service) ListRecent(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}

	rows, err := s.db.Query(ctx, `
SELECT id, user_id, source, event_type, payload, created_at
FROM events
ORDER BY created_at DESC
LIMIT $1
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.UserID, &event.Source, &event.EventType, &event.Payload, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Service) ListByUser(ctx context.Context, userID int64, limit int) ([]Event, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}

	rows, err := s.db.Query(ctx, `
SELECT id, user_id, source, event_type, payload, created_at
FROM events
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2
`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.UserID, &event.Source, &event.EventType, &event.Payload, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}
