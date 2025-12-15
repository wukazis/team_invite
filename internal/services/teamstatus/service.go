package teamstatus

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"team-invite/internal/database"
)

const scriptPath = "/app/team_status.py"

type Service struct {
	store    *database.Store
	logger   Logger
	interval time.Duration

	mu          sync.RWMutex
	adminCache  []database.TeamAccountStatus
	publicCache []database.TeamAccountStatus
}

type Logger interface {
	Warn(msg string, args ...any)
}

type scriptPayload struct {
	SeatsInUse     int    `json:"seats_in_use"`
	SeatsEntitled  int    `json:"seats_entitled"`
	PendingInvites int    `json:"pending_invites"`
	PlanType       string `json:"plan_type"`
	ActiveUntil    string `json:"active_until"`
	Error          string `json:"error"`
	StatusCode     int    `json:"status_code"`
	Body           string `json:"body"`
}

func New(store *database.Store, logger Logger, interval time.Duration) *Service {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Service{
		store:    store,
		logger:   logger,
		interval: interval,
	}
}

func (s *Service) Start(ctx context.Context) {
	go s.run(ctx)
}

// RefreshAsync triggers a refresh in background (non-blocking).
func (s *Service) RefreshAsync(ctx context.Context) {
	go s.refresh(ctx)
}

// RefreshNow triggers a refresh synchronously.
func (s *Service) RefreshNow(ctx context.Context) {
	s.refresh(ctx)
}

func (s *Service) run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	s.refresh(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refresh(ctx)
		}
	}
}

func (s *Service) refresh(ctx context.Context) {
	accounts, err := s.store.ListTeamAccounts(ctx)
	if err != nil {
		s.logger.Warn("team status refresh failed: list accounts", "error", err)
		return
	}

	var adminStatuses []database.TeamAccountStatus
	for _, acc := range accounts {
		status := database.TeamAccountStatus{TeamAccount: acc}
		payload, fetchErr := fetchStatus(ctx, acc.AccountID, acc.AuthToken)
		if fetchErr != nil {
			s.logger.Warn("team status fetch failed", "account", acc.ID, "error", fetchErr)
		} else {
			status.SeatsInUse = payload.SeatsInUse
			status.SeatsEntitled = payload.SeatsEntitled
			status.PendingInvites = payload.PendingInvites
			status.PlanType = payload.PlanType
			status.ActiveUntil = payload.ActiveUntil
		}
		adminStatuses = append(adminStatuses, status)
	}

	publicStatuses := make([]database.TeamAccountStatus, 0, len(adminStatuses))
	for _, st := range adminStatuses {
		if !st.Enabled {
			continue
		}
		st.AccountID = ""
		st.AuthToken = ""
		publicStatuses = append(publicStatuses, st)
	}

	s.mu.Lock()
	s.adminCache = adminStatuses
	s.publicCache = publicStatuses
	s.mu.Unlock()
}

func (s *Service) PublicStatuses() []database.TeamAccountStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]database.TeamAccountStatus, len(s.publicCache))
	copy(out, s.publicCache)
	return out
}

func (s *Service) AdminStatuses() []database.TeamAccountStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]database.TeamAccountStatus, len(s.adminCache))
	copy(out, s.adminCache)
	return out
}

func fetchStatus(ctx context.Context, accountID, authToken string) (*scriptPayload, error) {
	accountID = strings.TrimSpace(accountID)
	authToken = strings.TrimSpace(authToken)
	if accountID == "" || authToken == "" {
		return nil, fmt.Errorf("account or token empty")
	}

	cmd := exec.CommandContext(ctx, "python3", scriptPath, accountID, authToken)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("script error: %v, output: %s", err, string(output))
	}

	var payload scriptPayload
	if err := json.Unmarshal(output, &payload); err != nil {
		return nil, fmt.Errorf("parse error: %v, output: %s", err, string(output))
	}
	if payload.Error != "" {
		return nil, fmt.Errorf("script returned error: %s", payload.Error)
	}

	return &payload, nil
}
