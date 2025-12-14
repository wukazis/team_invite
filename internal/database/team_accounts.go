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
	rows, err := s.pool.Query(ctx, `
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
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, account_id, auth_token, max_seats, enabled, created_at
		FROM team_accounts
		WHERE enabled = true
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

func (s *Store) GetTeamAccount(ctx context.Context, id int64) (*TeamAccount, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, name, account_id, auth_token, max_seats, enabled, created_at
		FROM team_accounts WHERE id = $1`, id)
	var a TeamAccount
	if err := row.Scan(&a.ID, &a.Name, &a.AccountID, &a.AuthToken, &a.MaxSeats, &a.Enabled, &a.CreatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) CreateTeamAccount(ctx context.Context, name, accountID, authToken string, maxSeats int) (*TeamAccount, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO team_accounts (name, account_id, auth_token, max_seats)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, account_id, auth_token, max_seats, enabled, created_at`,
		name, accountID, authToken, maxSeats)
	var a TeamAccount
	if err := row.Scan(&a.ID, &a.Name, &a.AccountID, &a.AuthToken, &a.MaxSeats, &a.Enabled, &a.CreatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) UpdateTeamAccount(ctx context.Context, id int64, name, accountID, authToken string, maxSeats int, enabled bool) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE team_accounts
		SET name = $2, account_id = $3, auth_token = $4, max_seats = $5, enabled = $6
		WHERE id = $1`,
		id, name, accountID, authToken, maxSeats, enabled)
	return err
}

func (s *Store) DeleteTeamAccount(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM team_accounts WHERE id = $1`, id)
	return err
}
