package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"team-invite/internal/models"
	"team-invite/internal/oauth"
	"team-invite/internal/util/invitecode"
)

var (
	ErrQuotaEmpty     = errors.New("quota exhausted")
	ErrNoChances      = errors.New("no chances left")
	ErrAlreadyClaimed = errors.New("invite already claimed")
	ErrInviteAssigned = errors.New("invite already assigned")
	ErrInviteNotFound = errors.New("invite not found")
)

type Store struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *Store) UpsertUserFromOAuth(ctx context.Context, profile oauth.Profile) (*models.User, error) {
	query := `
INSERT INTO users (username, name, avatar_template, trust_level, active, silenced, chances, invite_status, linuxdo_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (linuxdo_id) DO UPDATE
SET username = EXCLUDED.username,
    name = EXCLUDED.name,
    avatar_template = EXCLUDED.avatar_template,
    trust_level = EXCLUDED.trust_level,
    active = EXCLUDED.active,
    silenced = EXCLUDED.silenced,
    updated_at = NOW()
RETURNING id, username, name, avatar_template, trust_level, active, silenced, chances, invite_status, created_at, updated_at;
`
	row := s.pool.QueryRow(ctx, query,
		profile.Username,
		profile.Name,
		profile.AvatarTemplate,
		profile.TrustLevel,
		profile.Active,
		profile.Silenced,
		1,
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
		&u.Chances,
		&u.InviteStatus,
		&u.CreatedAt,
		&u.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	row := s.pool.QueryRow(ctx, `
SELECT id, username, name, avatar_template, trust_level, active, silenced, chances, invite_status, created_at, updated_at
FROM users WHERE id = $1`, id)
	var u models.User
	if err := row.Scan(
		&u.ID,
		&u.Username,
		&u.DisplayName,
		&u.AvatarTemplate,
		&u.TrustLevel,
		&u.Active,
		&u.Silenced,
		&u.Chances,
		&u.InviteStatus,
		&u.CreatedAt,
		&u.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) ConsumeChance(ctx context.Context, userID int64) error {
	tag, err := s.pool.Exec(ctx, `
UPDATE users SET chances = chances - 1, updated_at = NOW()
WHERE id = $1 AND chances > 0`, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNoChances
	}
	return nil
}

func (s *Store) AddChance(ctx context.Context, userID int64, delta int) error {
	_, err := s.pool.Exec(ctx, `
UPDATE users SET chances = chances + $2, updated_at = NOW()
WHERE id = $1`, userID, delta)
	return err
}

func (s *Store) ListUsers(ctx context.Context, limit, offset int) ([]models.UserWithInvite, int64, error) {
	rows, err := s.pool.Query(ctx, `
SELECT
    u.id,
    u.username,
    u.name,
    u.avatar_template,
    u.trust_level,
    u.active,
    u.silenced,
    u.chances,
    u.invite_status,
    u.created_at,
    u.updated_at,
    i.code,
    i.used,
    i.used_email
FROM users u
LEFT JOIN LATERAL (
    SELECT code, used, used_email
    FROM invite_codes
    WHERE user_id = u.id
    ORDER BY created_at DESC
    LIMIT 1
) AS i ON true
ORDER BY u.id DESC
LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var users []models.UserWithInvite
	for rows.Next() {
		var u models.UserWithInvite
		if err := rows.Scan(
			&u.ID,
			&u.Username,
			&u.DisplayName,
			&u.AvatarTemplate,
			&u.TrustLevel,
			&u.Active,
			&u.Silenced,
			&u.Chances,
			&u.InviteStatus,
			&u.CreatedAt,
			&u.UpdatedAt,
			&u.InviteCode,
			&u.InviteUsed,
			&u.InviteEmail,
		); err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	var total int64
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&total); err != nil {
		return nil, 0, err
	}
	return users, total, rows.Err()
}

func (s *Store) UpdateUser(ctx context.Context, id int64, chances *int, inviteStatus *models.UserStatus) error {
	setParts := []string{}
	args := []interface{}{}
	argIdx := 1
	if chances != nil {
		setParts = append(setParts, fmt.Sprintf("chances = $%d", argIdx))
		args = append(args, *chances)
		argIdx++
	}
	if inviteStatus != nil {
		setParts = append(setParts, fmt.Sprintf("invite_status = $%d", argIdx))
		args = append(args, *inviteStatus)
		argIdx++
	}
	if len(setParts) == 0 {
		return nil
	}
	setParts = append(setParts, fmt.Sprintf("updated_at = NOW()"))
	query := fmt.Sprintf("UPDATE users SET %s WHERE id = $%d", strings.Join(setParts, ", "), argIdx)
	args = append(args, id)
	cmd, err := s.pool.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

func (s *Store) ListSpinRecords(ctx context.Context, limit, offset int) ([]models.SpinRecord, int64, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, user_id, username, prize, status, detail, spin_id, created_at
FROM spin_records
ORDER BY created_at DESC
LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []models.SpinRecord
	for rows.Next() {
		var r models.SpinRecord
		if err := rows.Scan(&r.ID, &r.UserID, &r.Username, &r.Prize, &r.Status, &r.Detail, &r.SpinID, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, r)
	}
	var total int64
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM spin_records`).Scan(&total); err != nil {
		return nil, 0, err
	}
	return list, total, rows.Err()
}

func (s *Store) ListInviteCodes(ctx context.Context, limit, offset int) ([]models.InviteCode, int64, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id, code, used, used_email, used_at, user_id, created_at, team_account_id
FROM invite_codes
ORDER BY created_at DESC
LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var codes []models.InviteCode
	for rows.Next() {
		var c models.InviteCode
		var userID sql.NullInt64
		var teamAccountID sql.NullInt64
		if err := rows.Scan(&c.ID, &c.Code, &c.Used, &c.UsedEmail, &c.UsedAt, &userID, &c.CreatedAt, &teamAccountID); err != nil {
			return nil, 0, err
		}
		c.UserID = nullableInt64(userID)
		if teamAccountID.Valid {
			c.TeamAccountID = &teamAccountID.Int64
		}
		codes = append(codes, c)
	}
	var total int64
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM invite_codes`).Scan(&total); err != nil {
		return nil, 0, err
	}
	return codes, total, rows.Err()
}

func (s *Store) CreateInviteCodes(ctx context.Context, count int, teamAccountID *int64) ([]models.InviteCode, error) {
	if count <= 0 {
		count = 1
	}
	invites := make([]models.InviteCode, 0, count)
	for i := 0; i < count; i++ {
		for {
			code, err := invitecode.Generate()
			if err != nil {
				return nil, err
			}
			row := s.pool.QueryRow(ctx, `
INSERT INTO invite_codes (code, used, used_email, user_id, team_account_id)
VALUES ($1, false, NULL, NULL, $2)
RETURNING id, code, used, used_email, used_at, user_id, created_at, team_account_id`, code, teamAccountID)
			var invite models.InviteCode
			var userID sql.NullInt64
			var teamID sql.NullInt64
			if err := row.Scan(&invite.ID, &invite.Code, &invite.Used, &invite.UsedEmail, &invite.UsedAt, &userID, &invite.CreatedAt, &teamID); err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == "23505" {
					continue
				}
				return nil, err
			}
			invite.UserID = nullableInt64(userID)
			if teamID.Valid {
				invite.TeamAccountID = &teamID.Int64
			}
			invites = append(invites, invite)
			break
		}
	}
	return invites, nil
}

func (s *Store) DeleteInviteCode(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM invite_codes WHERE id = $1`, id)
	return err
}

func (s *Store) GetInviteCodeByCode(ctx context.Context, code string) (*models.InviteCode, error) {
	row := s.pool.QueryRow(ctx, `
SELECT id, code, used, used_email, used_at, user_id, created_at, team_account_id
FROM invite_codes
WHERE code = $1`, strings.TrimSpace(code))
	var invite models.InviteCode
	var userID sql.NullInt64
	var teamID sql.NullInt64
	if err := row.Scan(&invite.ID, &invite.Code, &invite.Used, &invite.UsedEmail, &invite.UsedAt, &userID, &invite.CreatedAt, &teamID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInviteNotFound
		}
		return nil, err
	}
	invite.UserID = nullableInt64(userID)
	if teamID.Valid {
		invite.TeamAccountID = &teamID.Int64
	}
	return &invite, nil
}

func (s *Store) UpdateInviteCode(ctx context.Context, id int64, used bool, email *string) error {
	if used && (email == nil || strings.TrimSpace(*email) == "") {
		return fmt.Errorf("email required when marking used")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var userID *int64
	if err := tx.QueryRow(ctx, `SELECT user_id FROM invite_codes WHERE id = $1 FOR UPDATE`, id).Scan(&userID); err != nil {
		return err
	}
	if used {
		_, err = tx.Exec(ctx, `
UPDATE invite_codes SET used = true, used_email = $2, used_at = NOW() WHERE id = $1`, id, strings.TrimSpace(*email))
		if err != nil {
			return err
		}
		if userID != nil {
			if _, err := tx.Exec(ctx, `UPDATE users SET invite_status = $2, updated_at = NOW() WHERE id = $1`, *userID, models.UserStatusCompleted); err != nil {
				return err
			}
		}
	} else {
		_, err = tx.Exec(ctx, `
UPDATE invite_codes SET used = false, used_email = NULL, used_at = NULL WHERE id = $1`, id)
		if err != nil {
			return err
		}
		if userID != nil {
			if _, err := tx.Exec(ctx, `UPDATE users SET invite_status = $2, updated_at = NOW() WHERE id = $1`, *userID, models.UserStatusPendingSubmission); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) AssignInviteCode(ctx context.Context, codeID, userID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var used bool
	var existingUser sql.NullInt64
	if err := tx.QueryRow(ctx, `SELECT used, user_id FROM invite_codes WHERE id = $1 FOR UPDATE`, codeID).Scan(&used, &existingUser); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
	if err := tx.QueryRow(ctx, `SELECT invite_status FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("user not found")
		}
		return err
	}
	if status != models.UserStatusNone {
		return fmt.Errorf("user already has invite status %d", status)
	}

	if _, err := tx.Exec(ctx, `UPDATE invite_codes SET user_id = $1 WHERE id = $2`, userID, codeID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET invite_status = $2, updated_at = NOW() WHERE id = $1`, userID, models.UserStatusPendingSubmission); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ResetChancesForUnwon(ctx context.Context) (int64, error) {
	cmd, err := s.pool.Exec(ctx, `UPDATE users SET chances = 1, updated_at = NOW() WHERE invite_status = $1`, models.UserStatusNone)
	if err != nil {
		return 0, err
	}
	return cmd.RowsAffected(), nil
}

func (s *Store) RecordSpin(ctx context.Context, rec models.SpinRecord) error {
	query := `
INSERT INTO spin_records (user_id, username, prize, status, detail, spin_id)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (spin_id) DO UPDATE SET status = EXCLUDED.status, detail = EXCLUDED.detail`

	_, err := s.pool.Exec(ctx, query, rec.UserID, rec.Username, rec.Prize, rec.Status, rec.Detail, rec.SpinID)
	if err == nil {
		return nil
	}

	// Auto-heal: if the table sequence got out of sync (common after manual imports/restores),
	// inserts may fail with duplicate key on the primary key despite using BIGSERIAL.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "spin_records_pkey" {
		if repairErr := s.repairSpinRecordIDSequence(ctx); repairErr != nil {
			return err
		}
		_, retryErr := s.pool.Exec(ctx, query, rec.UserID, rec.Username, rec.Prize, rec.Status, rec.Detail, rec.SpinID)
		if retryErr == nil {
			return nil
		}
	}
	return err
}

func (s *Store) repairSpinRecordIDSequence(ctx context.Context) error {
	var seqName *string
	if err := s.pool.QueryRow(ctx, `SELECT pg_get_serial_sequence('spin_records','id')`).Scan(&seqName); err != nil {
		return err
	}
	if seqName == nil || *seqName == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `SELECT setval($1::regclass, (SELECT COALESCE(MAX(id),0) FROM spin_records)+1, false)`, *seqName)
	return err
}

func (s *Store) GetQuota(ctx context.Context) (int, error) {
	row := s.pool.QueryRow(ctx, `SELECT value FROM config WHERE key = 'quota'`)
	var val string
	if err := row.Scan(&val); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	quota, err := strconv.Atoi(val)
	if err != nil {
		return 0, err
	}
	return quota, nil
}

func (s *Store) UpdateQuota(ctx context.Context, quota int) error {
	_, err := s.pool.Exec(ctx, `
INSERT INTO config (key, value) VALUES ('quota', $1)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, strconv.Itoa(quota))
	return err
}

type PrizeConfigCacheValue struct {
	Items []models.PrizeConfigItem
}

func (s *Store) LoadPrizeConfig(ctx context.Context) ([]models.PrizeConfigItem, error) {
	row := s.pool.QueryRow(ctx, `SELECT value FROM config WHERE key = 'prize_config'`)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return defaultPrizeConfig(), nil
		}
		return nil, err
	}
	if raw == "" {
		return defaultPrizeConfig(), nil
	}
	var items []models.PrizeConfigItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return defaultPrizeConfig(), nil
	}
	return items, nil
}

func (s *Store) UpdatePrizeConfig(ctx context.Context, items []models.PrizeConfigItem) error {
	payload, err := json.Marshal(items)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO config (key, value) VALUES ('prize_config', $1)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, string(payload))
	return err
}

func defaultPrizeConfig() []models.PrizeConfigItem {
	return []models.PrizeConfigItem{
		{Type: "win", Name: "🎉 Team", Probability: 0.075},
		{Type: "retry", Name: "🔄 再来一次", Probability: 1.0 / 6.0},
		{Type: "lose", Name: "😢 谢谢参与", Probability: 1 - 0.075 - (1.0 / 6.0)},
	}
}

type QuotaSchedule struct {
	Target    int       `json:"target"`
	ApplyAt   time.Time `json:"applyAt"`
	Author    string    `json:"author"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Store) SaveQuotaSchedule(ctx context.Context, schedule QuotaSchedule) error {
	payload, err := json.Marshal(schedule)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO config (key, value) VALUES ('quota_schedule', $1)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, string(payload))
	return err
}

func (s *Store) LoadQuotaSchedule(ctx context.Context) (*QuotaSchedule, error) {
	row := s.pool.QueryRow(ctx, `SELECT value FROM config WHERE key = 'quota_schedule'`)
	var raw string
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	var schedule QuotaSchedule
	if err := json.Unmarshal([]byte(raw), &schedule); err != nil {
		return nil, nil
	}
	return &schedule, nil
}

func (s *Store) ClearQuotaSchedule(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM config WHERE key = 'quota_schedule'`)
	return err
}

func (s *Store) AwardWin(ctx context.Context, userID int64, code string) (*models.InviteCode, int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx)

	var chances int
	if err := tx.QueryRow(ctx, `SELECT chances FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&chances); err != nil {
		return nil, 0, err
	}
	if chances < 0 {
		return nil, 0, ErrNoChances
	}

	var quota int
	row := tx.QueryRow(ctx, `SELECT value FROM config WHERE key = 'quota' FOR UPDATE`)
	var val string
	if err := row.Scan(&val); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, 0, ErrQuotaEmpty
		}
		return nil, 0, err
	}
	if _, err := fmt.Sscanf(val, "%d", &quota); err != nil {
		return nil, 0, err
	}
	if quota <= 0 {
		return nil, quota, ErrQuotaEmpty
	}
	quota--
	if _, err := tx.Exec(ctx, `UPDATE config SET value = $2 WHERE key = $1`, "quota", strconv.Itoa(quota)); err != nil {
		return nil, 0, err
	}

	if _, err := tx.Exec(ctx, `UPDATE users SET invite_status = $2, updated_at = NOW() WHERE id = $1`, userID, models.UserStatusPendingSubmission); err != nil {
		return nil, 0, err
	}

row = tx.QueryRow(ctx, `
INSERT INTO invite_codes (code, used, used_email, user_id, team_account_id)
VALUES ($1, false, NULL, $2, NULL)
RETURNING id, code, used, used_email, used_at, user_id, created_at, team_account_id`, code, userID)
	var invite models.InviteCode
	var insertedUserID sql.NullInt64
	var teamID sql.NullInt64
	if err := row.Scan(&invite.ID, &invite.Code, &invite.Used, &invite.UsedEmail, &invite.UsedAt, &insertedUserID, &invite.CreatedAt, &teamID); err != nil {
		return nil, 0, err
	}
	invite.UserID = nullableInt64(insertedUserID)
	if teamID.Valid {
		invite.TeamAccountID = &teamID.Int64
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, 0, err
	}
	return &invite, quota, nil
}

func (s *Store) LatestInviteForUser(ctx context.Context, userID int64) (*models.InviteCode, error) {
row := s.pool.QueryRow(ctx, `
SELECT id, code, used, used_email, used_at, user_id, created_at, team_account_id
FROM invite_codes
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT 1`, userID)
	var invite models.InviteCode
	var storedUserID sql.NullInt64
	var teamID sql.NullInt64
	if err := row.Scan(&invite.ID, &invite.Code, &invite.Used, &invite.UsedEmail, &invite.UsedAt, &storedUserID, &invite.CreatedAt, &teamID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	invite.UserID = nullableInt64(storedUserID)
	if teamID.Valid {
		invite.TeamAccountID = &teamID.Int64
	}
	return &invite, nil
}

func (s *Store) CompleteInviteSubmission(ctx context.Context, inviteID int64, email string, send func(context.Context) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var used bool
	var userID *int64
	if err := tx.QueryRow(ctx, `SELECT used, user_id FROM invite_codes WHERE id = $1 FOR UPDATE`, inviteID).Scan(&used, &userID); err != nil {
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
	if _, err := tx.Exec(ctx, `
UPDATE invite_codes SET used = true, used_email = $2, used_at = NOW() WHERE id = $1`, inviteID, email); err != nil {
		return err
	}
	if userID != nil {
		if _, err := tx.Exec(ctx, `
UPDATE users SET invite_status = $2, updated_at = NOW() WHERE id = $1`,
			*userID, models.UserStatusCompleted); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// CompleteInviteSubmissionByCode allows using unassigned admin-generated invite codes.
// It only validates that the code exists and is unused. If the code was already assigned
// to another user (won code), it will be rejected.
func (s *Store) CompleteInviteSubmissionByCode(ctx context.Context, code string, userID int64, email string, send func(context.Context) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var used bool
	var existingUser sql.NullInt64
	var codeID int64
	if err := tx.QueryRow(ctx, `SELECT id, used, user_id FROM invite_codes WHERE code = $1 FOR UPDATE`, strings.TrimSpace(code)).Scan(&codeID, &used, &existingUser); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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

	// Mark code used and record who used it if it was unassigned.
	if _, err := tx.Exec(ctx, `
UPDATE invite_codes
SET used = true,
    used_email = $2,
    used_at = NOW(),
    user_id = COALESCE(user_id, $3)
WHERE id = $1`, codeID, email, userID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
UPDATE users SET invite_status = $2, updated_at = NOW() WHERE id = $1`,
		userID, models.UserStatusCompleted); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func nullableInt64(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	val := n.Int64
	return &val
}

func (s *Store) HourlyStats(ctx context.Context, since time.Time) ([]models.HourlySummary, models.SpinOverview, error) {
	rows, err := s.pool.Query(ctx, `
WITH summary AS (
    SELECT date_trunc('hour', created_at) AS hour_bucket,
           status,
           COUNT(*) AS cnt
    FROM spin_records
    WHERE created_at >= $1
    GROUP BY hour_bucket, status
)
SELECT hour_bucket, status, cnt
FROM summary
ORDER BY hour_bucket ASC`, since)
	if err != nil {
		return nil, models.SpinOverview{}, err
	}
	defer rows.Close()

	type key struct {
		H string
	}
	statMap := map[string]*models.HourlySummary{}
	overview := models.SpinOverview{}
	for rows.Next() {
		var ts time.Time
		var status string
		var cnt int32
		if err := rows.Scan(&ts, &status, &cnt); err != nil {
			return nil, overview, err
		}
		label := ts.Format("2006-01-02 15:00")
		item, ok := statMap[label]
		if !ok {
			item = &models.HourlySummary{HourLabel: label}
			statMap[label] = item
		}
		switch status {
		case "win":
			item.Win += cnt
			overview.Win += cnt
		case "retry":
			item.Retry += cnt
			overview.Retry += cnt
		case "lose":
			item.Lose += cnt
			overview.Lose += cnt
		default:
			overview.Unknown += cnt
		}
		overview.Total += cnt
	}

	stats := make([]models.HourlySummary, 0, len(statMap))
	for _, entry := range statMap {
		stats = append(stats, *entry)
	}

	// keep chronological order
	slices.SortFunc(stats, func(a, b models.HourlySummary) int {
		return strings.Compare(a.HourLabel, b.HourLabel)
	})
	return stats, overview, rows.Err()
}
func (s *Store) AssignInviteByCode(ctx context.Context, code string, userID int64) error {
	return fmt.Errorf("AssignInviteByCode is deprecated; use CompleteInviteSubmissionByCode")
}
