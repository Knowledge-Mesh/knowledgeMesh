-- knowledgeMesh initial schema (buyers, sellers, models, billing, inference matches)

CREATE TABLE buyer_users (
	id UUID PRIMARY KEY,
	display_name TEXT NOT NULL,
	email TEXT NOT NULL,
	password_hash TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX buyer_users_email_lower ON buyer_users (lower(email));

CREATE TABLE seller_users (
	id UUID PRIMARY KEY,
	display_name TEXT NOT NULL,
	email TEXT NOT NULL,
	password_hash TEXT NOT NULL,
	on_duty BOOLEAN NOT NULL DEFAULT false,
	peer_id TEXT,
	status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX seller_users_email_lower ON seller_users (lower(email));

CREATE TABLE seller_models (
	id UUID PRIMARY KEY,
	seller_id UUID NOT NULL REFERENCES seller_users(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	version TEXT,
	skill_name TEXT NOT NULL,
	model_name TEXT NOT NULL,
	model_type TEXT,
	tuning_tier TEXT,
	hourly_tokens INT NOT NULL DEFAULT 0,
	daily_tokens INT NOT NULL DEFAULT 0,
	total_tokens INT NOT NULL DEFAULT 0,
	rate_per_token DOUBLE PRECISION NOT NULL DEFAULT 0,
	active BOOLEAN NOT NULL DEFAULT true,
	meta JSONB
);

CREATE INDEX idx_seller_models_seller_id ON seller_models (seller_id);

CREATE TABLE buyer_billing (
	buyer_id UUID PRIMARY KEY REFERENCES buyer_users(id) ON DELETE CASCADE,
	wallet_balance BIGINT NOT NULL DEFAULT 0,
	token_quota BIGINT NOT NULL DEFAULT 10000000,
	tokens_used BIGINT NOT NULL DEFAULT 0,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE seller_billing (
	seller_id UUID PRIMARY KEY REFERENCES seller_users(id) ON DELETE CASCADE,
	wallet_balance BIGINT NOT NULL DEFAULT 0,
	token_quota BIGINT NOT NULL DEFAULT 10000000,
	tokens_used BIGINT NOT NULL DEFAULT 0,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE billing_transactions (
	id UUID PRIMARY KEY,
	party_kind TEXT NOT NULL CHECK (party_kind IN ('buyer', 'seller')),
	party_id UUID NOT NULL,
	tx_type TEXT NOT NULL,
	amount BIGINT NOT NULL,
	balance_after BIGINT NOT NULL,
	details JSONB,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_billing_tx_party_created ON billing_transactions (party_kind, party_id, created_at DESC);

CREATE TABLE inference_matches (
	request_id TEXT PRIMARY KEY,
	buyer_id UUID NOT NULL REFERENCES buyer_users(id),
	seller_id UUID NOT NULL REFERENCES seller_users(id),
	seller_peer_id TEXT NOT NULL,
	estimated_tokens INT NOT NULL DEFAULT 0,
	settled BOOLEAN NOT NULL DEFAULT false,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_inference_matches_buyer ON inference_matches (buyer_id, created_at DESC);

-- Default billing rows for existing accounts (idempotent inserts)
INSERT INTO buyer_billing (buyer_id, wallet_balance, token_quota, tokens_used)
SELECT id, 1000000, 10000000, 0 FROM buyer_users
ON CONFLICT (buyer_id) DO NOTHING;

INSERT INTO seller_billing (seller_id, wallet_balance, token_quota, tokens_used)
SELECT id, 0, 10000000, 0 FROM seller_users
ON CONFLICT (seller_id) DO NOTHING;
