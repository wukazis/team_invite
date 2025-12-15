package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"team-invite/internal/models"
	"team-invite/internal/oauth"
	"team-invite/internal/util/invitecode"
)

var (
	ErrAlreadyClaimed = errors.New("invite already claimed")
	ErrInviteAssigned = errors.New("invite already assigned")
	ErrInviteNotFound = errors.New("invite not found")
)

type Store struct {
	db       *sql.DB
	dbType   string // "postgres" or "sqlite"
}

func New(ctx context.Context, dsn string) (*Store, error) {
	var db *sql.DB
	var err error
	var dbType string

	if dsn == "" || strings.HasPrefix(dsn, "sqlite:") {
		// SQLite
		dbType = "sqlite"
		sqlitePath := "data.db"
		if strings.HasPrefix(dsn, "sqlite:") {
			sqlitePath = strings.TrimPrefix(dsn, "sqlite:")
		}
		db, err = sql.Open("sqlite", sqlitePath)
		if err != nil {
			return nil, fmt.Errorf("sqlite open: %w", err)
		}
		// Enable foreign keys
		if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
			return nil, err
		}
	} else {
		// PostgreSQL
		dbType = "postgres"
		db, err = sql.Open("pgx", dsn)
		if err != nil {
			return nil, fmt.Errorf("postgres open: %w", err)
		}
	}

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("db ping: %w", err)
	}

	store := &Store{db: db, dbType: dbType}
	if err := store.migrate(ctx); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return store, nil
}

func (s *Store) Close() {
	if s.db != nil {
		s.db.Close()
	}
}

func (s *Store) migrate(ctx context.Context) error {
	var schema string
	if s.dbType == "sqlite" {
		schema = sqliteSchema
	} else {
		schema = postgresSchema
	}
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// placeholder returns the correct placeholder for the database type
func (s *Store) ph(n int) string {
	if s.dbType == "sqlite" {
		return "?"
	}
	return fmt.Sprintf("$%d", n)
}

// autoIncrement returns the correct auto increment syntax
func (s *Store) autoInc() string {
	if s.dbType == "sqlite" {
		return "INTEGER PRIMARY KEY AUTOINCREMENT"
	}
	return "BIGSERIAL PRIMARY KEY"
}

// now returns the correct NOW() function
func (s *Store) now() string {
	if s.dbType == "sqlite" {
		return "datetime('now')"
	}
	return "NOW()"
}


const sqliteSchema = `
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    linuxdo_id TEXT UNIQUE NOT NULL,
    username TEXT NOT NULL,
    name TEXT,
    avatar_template TEXT,
    trust_level INTEGER DEFAULT 0,
    active INTEGER DEFAULT 1,
    silenced INTEGER DEFAULT 0,
    invite_status INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS team_accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    account_id TEXT NOT NULL,
    auth_token TEXT NOT NULL,
    max_seats INTEGER DEFAULT 50,
    enabled INTEGER DEFAULT 1,
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS invite_codes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    code TEXT UNIQUE NOT NULL,
    used INTEGER DEFAULT 0,
    used_email TEXT,
    user_id INTEGER REFERENCES users(id),
    team_account_id INTEGER REFERENCES team_accounts(id),
    created_at TEXT DEFAULT (datetime('now')),
    used_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_users_status ON users(invite_status);
CREATE INDEX IF NOT EXISTS idx_invite_codes_user_id ON invite_codes(user_id);
`

const postgresSchema = `
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    linuxdo_id TEXT UNIQUE NOT NULL,
    username TEXT NOT NULL,
    name TEXT,
    avatar_template TEXT,
    trust_level INTEGER DEFAULT 0,
    active BOOLEAN DEFAULT TRUE,
    silenced BOOLEAN DEFAULT FALSE,
    invite_status SMALLINT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS team_accounts (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    account_id TEXT NOT NULL,
    auth_token TEXT NOT NULL,
    max_seats INTEGER DEFAULT 50,
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS invite_codes (
    id BIGSERIAL PRIMARY KEY,
    code TEXT UNIQUE NOT NULL,
    used BOOLEAN DEFAULT FALSE,
    used_email TEXT,
    user_id BIGINT REFERENCES users(id),
    team_account_id BIGINT REFERENCES team_accounts(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    used_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_users_status ON users(invite_status);
CREATE INDEX IF NOT EXISTS idx_invite_codes_user_id ON invite_codes(user_id);
`


func (s *Store) UpsertUserFromOAuth(ctx context.Context, profile oauth.Profile) (*models.User, error) {
	var query string
	if s.dbType == "sqlite" {
		query = `
INSERT INTO users (username, name, avatar_template, trust_level, active, silenced, invite_status, linuxdo_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (linuxdo_id) DO UPDATE
SET username = excluded.username,
    name = excluded.name,
    avatar_template = excluded.avatar_template,
    trust_level = excluded.trust_level,
    active = excluded.active,
    silenced = excluded.silenced,
    updated_at = datetime('now')
RETURNING id, username, name, avatar_template, trust_level, active, silenced, invite_status, created_at, updated_at`
	} else {
		query = `
INSERT INTO users (username, name, avatar_template, trust_level, active, silenced, invite_status, linuxdo_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (linuxdo_id) DO UPDATE
SET username = EXCLUDED.username,
    name = EXCLUDED.name,
    avatar_template = EXCLUDED.avatar_template,
    trust_level = EXCLUDED.trust_level,
    active = EXCLUDED.active,
    silenced = EXCLUDED.silenced,
    updated_at = NOW()
RETURNING id, username, name, avatar_template, trust_level, active, silenced, invite_status, created_at, updated_at`
	}

	row := s.db.QueryRowContext(ctx, query,
		profile.Username,
		profile.Name,
		profile.AvatarTemplate,
		profile.TrustLevel,
		profile.Active,
		profile.Silenced,
		models.UserStatusNone,
		profile.ID,
	)
	var u models.User
	if err := row.Scan(
		&u.ID,
		&u.Username,
		&u.DisplayName,
		&u.AvatarTemplate,
		&u.TrustLevel,
		&u.Active,
		&u.Silenced,
		&u.InviteStatus,
		&u.CreatedAt,
		&u.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	var query string
	if s.dbType == "sqlite" {
		query = `SELECT id, username, name, avatar_template, trust_level, active, silenced, invite_status, created_at, updated_at FROM users WHERE id = ?`
	} else {
		query = `SELECT id, username, name, avatar_template, trust_level, active, silenced, invite_status, created_at, updated_at FROM users WHERE id = $1`
	}
	row := s.db.QueryRowContext(ctx, query, id)
	var u models.User
	if err := row.Scan(
		&u.ID,
		&u.Username,
		&u.DisplayName,
		&u.AvatarTemplate,
		&u.TrustLevel,
		&u.Active,
		&u.Silenced,
		&u.InviteStatus,
		&u.CreatedAt,
		&u.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) ListUsers(ctx context.Context, limit, offset int) ([]models.UserWithInvite, int64, error) {
	var query string
	if s.dbType == "sqlite" {
		query = `
SELECT
    u.id, u.username, u.name, u.avatar_template, u.trust_level, u.active, u.silenced,
    u.invite_status, u.created_at, u.updated_at,
    i.code, i.used, i.used_email
FROM users u
LEFT JOIN (
    SELECT user_id, code, used, used_email,
           ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY created_at DESC) as rn
    FROM invite_codes
) i ON i.user_id = u.id AND i.rn = 1
ORDER BY u.id DESC
LIMIT ? OFFSET ?`
	} else {
		query = `
SELECT
    u.id, u.username, u.name, u.avatar_template, u.trust_level, u.active, u.silenced,
    u.invite_status, u.created_at, u.updated_at,
    i.code, i.used, i.used_email
FROM users u
LEFT JOIN LATERAL (
    SELECT code, used, used_email
    FROM invite_codes
    WHERE user_id = u.id
    ORDER BY created_at DESC
    LIMIT 1
) AS i ON true
ORDER BY u.id DESC
LIMIT $1 OFFSET $2`
	}

	rows, err := s.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []models.UserWithInvite
	for rows.Next() {
		var u models.UserWithInvite
		if err := rows.Scan(
			&u.ID, &u.Username, &u.DisplayName, &u.AvatarTemplate, &u.TrustLevel,
			&u.Active, &u.Silenced, &u.InviteStatus, &u.CreatedAt, &u.UpdatedAt,
			&u.InviteCode, &u.InviteUsed, &u.InviteEmail,
		); err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}

	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&total); err != nil {
		return nil, 0, err
	}
	return users, total, rows.Err()
}

func (s *Store) UpdateUser(ctx context.Context, id int64, inviteStatus *models.UserStatus) error {
	if inviteStatus == nil {
		return nil
	}
	var query string
	if s.dbType == "sqlite" {
		query = `UPDATE users SET invite_status = ?, updated_at = datetime('now') WHERE id = ?`
	} else {
		query = `UPDATE users SET invite_status = $1, updated_at = NOW() WHERE id = $2`
	}
	result, err := s.db.ExecContext(ctx, query, *inviteStatus, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}


func (s *Store) ListInviteCodes(ctx context.Context, limit, offset int) ([]models.InviteCode, int64, error) {
	var query string
	if s.dbType == "sqlite" {
		query = `SELECT id, code, used, used_email, used_at, user_id, created_at, team_account_id FROM invite_codes ORDER BY created_at DESC LIMIT ? OFFSET ?`
	} else {
		query = `SELECT id, code, used, used_email, used_at, user_id, created_at, team_account_id FROM invite_codes ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	}

	rows, err := s.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var codes []models.InviteCode
	for rows.Next() {
		var c models.InviteCode
		var userID, teamAccountID sql.NullInt64
		if err := rows.Scan(&c.ID, &c.Code, &c.Used, &c.UsedEmail, &c.UsedAt, &userID, &c.CreatedAt, &teamAccountID); err != nil {
			return nil, 0, err
		}
		c.UserID = nullableInt64(userID)
		c.TeamAccountID = nullableInt64(teamAccountID)
		codes = append(codes, c)
	}

	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM invite_codes`).Scan(&total); err != nil {
		return nil, 0, err
	}
	return codes, total, rows.Err()
}

func (s *Store) CreateInviteCodes(ctx context.Context, count int, teamAccountID *int64) ([]models.InviteCode, error) {
	if count <= 0 {
		count = 1
	}
	invites := make([]models.InviteCode, 0, count)

	var query string
	if s.dbType == "sqlite" {
		query = `INSERT INTO invite_codes (code, used, used_email, user_id, team_account_id) VALUES (?, 0, NULL, NULL, ?) RETURNING id, code, used, used_email, used_at, user_id, created_at, team_account_id`
	} else {
		query = `INSERT INTO invite_codes (code, used, used_email, user_id, team_account_id) VALUES ($1, false, NULL, NULL, $2) RETURNING id, code, used, used_email, used_at, user_id, created_at, team_account_id`
	}

	for i := 0; i < count; i++ {
		for {
			code, err := invitecode.Generate()
			if err != nil {
				return nil, err
			}
			row := s.db.QueryRowContext(ctx, query, code, teamAccountID)
			var invite models.InviteCode
			var userID, teamID sql.NullInt64
			if err := row.Scan(&invite.ID, &invite.Code, &invite.Used, &invite.UsedEmail, &invite.UsedAt, &userID, &invite.CreatedAt, &teamID); err != nil {
				if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate") {
					continue
				}
				return nil, err
			}
			invite.UserID = nullableInt64(userID)
			invite.TeamAccountID = nullableInt64(teamID)
			invites = append(invites, invite)
			break
		}
	}
	return invites, nil
}

func (s *Store) DeleteInviteCode(ctx context.Context, id int64) error {
	var query string
	if s.dbType == "sqlite" {
		query = `DELETE FROM invite_codes WHERE id = ?`
	} else {
		query = `DELETE FROM invite_codes WHERE id = $1`
	}
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}

func (s *Store) GetInviteCodeByCode(ctx context.Context, code string) (*models.InviteCode, error) {
	var query string
	if s.dbType == "sqlite" {
		query = `SELECT id, code, used, used_email, used_at, user_id, created_at, team_account_id FROM invite_codes WHERE code = ?`
	} else {
		query = `SELECT id, code, used, used_email, used_at, user_id, created_at, team_account_id FROM invite_codes WHERE code = $1`
	}

	row := s.db.QueryRowContext(ctx, query, strings.TrimSpace(code))
	var invite models.InviteCode
	var userID, teamID sql.NullInt64
	if err := row.Scan(&invite.ID, &invite.Code, &invite.Used, &invite.UsedEmail, &invite.UsedAt, &userID, &invite.CreatedAt, &teamID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInviteNotFound
		}
		return nil, err
	}
	invite.UserID = nullableInt64(userID)
	invite.TeamAccountID = nullableInt64(teamID)
	return &invite, nil
}

func (s *Store) UpdateInviteCode(ctx context.Context, id int64, used bool, email *string) error {
	if used && (email == nil || strings.TrimSpace(*email) == "") {
		return fmt.Errorf("email required when marking used")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var userID sql.NullInt64
	var selectQuery string
	if s.dbType == "sqlite" {
		selectQuery = `SELECT user_id FROM invite_codes WHERE id = ?`
	} else {
		selectQuery = `SELECT user_id FROM invite_codes WHERE id = $1`
	}
	if err := tx.QueryRowContext(ctx, selectQuery, id).Scan(&userID); err != nil {
		return err
	}

	if used {
		var updateQuery string
		if s.dbType == "sqlite" {
			updateQuery = `UPDATE invite_codes SET used = 1, used_email = ?, used_at = datetime('now') WHERE id = ?`
		} else {
			updateQuery = `UPDATE invite_codes SET used = true, used_email = $1, used_at = NOW() WHERE id = $2`
		}
		if _, err := tx.ExecContext(ctx, updateQuery, strings.TrimSpace(*email), id); err != nil {
			return err
		}
		if userID.Valid {
			var userUpdate string
			if s.dbType == "sqlite" {
				userUpdate = `UPDATE users SET invite_status = ?, updated_at = datetime('now') WHERE id = ?`
			} else {
				userUpdate = `UPDATE users SET invite_status = $1, updated_at = NOW() WHERE id = $2`
			}
			if _, err := tx.ExecContext(ctx, userUpdate, models.UserStatusCompleted, userID.Int64); err != nil {
				return err
			}
		}
	} else {
		var updateQuery string
		if s.dbType == "sqlite" {
			updateQuery = `UPDATE invite_codes SET used = 0, used_email = NULL, used_at = NULL WHERE id = ?`
		} else {
			updateQuery = `UPDATE invite_codes SET used = false, used_email = NULL, used_at = NULL WHERE id = $1`
		}
		if _, err := tx.ExecContext(ctx, updateQuery, id); err != nil {
			return err
		}
		if userID.Valid {
			var userUpdate string
			if s.dbType == "sqlite" {
				userUpdate = `UPDATE users SET invite_status = ?, updated_at = datetime('now') WHERE id = ?`
			} else {
				userUpdate = `UPDATE users SET invite_status = $1, updated_at = NOW() WHERE id = $2`
			}
			if _, err := tx.ExecContext(ctx, userUpdate, models.UserStatusPendingSubmission, userID.Int64); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}


func (s *Store) AssignInviteCode(ctx context.Context, codeID, userID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var used bool
	var existingUser sql.NullInt64
	var selectQuery string
	if s.dbType == "sqlite" {
		selectQuery = `SELECT used, user_id FROM invite_codes WHERE id = ?`
	} else {
		selectQuery = `SELECT used, user_id FROM invite_codes WHERE id = $1`
	}
	if err := tx.QueryRowContext(ctx, selectQuery, codeID).Scan(&used, &existingUser); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInviteNotFound
		}
		return err
	}
	if used {
		return ErrAlreadyClaimed
	}
	if existingUser.Valid {
		return ErrInviteAssigned
	}

	var status models.UserStatus
	var userQuery string
	if s.dbType == "sqlite" {
		userQuery = `SELECT invite_status FROM users WHERE id = ?`
	} else {
		userQuery = `SELECT invite_status FROM users WHERE id = $1`
	}
	if err := tx.QueryRowContext(ctx, userQuery, userID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("user not found")
		}
		return err
	}
	if status != models.UserStatusNone {
		return fmt.Errorf("user already has invite status %d", status)
	}

	var updateCode, updateUser string
	if s.dbType == "sqlite" {
		updateCode = `UPDATE invite_codes SET user_id = ? WHERE id = ?`
		updateUser = `UPDATE users SET invite_status = ?, updated_at = datetime('now') WHERE id = ?`
	} else {
		updateCode = `UPDATE invite_codes SET user_id = $1 WHERE id = $2`
		updateUser = `UPDATE users SET invite_status = $1, updated_at = NOW() WHERE id = $2`
	}
	if _, err := tx.ExecContext(ctx, updateCode, userID, codeID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, updateUser, models.UserStatusPendingSubmission, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) LatestInviteForUser(ctx context.Context, userID int64) (*models.InviteCode, error) {
	var query string
	if s.dbType == "sqlite" {
		query = `SELECT id, code, used, used_email, used_at, user_id, created_at, team_account_id FROM invite_codes WHERE user_id = ? ORDER BY created_at DESC LIMIT 1`
	} else {
		query = `SELECT id, code, used, used_email, used_at, user_id, created_at, team_account_id FROM invite_codes WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1`
	}

	row := s.db.QueryRowContext(ctx, query, userID)
	var invite models.InviteCode
	var storedUserID, teamID sql.NullInt64
	if err := row.Scan(&invite.ID, &invite.Code, &invite.Used, &invite.UsedEmail, &invite.UsedAt, &storedUserID, &invite.CreatedAt, &teamID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	invite.UserID = nullableInt64(storedUserID)
	invite.TeamAccountID = nullableInt64(teamID)
	return &invite, nil
}

func (s *Store) CompleteInviteSubmission(ctx context.Context, inviteID int64, email string, send func(context.Context) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var used bool
	var userID sql.NullInt64
	var selectQuery string
	if s.dbType == "sqlite" {
		selectQuery = `SELECT used, user_id FROM invite_codes WHERE id = ?`
	} else {
		selectQuery = `SELECT used, user_id FROM invite_codes WHERE id = $1`
	}
	if err := tx.QueryRowContext(ctx, selectQuery, inviteID).Scan(&used, &userID); err != nil {
		return err
	}
	if used {
		return ErrAlreadyClaimed
	}

	if send != nil {
		if err := send(ctx); err != nil {
			return err
		}
	}

	var updateCode, updateUser string
	if s.dbType == "sqlite" {
		updateCode = `UPDATE invite_codes SET used = 1, used_email = ?, used_at = datetime('now') WHERE id = ?`
		updateUser = `UPDATE users SET invite_status = ?, updated_at = datetime('now') WHERE id = ?`
	} else {
		updateCode = `UPDATE invite_codes SET used = true, used_email = $1, used_at = NOW() WHERE id = $2`
		updateUser = `UPDATE users SET invite_status = $1, updated_at = NOW() WHERE id = $2`
	}
	if _, err := tx.ExecContext(ctx, updateCode, email, inviteID); err != nil {
		return err
	}
	if userID.Valid {
		if _, err := tx.ExecContext(ctx, updateUser, models.UserStatusCompleted, userID.Int64); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) CompleteInviteSubmissionByCode(ctx context.Context, code string, userID int64, email string, send func(context.Context) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var used bool
	var existingUser sql.NullInt64
	var codeID int64
	var selectQuery string
	if s.dbType == "sqlite" {
		selectQuery = `SELECT id, used, user_id FROM invite_codes WHERE code = ?`
	} else {
		selectQuery = `SELECT id, used, user_id FROM invite_codes WHERE code = $1`
	}
	if err := tx.QueryRowContext(ctx, selectQuery, strings.TrimSpace(code)).Scan(&codeID, &used, &existingUser); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInviteNotFound
		}
		return err
	}
	if used {
		return ErrAlreadyClaimed
	}
	if existingUser.Valid && existingUser.Int64 != userID {
		return ErrInviteAssigned
	}

	if send != nil {
		if err := send(ctx); err != nil {
			return err
		}
	}

	var updateCode, updateUser string
	if s.dbType == "sqlite" {
		updateCode = `UPDATE invite_codes SET used = 1, used_email = ?, used_at = datetime('now'), user_id = COALESCE(user_id, ?) WHERE id = ?`
		updateUser = `UPDATE users SET invite_status = ?, updated_at = datetime('now') WHERE id = ?`
	} else {
		updateCode = `UPDATE invite_codes SET used = true, used_email = $1, used_at = NOW(), user_id = COALESCE(user_id, $2) WHERE id = $3`
		updateUser = `UPDATE users SET invite_status = $1, updated_at = NOW() WHERE id = $2`
	}
	if _, err := tx.ExecContext(ctx, updateCode, email, userID, codeID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, updateUser, models.UserStatusCompleted, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func nullableInt64(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	val := n.Int64
	return &val
}
