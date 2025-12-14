#!/bin/bash
# 完整重建脚本

echo "=== 1. 停止所有容器 ==="
sudo docker compose down

echo "=== 2. 修复数据库表 ==="
sudo docker compose up -d db
sleep 5

# 等待数据库就绪
until sudo docker exec team-invite-db pg_isready -U postgres; do
  echo "等待数据库..."
  sleep 2
done

# 创建 team_accounts 表
sudo docker exec -i team-invite-db psql -U postgres -d team_invite << 'EOF'
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
EOF

echo "=== 3. 重新构建所有容器 ==="
sudo docker compose build --no-cache

echo "=== 4. 启动所有服务 ==="
sudo docker compose up -d

echo "=== 5. 查看容器状态 ==="
sudo docker compose ps

echo "=== 完成！==="
echo "前端: http://localhost:37013"
echo "后台: http://localhost:37013/admin"
