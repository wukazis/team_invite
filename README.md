# Team Invite (Go + SPA)

Team Invite 项目，后端使用 Go + Gin + PostgreSQL，前端为 React/Vite SPA，支持 Linux.do OAuth、JWT 鉴权与后台管理。

## 目录结构

- `cmd/server`：Go 主程序入口。
- `internal/`：配置、数据库、OAuth、邀请服务等核心逻辑。
- `frontend/`：Vite + React + TS 的 SPA，涵盖首页、登录、邀请码填写与后台管理页面。
- `db/schema.sql`：PostgreSQL 建表脚本。

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

## 初始化

1. 安装依赖：
   ```bash
   go mod tidy
   cd frontend && npm install
   ```
2. 初始化数据库：
   ```bash
   createdb team_invite
   psql "$POSTGRES_URL" -f db/schema.sql
   ```
3. 启动后端：
   ```bash
   go build ./cmd/server
   ./server
   ```
4. 启动前端 (开发)：
   ```bash
   cd frontend
   npm run dev
   ```
   生产可运行 `npm run build` 并将 `frontend/dist` 部署至 CDN/静态服务器。

## API 摘要

- `/api/oauth/login`：获取 Linux.do 授权 URL。
- `/api/state`：查询用户状态（需要用户 JWT）。
- `/api/invite` GET/POST：查询/提交邀请码。
- `/api/admin/*`：后台功能（管理员 JWT + IP 白名单 + TOTP）：
  - `/env` `.env` 读取/写入
  - `/users` 用户管理
  - `/invite-codes` 邀请码管理
  - `/team-accounts` 车账号管理

## 使用流程

1. 管理员在后台生成邀请码
2. 用户通过 Linux.do OAuth 登录
3. 用户输入邀请码和邮箱领取邀请
4. 系统自动发送邀请到用户邮箱
