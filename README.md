# Team Invite (Go + SPA)

全新架构的 Team Invite 项目，后端使用 Go + Gin + PostgreSQL，前端为 React/Vite SPA，并保留 Linux.do OAuth、JWT 鉴权与 .env 在线编辑后台。

## 目录结构

- `cmd/server`：Go 主程序入口。
- `internal/`：配置、数据库、OAuth、抽奖服务、调度器等核心逻辑。
- `frontend/`：Vite + React + TS 的 SPA，涵盖首页抽奖、登录、邀请码填写与后台管理页面。
- `db/schema.sql`：PostgreSQL 建表脚本。
- `users.db/main.py`：保留的旧 Flask/SQLite 代码（可参考迁移逻辑）。

## 环境变量

`.env` 主要字段：

```
POSTGRES_URL=postgres://user:pass@host:5432/team_invite?sslmode=disable
JWT_SECRET=随机字符串
JWT_ISSUER=team-invite
ADMIN_PASSWORD=后台密码
ADMIN_TOTP_SECRET=管理员 TOTP 秘钥
ADMIN_ALLOWED_IPS=1.1.1.1,2.2.2.2   # (可选) 限制后台访问
LINUXDO_CLIENT_ID=...
LINUXDO_CLIENT_SECRET=...
LINUXDO_REDIRECT_URI=https://example.com/api/oauth/callback
APP_BASE_URL=https://spa.example.com
HTTP_PORT=8080
```

后台支持在线修改 `.env` 指定字段。

## 初始化

1. 安装依赖：
   ```bash
   cd /home/ubuntu/team-invite
   go mod tidy
   cd frontend && npm install
   ```
2. 初始化数据库：
   ```bash
   createdb team_invite         # 若需要
   psql "$POSTGRES_URL" -f db/schema.sql
   ```
3. 启动后端：
   ```bash
   cd /home/ubuntu/team-invite
   GOCACHE=.gocache go build ./cmd/server
   ./cmd/server
   ```
4. 启动前端 (开发)：
   ```bash
   cd frontend
   npm run dev
   ```
   生产可运行 `npm run build` 并将 `frontend/dist` 部署至 CDN/静态服务器。

## API 摘要

- `/api/oauth/login`：获取 Linux.do 授权 URL。
- `/api/state`：查询用户状态/次数（需要用户 JWT）。
- `/api/spins`：抽奖，内置幂等 `spinId`。
- `/api/invite` GET/POST：查询/提交中奖邀请码。
- `/api/quota/public`：匿名轮询剩余名额（前端每秒一次）。
- `/api/admin/*`：后台功能（管理员 JWT + IP 白名单 + TOTP）：
  - `/env` `.env` 读取/写入
  - `/quota` 立即调配名额
  - `/quota/schedule` 定时补额
  - `/users/reset` 重置未中奖用户次数并完成“待填写”状态
  - `/stats/hourly` 24 小时抽奖统计
  - `/prize-config` 中奖概率配置（含缓存）

## 其他说明

- 抽奖逻辑：`internal/services/draw` 负责，从缓存读取奖项概率；中奖会生成邀请码并把用户状态改为“待填写”，提交 invite 后置为“已完成”。
- `internal/scheduler/quota` 会按照 `config.quota_schedule` 定时补充名额，后台可配置延时。
- 前端 Home 页面在名额为 0 时按钮立即置灰，名额恢复后无需刷新即可启用，但保留手动刷新按钮提示。

如需进一步扩展（例如多节点部署、监控面板、SSE 实时推送等），可在现有 API/前端基础上继续迭代。
