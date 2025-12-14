CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    linuxdo_id TEXT UNIQUE NOT NULL,
    username TEXT NOT NULL,
    name TEXT,
    avatar_template TEXT,
    trust_level INTEGER DEFAULT 0,
    active BOOLEAN DEFAULT TRUE,
    silenced BOOLEAN DEFAULT FALSE,
    chances INTEGER DEFAULT 1,
    invite_status SMALLINT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS spin_records (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES users(id),
    username TEXT NOT NULL,
    prize TEXT NOT NULL,
    status TEXT NOT NULL,
    detail TEXT,
    spin_id TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS invite_codes (
    id BIGSERIAL PRIMARY KEY,
    code TEXT UNIQUE NOT NULL,
    used BOOLEAN DEFAULT FALSE,
    used_email TEXT,
    user_id BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    used_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_users_status ON users(invite_status);
CREATE INDEX IF NOT EXISTS idx_spin_records_user_id ON spin_records(user_id);
CREATE INDEX IF NOT EXISTS idx_invite_codes_user_id ON invite_codes(user_id);

CREATE TABLE IF NOT EXISTS team_accounts (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    account_id TEXT NOT NULL,
    auth_token TEXT NOT NULL,
    max_seats INTEGER DEFAULT 50,
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

ALTER TABLE invite_codes ADD COLUMN IF NOT EXISTS team_account_id BIGINT REFERENCES team_accounts(id);

INSERT INTO config (key, value) VALUES ('quota', '0')
    ON CONFLICT (key) DO NOTHING;
