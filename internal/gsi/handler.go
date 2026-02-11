package gsi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/2006michigun2006-hub/cs2-livedrop/internal/auth"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/events"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/httpx"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/lottery"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/stream"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	events  *events.Service
	lottery *lottery.Service
	stream  *stream.Service
	db      *pgxpool.Pool
}

func NewHandler(events *events.Service, lottery *lottery.Service, stream *stream.Service, db *pgxpool.Pool) *Handler {
	return &Handler{events: events, lottery: lottery, stream: stream, db: db}
}

func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.Error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid gsi payload")
		return
	}

	var userID *int64
	if user, ok := auth.UserFromContext(r.Context()); ok {
		userID = &user.ID
		_ = h.lottery.RecordActivity(r.Context(), user.ID, 1)
	}

	stored, triggeredRounds, packetHash, deduplicated, err := h.processPayload(r.Context(), payload, userID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":           "ok",
		"deduplicated":     deduplicated,
		"packet_hash":      packetHash,
		"stored_events":    stored,
		"triggered_rounds": triggeredRounds,
	})
}

type fakeGenerateRequest struct {
	EventType string `json:"event_type"`
	Count     int    `json:"count"`
}

func (h *Handler) GenerateFake(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req fakeGenerateRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Count <= 0 || req.Count > 20 {
		req.Count = 1
	}

	eventType := strings.ToLower(strings.TrimSpace(req.EventType))
	if eventType == "" {
		eventType = "ace"
	}

	userID := user.ID
	storedAll := make([]events.Event, 0)
	roundsAll := make([]lottery.Round, 0)

	for i := 0; i < req.Count; i++ {
		payload := fakePayloadForEvent(eventType)
		payload["__fake_nonce"] = fmt.Sprintf("%d_%d_%d", userID, time.Now().UnixNano(), i)
		stored, rounds, _, _, err := h.processPayload(r.Context(), payload, &userID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err.Error())
			return
		}
		storedAll = append(storedAll, stored...)
		roundsAll = append(roundsAll, rounds...)
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":           "ok",
		"generated":        req.Count,
		"event_type":       eventType,
		"generated_events": len(storedAll),
		"generated_rounds": len(roundsAll),
		"stored_events":    storedAll,
		"triggered_rounds": roundsAll,
	})
}

func (h *Handler) processPayload(ctx context.Context, payload map[string]interface{}, userID *int64) ([]events.Event, []lottery.Round, string, bool, error) {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, "", false, err
	}

	packetHash := hashPayload(rawPayload)

	inserted, err := h.insertPacketHash(ctx, packetHash, userID)
	if err != nil {
		return nil, nil, "", false, err
	}
	if !inserted {
		return nil, nil, packetHash, true, nil
	}

	stored := make([]events.Event, 0)
	triggeredRounds := make([]lottery.Round, 0)
	eventIDs := make([]int64, 0)

	for _, ev := range deriveEvents(payload) {
		evPayload := rawPayload
		if len(ev.Payload) > 0 {
			evPayload, _ = json.Marshal(map[string]interface{}{
				"derived": ev.Payload,
				"raw":     payload,
			})
		}

		event, err := h.events.Create(ctx, userID, "gsi", ev.Type, evPayload)
		if err != nil {
			return nil, nil, "", false, err
		}
		stored = append(stored, event)
		eventIDs = append(eventIDs, event.ID)

		if ev.Type == "ace" || ev.Type == "headshot" || ev.Type == "bomb_plant" {
			round, err := h.lottery.TriggerFromGameEvent(ctx, ev.Type, &event.ID, 100)
			if err == nil && round != nil {
				triggeredRounds = append(triggeredRounds, *round)
			}
		}
		if userID != nil && h.stream != nil {
			streamRounds, err := h.stream.HandleGameEvent(ctx, *userID, ev.Type, &event.ID)
			if err == nil && len(streamRounds) > 0 {
				triggeredRounds = append(triggeredRounds, streamRounds...)
			}
		}
	}

	_ = h.attachEventIDs(ctx, packetHash, eventIDs)
	return stored, triggeredRounds, packetHash, false, nil
}

func fakePayloadForEvent(eventType string) map[string]interface{} {
	switch eventType {
	case "headshot":
		return map[string]interface{}{
			"player": map[string]interface{}{
				"state": map[string]interface{}{
					"round_kills":  1,
					"round_killhs": 1,
					"health":       100,
				},
			},
			"round": map[string]interface{}{"phase": "live"},
		}
	case "bomb_plant":
		return map[string]interface{}{
			"player": map[string]interface{}{
				"state": map[string]interface{}{
					"round_kills":  0,
					"round_killhs": 0,
					"health":       100,
				},
			},
			"round": map[string]interface{}{"phase": "live", "bomb": "planted"},
		}
	case "round_win":
		return map[string]interface{}{
			"player": map[string]interface{}{
				"state": map[string]interface{}{
					"round_kills":  1,
					"round_killhs": 0,
					"health":       100,
				},
			},
			"round": map[string]interface{}{"phase": "over"},
		}
	case "kill":
		return map[string]interface{}{
			"player": map[string]interface{}{
				"state": map[string]interface{}{
					"round_kills":  1,
					"round_killhs": 0,
					"health":       100,
				},
			},
			"round": map[string]interface{}{"phase": "live"},
		}
	case "death":
		return map[string]interface{}{
			"player": map[string]interface{}{
				"state": map[string]interface{}{
					"round_kills":  0,
					"round_killhs": 0,
					"health":       0,
				},
			},
			"round": map[string]interface{}{"phase": "live"},
		}
	default: // ace
		return map[string]interface{}{
			"player": map[string]interface{}{
				"state": map[string]interface{}{
					"round_kills":  5,
					"round_killhs": 2,
					"health":       100,
				},
			},
			"round": map[string]interface{}{"phase": "live"},
		}
	}
}

func hashPayload(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func (h *Handler) insertPacketHash(ctx context.Context, packetHash string, userID *int64) (bool, error) {
	result, err := h.db.Exec(ctx, `
INSERT INTO gsi_packets (packet_hash, user_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING
`, packetHash, userID)
	if err != nil {
		return false, err
	}
	return result.RowsAffected() > 0, nil
}

func (h *Handler) attachEventIDs(ctx context.Context, packetHash string, eventIDs []int64) error {
	raw, err := json.Marshal(eventIDs)
	if err != nil {
		return err
	}
	_, err = h.db.Exec(ctx, `UPDATE gsi_packets SET event_ids = $2 WHERE packet_hash = $1`, packetHash, raw)
	return err
}
