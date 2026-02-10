package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	if err := runMigrations(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    username TEXT UNIQUE,
    email TEXT UNIQUE,
    password_hash TEXT,
    steam_id TEXT UNIQUE,
    telegram_id TEXT UNIQUE,
    telegram_username TEXT,
    role TEXT NOT NULL DEFAULT 'viewer',
    balance_cents BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (username IS NOT NULL OR steam_id IS NOT NULL),
    CHECK (email IS NOT NULL OR steam_id IS NOT NULL)
);

CREATE TABLE IF NOT EXISTS events (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    source TEXT NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_events_created_at ON events (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_user_id ON events (user_id);

ALTER TABLE users ADD COLUMN IF NOT EXISTS balance_cents BIGINT NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS telegram_id TEXT UNIQUE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS telegram_username TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'viewer';

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_check') THEN
        ALTER TABLE users DROP CONSTRAINT users_check;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_check1') THEN
        ALTER TABLE users DROP CONSTRAINT users_check1;
    END IF;
END$$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_identity_check') THEN
        ALTER TABLE users
        ADD CONSTRAINT users_identity_check CHECK (username IS NOT NULL OR steam_id IS NOT NULL OR telegram_id IS NOT NULL);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_contact_check') THEN
        ALTER TABLE users
        ADD CONSTRAINT users_contact_check CHECK (email IS NOT NULL OR steam_id IS NOT NULL OR telegram_id IS NOT NULL);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_role_check') THEN
        ALTER TABLE users
        ADD CONSTRAINT users_role_check CHECK (role IN ('viewer', 'streamer', 'admin'));
    END IF;
END$$;

CREATE TABLE IF NOT EXISTS wallet_transactions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount_cents BIGINT NOT NULL,
    reason TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wallet_transactions_user_id ON wallet_transactions (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS cases (
    id BIGSERIAL PRIMARY KEY,
    streamer_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    stream_session_id BIGINT,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    reward_item_type TEXT NOT NULL DEFAULT 'case',
    reward_item_name TEXT NOT NULL DEFAULT 'Revolution Case',
    target_amount_cents BIGINT NOT NULL CHECK (target_amount_cents > 0),
    current_amount_cents BIGINT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'open',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cases_streamer_id ON cases (streamer_id);
ALTER TABLE cases ADD COLUMN IF NOT EXISTS stream_session_id BIGINT;
ALTER TABLE cases ADD COLUMN IF NOT EXISTS reward_item_type TEXT NOT NULL DEFAULT 'case';
ALTER TABLE cases ADD COLUMN IF NOT EXISTS reward_item_name TEXT NOT NULL DEFAULT 'Revolution Case';
UPDATE cases
SET reward_item_name = 'AK-47 | Slate'
WHERE reward_item_type = 'skin' AND LOWER(COALESCE(reward_item_name, '')) LIKE '%random%';
CREATE INDEX IF NOT EXISTS idx_cases_stream_session_id ON cases (stream_session_id, created_at DESC);

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'cases_reward_item_type_check') THEN
        ALTER TABLE cases DROP CONSTRAINT cases_reward_item_type_check;
    END IF;
    ALTER TABLE cases
    ADD CONSTRAINT cases_reward_item_type_check CHECK (reward_item_type IN ('case', 'skin'));
END$$;

CREATE TABLE IF NOT EXISTS case_contributions (
    id BIGSERIAL PRIMARY KEY,
    case_id BIGINT NOT NULL REFERENCES cases(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_case_contrib_case_id ON case_contributions (case_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_case_contrib_user_id ON case_contributions (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS viewer_activity (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    score BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS lottery_rounds (
    id BIGSERIAL PRIMARY KEY,
    trigger_event_id BIGINT REFERENCES events(id) ON DELETE SET NULL,
    case_id BIGINT REFERENCES cases(id) ON DELETE SET NULL,
    stream_session_id BIGINT,
    winner_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    trigger_type TEXT NOT NULL,
    prize_cents BIGINT NOT NULL DEFAULT 0,
    details JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lottery_rounds_created_at ON lottery_rounds (created_at DESC);
ALTER TABLE lottery_rounds ADD COLUMN IF NOT EXISTS stream_session_id BIGINT;

CREATE TABLE IF NOT EXISTS gsi_packets (
    packet_hash TEXT PRIMARY KEY,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    event_ids JSONB NOT NULL DEFAULT '[]'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS stream_sessions (
    id BIGSERIAL PRIMARY KEY,
    streamer_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    invite_code TEXT NOT NULL UNIQUE,
    telegram_chat_id TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_stream_sessions_streamer_id ON stream_sessions (streamer_id, created_at DESC);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'cases_stream_session_fk') THEN
        ALTER TABLE cases
        ADD CONSTRAINT cases_stream_session_fk FOREIGN KEY (stream_session_id) REFERENCES stream_sessions(id) ON DELETE SET NULL;
    END IF;
END$$;

CREATE TABLE IF NOT EXISTS stream_participants (
    stream_session_id BIGINT NOT NULL REFERENCES stream_sessions(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (stream_session_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_stream_participants_user_id ON stream_participants (user_id, joined_at DESC);

CREATE TABLE IF NOT EXISTS giveaway_rules (
    id BIGSERIAL PRIMARY KEY,
    stream_session_id BIGINT NOT NULL REFERENCES stream_sessions(id) ON DELETE CASCADE,
    trigger_type TEXT NOT NULL,
    prize_type TEXT NOT NULL DEFAULT 'skin',
    prize_name TEXT NOT NULL,
    prize_cents BIGINT NOT NULL DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_giveaway_rules_session_trigger ON giveaway_rules (stream_session_id, trigger_type, enabled);
ALTER TABLE giveaway_rules ADD COLUMN IF NOT EXISTS prize_type TEXT NOT NULL DEFAULT 'skin';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'giveaway_rules_prize_type_check') THEN
        ALTER TABLE giveaway_rules
        ADD CONSTRAINT giveaway_rules_prize_type_check CHECK (prize_type IN ('skin', 'case'));
    END IF;
END$$;

CREATE TABLE IF NOT EXISTS inventory_items (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    item_type TEXT NOT NULL,
    name TEXT NOT NULL,
    rarity TEXT NOT NULL DEFAULT 'consumer',
    price_cents BIGINT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'available',
    source TEXT NOT NULL DEFAULT 'system',
    parent_item_id BIGINT REFERENCES inventory_items(id) ON DELETE SET NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    opened_at TIMESTAMPTZ,
    sold_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_inventory_user_created ON inventory_items (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_inventory_parent_item_id ON inventory_items (parent_item_id);
ALTER TABLE inventory_items ADD COLUMN IF NOT EXISTS price_cents BIGINT NOT NULL DEFAULT 0;
ALTER TABLE inventory_items ADD COLUMN IF NOT EXISTS sold_at TIMESTAMPTZ;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'inventory_status_check') THEN
        ALTER TABLE inventory_items DROP CONSTRAINT inventory_status_check;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'inventory_item_type_check') THEN
        ALTER TABLE inventory_items
        ADD CONSTRAINT inventory_item_type_check CHECK (item_type IN ('skin', 'case'));
    END IF;
    ALTER TABLE inventory_items
    ADD CONSTRAINT inventory_status_check CHECK (status IN ('available', 'unopened', 'opened', 'sold'));
END$$;
`)
	return err
}
