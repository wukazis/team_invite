package database

import (
	"context"
	"time"
)

type TeamAccount struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	AccountID string    `json:"accountId"`
	AuthToken string    `json:"authToken,omitempty"`
	MaxSeats  int       `json:"maxSeats"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
}

type TeamAccountStatus struct {
	TeamAccount
	SeatsInUse     int    `json:"seatsInUse"`
	SeatsEntitled  int    `json:"seatsEntitled"`
	PendingInvites int    `json:"pendingInvites"`
	PlanType       string `json:"planType"`
	ActiveUntil    string `json:"activeUntil"`
}

func (s *Store) ListTeamAccounts(ctx context.Context) ([]TeamAccount, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, account_id, auth_token, max_seats, enabled, created_at
		FROM team_accounts
		ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []TeamAccount
	for rows.Next() {
		var a TeamAccount
		if err := rows.Scan(&a.ID, &a.Name, &a.AccountID, &a.AuthToken, &a.MaxSeats, &a.Enabled, &a.CreatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (s *Store) ListEnabledTeamAccounts(ctx context.Context) ([]TeamAccount, error) {
	var query string
	if s.dbType == "sqlite" {
		query = `SELECT id, name, account_id, auth_token, max_seats, enabled, created_at FROM team_accounts WHERE enabled = 1 ORDER BY id ASC`
	} else {
		query = `SELECT id, name, account_id, auth_token, max_seats, enabled, created_at FROM team_accounts WHERE enabled = true ORDER BY id ASC`
	}

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []TeamAccount
	for rows.Next() {
		var a TeamAccount
		if err := rows.Scan(&a.ID, &a.Name, &a.AccountID, &a.AuthToken, &a.MaxSeats, &a.Enabled, &a.CreatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (s *Store) GetTeamAccount(ctx context.Context, id int64) (*TeamAccount, error) {
	var query string
	if s.dbType == "sqlite" {
		query = `SELECT id, name, account_id, auth_token, max_seats, enabled, created_at FROM team_accounts WHERE id = ?`
	} else {
		query = `SELECT id, name, account_id, auth_token, max_seats, enabled, created_at FROM team_accounts WHERE id = $1`
	}

	row := s.db.QueryRowContext(ctx, query, id)
	var a TeamAccount
	if err := row.Scan(&a.ID, &a.Name, &a.AccountID, &a.AuthToken, &a.MaxSeats, &a.Enabled, &a.CreatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) CreateTeamAccount(ctx context.Context, name, accountID, authToken string, maxSeats int) (*TeamAccount, error) {
	var query string
	if s.dbType == "sqlite" {
		query = `INSERT INTO team_accounts (name, account_id, auth_token, max_seats) VALUES (?, ?, ?, ?) RETURNING id, name, account_id, auth_token, max_seats, enabled, created_at`
	} else {
		query = `INSERT INTO team_accounts (name, account_id, auth_token, max_seats) VALUES ($1, $2, $3, $4) RETURNING id, name, account_id, auth_token, max_seats, enabled, created_at`
	}

	row := s.db.QueryRowContext(ctx, query, name, accountID, authToken, maxSeats)
	var a TeamAccount
	if err := row.Scan(&a.ID, &a.Name, &a.AccountID, &a.AuthToken, &a.MaxSeats, &a.Enabled, &a.CreatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) UpdateTeamAccount(ctx context.Context, id int64, name, accountID, authToken string, maxSeats int, enabled bool) error {
	var query string
	if s.dbType == "sqlite" {
		query = `UPDATE team_accounts SET name = ?, account_id = ?, auth_token = ?, max_seats = ?, enabled = ? WHERE id = ?`
		_, err := s.db.ExecContext(ctx, query, name, accountID, authToken, maxSeats, boolToInt(enabled), id)
		return err
	}
	query = `UPDATE team_accounts SET name = $1, account_id = $2, auth_token = $3, max_seats = $4, enabled = $5 WHERE id = $6`
	_, err := s.db.ExecContext(ctx, query, name, accountID, authToken, maxSeats, enabled, id)
	return err
}

func (s *Store) DeleteTeamAccount(ctx context.Context, id int64) error {
	var query string
	if s.dbType == "sqlite" {
		query = `DELETE FROM team_accounts WHERE id = ?`
	} else {
		query = `DELETE FROM team_accounts WHERE id = $1`
	}
	_, err := s.db.ExecContext(ctx, query, id)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
