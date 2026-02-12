package cases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/2006michigun2006-hub/cs2-livedrop/internal/inventory"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/lottery"
	"github.com/2006michigun2006-hub/cs2-livedrop/internal/wallet"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	db        *pgxpool.Pool
	wallet    *wallet.Service
	lottery   *lottery.Service
	inventory *inventory.Service
}

type Case struct {
	ID                 int64     `json:"id"`
	StreamerID         int64     `json:"streamer_id"`
	StreamSessionID    *int64    `json:"stream_session_id,omitempty"`
	Title              string    `json:"title"`
	Description        string    `json:"description"`
	RewardItemType     string    `json:"reward_item_type"`
	RewardItemName     string    `json:"reward_item_name"`
	TargetAmountCents  int64     `json:"target_amount_cents"`
	CurrentAmountCents int64     `json:"current_amount_cents"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type CrowdfundingView struct {
	Case                Case       `json:"case"`
	TotalContributors   int64      `json:"total_contributors"`
	TotalRaisedCents    int64      `json:"total_raised_cents"`
	LeftCents           int64      `json:"left_cents"`
	ProgressPercent     float64    `json:"progress_percent"`
	MyContributionCents int64      `json:"my_contribution_cents"`
	MyChancePercent     float64    `json:"my_chance_percent"`
	LastWinnerUserID    *int64     `json:"last_winner_user_id,omitempty"`
	LastWinnerAt        *time.Time `json:"last_winner_at,omitempty"`
}

func NewService(db *pgxpool.Pool, wallet *wallet.Service, lottery *lottery.Service, inventory *inventory.Service) *Service {
	return &Service{db: db, wallet: wallet, lottery: lottery, inventory: inventory}
}

func (s *Service) Create(ctx context.Context, streamerID int64, streamSessionID *int64, title, description, rewardItemType, rewardItemName string, targetAmountCents int64) (Case, error) {
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	rewardItemType = normalizeRewardType(rewardItemType)
	rewardItemName = strings.TrimSpace(rewardItemName)
	if title == "" || targetAmountCents <= 0 {
		return Case{}, errors.New("title and target_amount_cents are required")
	}
	if rewardItemType == "" {
		return Case{}, errors.New("reward_item_type must be case or skin")
	}
	if rewardItemName == "" {
		if rewardItemType == "case" {
			rewardItemName = "Revolution Case"
		} else {
			rewardItemName = "AK-47 | Slate"
		}
	}

	if streamSessionID != nil {
		var valid bool
		err := s.db.QueryRow(ctx, `
SELECT EXISTS(
	SELECT 1 FROM stream_sessions
	WHERE id = $1 AND streamer_id = $2 AND status = 'active'
)
`, *streamSessionID, streamerID).Scan(&valid)
		if err != nil {
			return Case{}, err
		}
		if !valid {
			return Case{}, errors.New("stream_session_id is invalid or not active")
		}
	}

	var c Case
	err := s.db.QueryRow(ctx, `
INSERT INTO cases (streamer_id, stream_session_id, title, description, reward_item_type, reward_item_name, target_amount_cents)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, streamer_id, stream_session_id, title, description, reward_item_type, reward_item_name, target_amount_cents, current_amount_cents, status, created_at, updated_at
`, streamerID, streamSessionID, title, description, rewardItemType, rewardItemName, targetAmountCents).Scan(
		&c.ID, &c.StreamerID, &c.StreamSessionID, &c.Title, &c.Description, &c.RewardItemType, &c.RewardItemName, &c.TargetAmountCents, &c.CurrentAmountCents, &c.Status, &c.CreatedAt, &c.UpdatedAt,
	)
	return c, err
}

func (s *Service) List(ctx context.Context, limit int) ([]Case, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}

	rows, err := s.db.Query(ctx, `
SELECT id, streamer_id, stream_session_id, title, description, reward_item_type, reward_item_name, target_amount_cents, current_amount_cents, status, created_at, updated_at
FROM cases
ORDER BY created_at DESC
LIMIT $1
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cases := make([]Case, 0)
	for rows.Next() {
		var c Case
		if err := rows.Scan(&c.ID, &c.StreamerID, &c.StreamSessionID, &c.Title, &c.Description, &c.RewardItemType, &c.RewardItemName, &c.TargetAmountCents, &c.CurrentAmountCents, &c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		cases = append(cases, c)
	}
	return cases, rows.Err()
}

func (s *Service) ListBySession(ctx context.Context, streamSessionID int64) ([]Case, error) {
	rows, err := s.db.Query(ctx, `
SELECT id, streamer_id, stream_session_id, title, description, reward_item_type, reward_item_name, target_amount_cents, current_amount_cents, status, created_at, updated_at
FROM cases
WHERE stream_session_id = $1
ORDER BY created_at DESC
`, streamSessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cases := make([]Case, 0)
	for rows.Next() {
		var c Case
		if err := rows.Scan(&c.ID, &c.StreamerID, &c.StreamSessionID, &c.Title, &c.Description, &c.RewardItemType, &c.RewardItemName, &c.TargetAmountCents, &c.CurrentAmountCents, &c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		cases = append(cases, c)
	}
	return cases, rows.Err()
}

func (s *Service) Update(ctx context.Context, caseID int64, streamerID int64, title, description, rewardItemType, rewardItemName string, targetAmountCents int64) (Case, error) {
	title = strings.TrimSpace(title)
	rewardItemType = normalizeRewardType(rewardItemType)
	rewardItemName = strings.TrimSpace(rewardItemName)
	if title == "" || targetAmountCents <= 0 {
		return Case{}, errors.New("title and target_amount_cents are required")
	}
	if rewardItemType == "" {
		return Case{}, errors.New("reward_item_type must be case or skin")
	}
	if rewardItemName == "" {
		if rewardItemType == "case" {
			rewardItemName = "Revolution Case"
		} else {
			rewardItemName = "AK-47 | Slate"
		}
	}

	result, err := s.db.Exec(ctx, `
UPDATE cases
SET title = $1, description = $2, reward_item_type = $3, reward_item_name = $4, target_amount_cents = $5, updated_at = NOW()
WHERE id = $6 AND streamer_id = $7 AND status = 'open'
`, title, description, rewardItemType, rewardItemName, targetAmountCents, caseID, streamerID)
	if err != nil {
		return Case{}, err
	}
	if result.RowsAffected() == 0 {
		return Case{}, errors.New("case not found or cannot be updated")
	}

	return s.GetByID(ctx, caseID)
}

func (s *Service) Delete(ctx context.Context, caseID int64, streamerID int64) error {
	result, err := s.db.Exec(ctx, `DELETE FROM cases WHERE id = $1 AND streamer_id = $2 AND status = 'open'`, caseID, streamerID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errors.New("case not found or cannot be deleted")
	}
	return nil
}

func (s *Service) Contribute(ctx context.Context, caseID int64, userID int64, amountCents int64) (Case, *lottery.Round, *inventory.Item, error) {
	if amountCents <= 0 {
		return Case{}, nil, nil, errors.New("amount_cents must be positive")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Case{}, nil, nil, err
	}
	defer tx.Rollback(ctx)

	var c Case
	if err := tx.QueryRow(ctx, `
SELECT id, streamer_id, stream_session_id, title, description, reward_item_type, reward_item_name, target_amount_cents, current_amount_cents, status, created_at, updated_at
FROM cases
WHERE id = $1
FOR UPDATE
`, caseID).Scan(
		&c.ID, &c.StreamerID, &c.StreamSessionID, &c.Title, &c.Description, &c.RewardItemType, &c.RewardItemName, &c.TargetAmountCents, &c.CurrentAmountCents, &c.Status, &c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return Case{}, nil, nil, err
	}
	if c.Status != "open" {
		return Case{}, nil, nil, errors.New("case is not open for funding")
	}
	if c.StreamSessionID != nil {
		var joined bool
		if err := tx.QueryRow(ctx, `
SELECT EXISTS(
	SELECT 1 FROM stream_participants
	WHERE stream_session_id = $1 AND user_id = $2
)
`, *c.StreamSessionID, userID).Scan(&joined); err != nil {
			return Case{}, nil, nil, err
		}
		if !joined {
			return Case{}, nil, nil, errors.New("join stream invite first to donate in this campaign")
		}
	}

	if _, err := s.wallet.AdjustBalance(ctx, tx, userID, -amountCents, "case_contribution", map[string]interface{}{"case_id": caseID}); err != nil {
		return Case{}, nil, nil, err
	}

	if _, err := tx.Exec(ctx, `INSERT INTO case_contributions (case_id, user_id, amount_cents) VALUES ($1, $2, $3)`, caseID, userID, amountCents); err != nil {
		return Case{}, nil, nil, err
	}

	c.CurrentAmountCents += amountCents
	newStatus := c.Status
	if c.CurrentAmountCents >= c.TargetAmountCents {
		newStatus = "funded"
	}

	if _, err := tx.Exec(ctx, `
UPDATE cases
SET current_amount_cents = $1, status = $2, updated_at = NOW()
WHERE id = $3
`, c.CurrentAmountCents, newStatus, caseID); err != nil {
		return Case{}, nil, nil, err
	}

	c.Status = newStatus
	if err := tx.Commit(ctx); err != nil {
		return Case{}, nil, nil, err
	}

	var round *lottery.Round
	var rewardItem *inventory.Item
	if c.Status == "funded" {
		r, err := s.lottery.DrawForCase(ctx, caseID, c.CurrentAmountCents, c.StreamSessionID)
		if err != nil {
			return c, nil, nil, err
		}
		round = &r
		c.Status = "closed"

		if s.inventory != nil && r.WinnerUserID != nil {
			awarded, err := s.grantCrowdfundingReward(ctx, *r.WinnerUserID, c)
			if err == nil {
				rewardItem = &awarded
			}
		}
	}

	latest, err := s.GetByID(ctx, caseID)
	if err == nil {
		c = latest
	}

	return c, round, rewardItem, nil
}

func (s *Service) GetByID(ctx context.Context, caseID int64) (Case, error) {
	var c Case
	err := s.db.QueryRow(ctx, `
SELECT id, streamer_id, stream_session_id, title, description, reward_item_type, reward_item_name, target_amount_cents, current_amount_cents, status, created_at, updated_at
FROM cases
WHERE id = $1
`, caseID).Scan(
		&c.ID, &c.StreamerID, &c.StreamSessionID, &c.Title, &c.Description, &c.RewardItemType, &c.RewardItemName, &c.TargetAmountCents, &c.CurrentAmountCents, &c.Status, &c.CreatedAt, &c.UpdatedAt,
	)
	return c, err
}

func (s *Service) GetCampaignByInvite(ctx context.Context, inviteCode string, userID int64) (CrowdfundingView, error) {
	inviteCode = strings.TrimSpace(inviteCode)
	if inviteCode == "" {
		return CrowdfundingView{}, errors.New("invite code is required")
	}

	var campaign Case
	err := s.db.QueryRow(ctx, `
SELECT c.id, c.streamer_id, c.stream_session_id, c.title, c.description, c.reward_item_type, c.reward_item_name, c.target_amount_cents, c.current_amount_cents, c.status, c.created_at, c.updated_at
FROM cases c
JOIN stream_sessions ss ON ss.id = c.stream_session_id
WHERE ss.invite_code = $1
ORDER BY c.created_at DESC
LIMIT 1
`, inviteCode).Scan(
		&campaign.ID, &campaign.StreamerID, &campaign.StreamSessionID, &campaign.Title, &campaign.Description, &campaign.RewardItemType, &campaign.RewardItemName, &campaign.TargetAmountCents, &campaign.CurrentAmountCents, &campaign.Status, &campaign.CreatedAt, &campaign.UpdatedAt,
	)
	if err != nil {
		return CrowdfundingView{}, err
	}

	var totalContrib int64
	if err := s.db.QueryRow(ctx, `SELECT COALESCE(SUM(amount_cents), 0) FROM case_contributions WHERE case_id = $1`, campaign.ID).Scan(&totalContrib); err != nil {
		return CrowdfundingView{}, err
	}

	var totalContributors int64
	if err := s.db.QueryRow(ctx, `SELECT COUNT(DISTINCT user_id) FROM case_contributions WHERE case_id = $1`, campaign.ID).Scan(&totalContributors); err != nil {
		return CrowdfundingView{}, err
	}

	var mine int64
	if userID > 0 {
		_ = s.db.QueryRow(ctx, `SELECT COALESCE(SUM(amount_cents), 0) FROM case_contributions WHERE case_id = $1 AND user_id = $2`, campaign.ID, userID).Scan(&mine)
	}

	view := CrowdfundingView{
		Case:                campaign,
		TotalContributors:   totalContributors,
		TotalRaisedCents:    totalContrib,
		MyContributionCents: mine,
	}

	left := campaign.TargetAmountCents - totalContrib
	if left < 0 {
		left = 0
	}
	view.LeftCents = left
	if campaign.TargetAmountCents > 0 {
		view.ProgressPercent = float64(totalContrib) * 100 / float64(campaign.TargetAmountCents)
		if view.ProgressPercent > 100 {
			view.ProgressPercent = 100
		}
	}
	if totalContrib > 0 && mine > 0 {
		view.MyChancePercent = float64(mine) * 100 / float64(totalContrib)
	}

	var winnerID int64
	var winnerAt time.Time
	err = s.db.QueryRow(ctx, `
SELECT winner_user_id, created_at
FROM lottery_rounds
WHERE case_id = $1 AND winner_user_id IS NOT NULL
ORDER BY created_at DESC
LIMIT 1
`, campaign.ID).Scan(&winnerID, &winnerAt)
	if err == nil {
		view.LastWinnerUserID = &winnerID
		view.LastWinnerAt = &winnerAt
	}

	return view, nil
}

func (s *Service) grantCrowdfundingReward(ctx context.Context, userID int64, c Case) (inventory.Item, error) {
	rewardType := normalizeRewardType(c.RewardItemType)
	rewardName := strings.TrimSpace(c.RewardItemName)
	if rewardType == "" {
		rewardType = "case"
	}
	if rewardName == "" {
		if rewardType == "case" {
			rewardName = "Revolution Case"
		} else {
			rewardName = "AK-47 | Slate"
		}
	}
	if rewardType == "skin" && strings.Contains(strings.ToLower(rewardName), "random") {
		rewardName = "AK-47 | Slate"
	}
	return s.inventory.GrantItem(ctx, userID, rewardType, rewardName, "restricted", "crowdfunding_reward", map[string]interface{}{
		"case_id":             c.ID,
		"stream_session_id":   c.StreamSessionID,
		"reward_item_type":    rewardType,
		"reward_item_name":    rewardName,
		"target_amount_cents": c.TargetAmountCents,
	})
}

func normalizeRewardType(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "case" || v == "skin" {
		return v
	}
	return ""
}
