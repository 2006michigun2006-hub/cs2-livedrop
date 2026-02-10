package wallet

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	db *pgxpool.Pool
}

type Transaction struct {
	ID          int64           `json:"id"`
	UserID      int64           `json:"user_id"`
	AmountCents int64           `json:"amount_cents"`
	Reason      string          `json:"reason"`
	Metadata    json.RawMessage `json:"metadata"`
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) GetBalance(ctx context.Context, userID int64) (int64, error) {
	var balance int64
	err := s.db.QueryRow(ctx, `SELECT balance_cents FROM users WHERE id = $1`, userID).Scan(&balance)
	return balance, err
}

func (s *Service) AdjustBalance(ctx context.Context, tx pgx.Tx, userID int64, amountCents int64, reason string, metadata map[string]interface{}) (int64, error) {
	if amountCents == 0 {
		return 0, errors.New("amount_cents must be non-zero")
	}
	if reason == "" {
		return 0, errors.New("reason is required")
	}
	if metadata == nil {
		metadata = map[string]interface{}{}
	}

	var current int64
	if err := tx.QueryRow(ctx, `SELECT balance_cents FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&current); err != nil {
		return 0, err
	}
	newBalance := current + amountCents
	if newBalance < 0 {
		return 0, errors.New("insufficient balance")
	}

	if _, err := tx.Exec(ctx, `UPDATE users SET balance_cents = $1 WHERE id = $2`, newBalance, userID); err != nil {
		return 0, err
	}

	rawMetadata, err := json.Marshal(metadata)
	if err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO wallet_transactions (user_id, amount_cents, reason, metadata)
VALUES ($1, $2, $3, $4)
`, userID, amountCents, reason, rawMetadata); err != nil {
		return 0, err
	}

	return newBalance, nil
}

func (s *Service) Credit(ctx context.Context, userID int64, amountCents int64, reason string, metadata map[string]interface{}) (int64, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	newBalance, err := s.AdjustBalance(ctx, tx, userID, amountCents, reason, metadata)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return newBalance, nil
}

func (s *Service) ListTransactions(ctx context.Context, userID int64, limit int) ([]Transaction, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}

	rows, err := s.db.Query(ctx, `
SELECT id, user_id, amount_cents, reason, metadata
FROM wallet_transactions
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2
`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	transactions := make([]Transaction, 0)
	for rows.Next() {
		var tx Transaction
		if err := rows.Scan(&tx.ID, &tx.UserID, &tx.AmountCents, &tx.Reason, &tx.Metadata); err != nil {
			return nil, err
		}
		transactions = append(transactions, tx)
	}
	return transactions, rows.Err()
}
