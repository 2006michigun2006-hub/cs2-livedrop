package lottery

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"time"

	"github.com/2006michigun2006-hub/cs2-livedrop/internal/wallet"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	db     *pgxpool.Pool
	wallet *wallet.Service
}

type Round struct {
	ID              int64           `json:"id"`
	TriggerEvent    *int64          `json:"trigger_event_id,omitempty"`
	CaseID          *int64          `json:"case_id,omitempty"`
	StreamSessionID *int64          `json:"stream_session_id,omitempty"`
	WinnerUserID    *int64          `json:"winner_user_id,omitempty"`
	TriggerType     string          `json:"trigger_type"`
	PrizeCents      int64           `json:"prize_cents"`
	Details         json.RawMessage `json:"details"`
	CreatedAt       time.Time       `json:"created_at"`
}

type weightedUser struct {
	UserID int64
	Weight int64
}

func NewService(db *pgxpool.Pool, wallet *wallet.Service) *Service {
	return &Service{db: db, wallet: wallet}
}

func (s *Service) Join(ctx context.Context, userID int64, scoreDelta int64) error {
	if scoreDelta <= 0 {
		scoreDelta = 1
	}
	_, err := s.db.Exec(ctx, `
INSERT INTO viewer_activity (user_id, score, updated_at)
VALUES ($1, $2, NOW())
ON CONFLICT (user_id)
DO UPDATE SET score = viewer_activity.score + EXCLUDED.score, updated_at = NOW()
`, userID, scoreDelta)
	return err
}

func (s *Service) RecordActivity(ctx context.Context, userID int64, scoreDelta int64) error {
	return s.Join(ctx, userID, scoreDelta)
}

func (s *Service) TriggerFromGameEvent(ctx context.Context, triggerType string, triggerEventID *int64, prizeCents int64) (*Round, error) {
	candidates, err := s.loadCandidates(ctx)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	winnerID, err := chooseWinner(candidates)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if prizeCents > 0 {
		if _, err := s.wallet.AdjustBalance(ctx, tx, winnerID, prizeCents, "lottery_reward", map[string]interface{}{"trigger_type": triggerType}); err != nil {
			return nil, err
		}
	}

	details, _ := json.Marshal(map[string]interface{}{"candidates": len(candidates)})
	round, err := s.insertRound(ctx, tx, triggerEventID, nil, nil, &winnerID, triggerType, prizeCents, details)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &round, nil
}

func (s *Service) TriggerForUsers(ctx context.Context, triggerType string, triggerEventID, streamSessionID *int64, prizeCents int64, userIDs []int64, extraDetails map[string]interface{}) (*Round, error) {
	candidates, err := s.loadCandidatesByUsers(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	winnerID, err := chooseWinner(candidates)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if prizeCents > 0 {
		if _, err := s.wallet.AdjustBalance(ctx, tx, winnerID, prizeCents, "stream_giveaway_reward", map[string]interface{}{"trigger_type": triggerType}); err != nil {
			return nil, err
		}
	}

	if extraDetails == nil {
		extraDetails = map[string]interface{}{}
	}
	extraDetails["candidates"] = len(candidates)
	extraDetails["stream_session_id"] = streamSessionID
	details, _ := json.Marshal(extraDetails)

	round, err := s.insertRound(ctx, tx, triggerEventID, nil, streamSessionID, &winnerID, triggerType, prizeCents, details)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &round, nil
}

func (s *Service) DrawForCase(ctx context.Context, caseID int64, potCents int64, streamSessionID *int64) (Round, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Round{}, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
SELECT user_id, SUM(amount_cents) AS weight
FROM case_contributions
WHERE case_id = $1
GROUP BY user_id
`, caseID)
	if err != nil {
		return Round{}, err
	}
	defer rows.Close()

	candidates := make([]weightedUser, 0)
	for rows.Next() {
		var c weightedUser
		if err := rows.Scan(&c.UserID, &c.Weight); err != nil {
			return Round{}, err
		}
		if c.Weight > 0 {
			candidates = append(candidates, c)
		}
	}
	if len(candidates) == 0 {
		return Round{}, errors.New("no contributors to draw from")
	}

	winnerID, err := chooseWinner(candidates)
	if err != nil {
		return Round{}, err
	}

	if _, err := tx.Exec(ctx, `UPDATE cases SET status = 'closed', updated_at = NOW() WHERE id = $1`, caseID); err != nil {
		return Round{}, err
	}

	details, _ := json.Marshal(map[string]interface{}{"draw": "crowdfunding_case", "contributors": len(candidates)})
	round, err := s.insertRound(ctx, tx, nil, &caseID, streamSessionID, &winnerID, "case_funded", potCents, details)
	if err != nil {
		return Round{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Round{}, err
	}

	return round, nil
}

func (s *Service) ListRounds(ctx context.Context, limit int) ([]Round, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}

	rows, err := s.db.Query(ctx, `
SELECT id, trigger_event_id, case_id, stream_session_id, winner_user_id, trigger_type, prize_cents, details, created_at
FROM lottery_rounds
ORDER BY created_at DESC
LIMIT $1
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rounds := make([]Round, 0)
	for rows.Next() {
		var r Round
		if err := rows.Scan(&r.ID, &r.TriggerEvent, &r.CaseID, &r.StreamSessionID, &r.WinnerUserID, &r.TriggerType, &r.PrizeCents, &r.Details, &r.CreatedAt); err != nil {
			return nil, err
		}
		rounds = append(rounds, r)
	}
	return rounds, rows.Err()
}

func (s *Service) loadCandidates(ctx context.Context) ([]weightedUser, error) {
	rows, err := s.db.Query(ctx, `
SELECT va.user_id, GREATEST(1, va.score + COALESCE(SUM(cc.amount_cents) / 100, 0)) AS weight
FROM viewer_activity va
LEFT JOIN case_contributions cc ON cc.user_id = va.user_id
WHERE va.updated_at > NOW() - INTERVAL '24 hours'
GROUP BY va.user_id, va.score
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candidates := make([]weightedUser, 0)
	for rows.Next() {
		var c weightedUser
		if err := rows.Scan(&c.UserID, &c.Weight); err != nil {
			return nil, err
		}
		if c.Weight > 0 {
			candidates = append(candidates, c)
		}
	}
	return candidates, rows.Err()
}

func (s *Service) loadCandidatesByUsers(ctx context.Context, userIDs []int64) ([]weightedUser, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
SELECT u.id, GREATEST(1, COALESCE(va.score, 0) + COALESCE(contrib.total / 100, 0)) AS weight
FROM users u
LEFT JOIN viewer_activity va ON va.user_id = u.id
LEFT JOIN (
    SELECT user_id, SUM(amount_cents) AS total
    FROM case_contributions
    GROUP BY user_id
) contrib ON contrib.user_id = u.id
WHERE u.id = ANY($1)
`, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candidates := make([]weightedUser, 0)
	for rows.Next() {
		var c weightedUser
		if err := rows.Scan(&c.UserID, &c.Weight); err != nil {
			return nil, err
		}
		if c.Weight > 0 {
			candidates = append(candidates, c)
		}
	}
	return candidates, rows.Err()
}

func (s *Service) insertRound(ctx context.Context, tx pgx.Tx, triggerEventID, caseID, streamSessionID, winnerID *int64, triggerType string, prizeCents int64, details json.RawMessage) (Round, error) {
	var round Round
	err := tx.QueryRow(ctx, `
INSERT INTO lottery_rounds (trigger_event_id, case_id, stream_session_id, winner_user_id, trigger_type, prize_cents, details)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, trigger_event_id, case_id, stream_session_id, winner_user_id, trigger_type, prize_cents, details, created_at
`, triggerEventID, caseID, streamSessionID, winnerID, triggerType, prizeCents, details).Scan(
		&round.ID,
		&round.TriggerEvent,
		&round.CaseID,
		&round.StreamSessionID,
		&round.WinnerUserID,
		&round.TriggerType,
		&round.PrizeCents,
		&round.Details,
		&round.CreatedAt,
	)
	return round, err
}

func chooseWinner(candidates []weightedUser) (int64, error) {
	totalWeight := int64(0)
	for _, candidate := range candidates {
		totalWeight += candidate.Weight
	}
	if totalWeight <= 0 {
		return 0, errors.New("invalid total weight")
	}

	r, err := rand.Int(rand.Reader, big.NewInt(totalWeight))
	if err != nil {
		return 0, err
	}
	threshold := r.Int64()

	running := int64(0)
	for _, candidate := range candidates {
		running += candidate.Weight
		if threshold < running {
			return candidate.UserID, nil
		}
	}

	return candidates[len(candidates)-1].UserID, nil
}
