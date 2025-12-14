-- 修复数据库：确保 team_accounts 表存在
CREATE TABLE IF NOT EXISTS team_accounts (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    account_id TEXT NOT NULL,
    auth_token TEXT NOT NULL,
    max_seats INTEGER DEFAULT 50,
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- 确保 invite_codes 表有 team_account_id 列
ALTER TABLE invite_codes ADD COLUMN IF NOT EXISTS team_account_id BIGINT REFERENCES team_accounts(id);
