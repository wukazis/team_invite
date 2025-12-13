package invite

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync/atomic"

	"team-invite/internal/config"
)

type Account struct {
	ID    string
	Token string
}

type Config struct {
	Accounts   []config.InviteAccount
	Strategy   string // primary | round_robin | failover
	ActiveID   string // optional, prefer this account first
	ScriptPath string // python script path
}

type Service struct {
	accounts   []Account
	strategy   string
	scriptPath string
	rr         uint64
}

type SendError struct {
	StatusCode int
	Body       string
}

func (e *SendError) Error() string {
	return fmt.Sprintf("invite send failed with status %d", e.StatusCode)
}

func New(cfg Config) *Service {
	accounts := make([]Account, 0, len(cfg.Accounts))
	for _, a := range cfg.Accounts {
		id := strings.TrimSpace(a.AccountID)
		token := strings.TrimSpace(a.AuthorizationToken)
		if id == "" || token == "" {
			continue
		}
		accounts = append(accounts, Account{ID: id, Token: token})
	}
	accounts = prioritizeAccounts(accounts, cfg.ActiveID)

	strategy := strings.ToLower(strings.TrimSpace(cfg.Strategy))
	if strategy == "" {
		strategy = "primary"
	}
	scriptPath := strings.TrimSpace(cfg.ScriptPath)
	if scriptPath == "" {
		scriptPath = "/home/ubuntu/team-invite/send_invite.py"
	}

	return &Service{
		accounts:   accounts,
		strategy:   strategy,
		scriptPath: scriptPath,
	}
}

// Backwards compatibility for older call sites.
func NewWithProxy(accountID, authToken, proxyURL, proxySecret string) *Service {
	_ = proxyURL
	_ = proxySecret
	return New(Config{
		Accounts:   []config.InviteAccount{{AccountID: accountID, AuthorizationToken: authToken}},
		Strategy:   "primary",
		ScriptPath: "/home/ubuntu/team-invite/send_invite.py",
	})
}

func (s *Service) Send(ctx context.Context, email string) error {
	if s == nil {
		return &SendError{StatusCode: 0, Body: "invite service not configured"}
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return fmt.Errorf("email is required")
	}
	if len(s.accounts) == 0 {
		return &SendError{StatusCode: 0, Body: "invite service not configured"}
	}

	plan := s.pickAccounts()
	var lastErr error
	for _, account := range plan {
		err := s.sendWithAccount(ctx, account, email)
		if err == nil {
			return nil
		}
		lastErr = err

		sendErr := new(SendError)
		if !asSendError(err, sendErr) {
			return err
		}
		if !isRetryableAuthError(sendErr) {
			return err
		}
	}
	if lastErr == nil {
		lastErr = &SendError{StatusCode: 0, Body: "invite service not configured"}
	}
	return lastErr
}

func (s *Service) pickAccounts() []Account {
	if len(s.accounts) <= 1 {
		return s.accounts
	}
	switch s.strategy {
	case "round_robin":
		start := int(atomic.AddUint64(&s.rr, 1)-1) % len(s.accounts)
		out := make([]Account, 0, len(s.accounts))
		out = append(out, s.accounts[start:]...)
		out = append(out, s.accounts[:start]...)
		return out
	case "failover":
		return s.accounts
	case "primary":
		fallthrough
	default:
		return s.accounts[:1]
	}
}

func (s *Service) sendWithAccount(ctx context.Context, account Account, email string) error {
	if account.ID == "" || account.Token == "" {
		return &SendError{StatusCode: 0, Body: "invite service not configured"}
	}

	cmd := exec.CommandContext(ctx, "python3", s.scriptPath, account.ID, account.Token, email)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return &SendError{StatusCode: 0, Body: fmt.Sprintf("script error: %v, output: %s", err, string(output))}
	}

	var result struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return &SendError{StatusCode: 0, Body: fmt.Sprintf("parse error: %v, output: %s", err, string(output))}
	}
	if result.Error != "" {
		return &SendError{StatusCode: 0, Body: result.Error}
	}
	if result.Status < 200 || result.Status >= 300 {
		return &SendError{StatusCode: result.Status, Body: result.Body}
	}
	return nil
}

func prioritizeAccounts(items []Account, activeID string) []Account {
	activeID = strings.TrimSpace(activeID)
	if activeID == "" || len(items) <= 1 {
		return items
	}
	for i, item := range items {
		if item.ID == activeID {
			if i == 0 {
				return items
			}
			out := make([]Account, 0, len(items))
			out = append(out, items[i])
			out = append(out, items[:i]...)
			out = append(out, items[i+1:]...)
			return out
		}
	}
	return items
}

func asSendError(err error, out *SendError) bool {
	se, ok := err.(*SendError)
	if !ok {
		return false
	}
	*out = *se
	return true
}

func isRetryableAuthError(err *SendError) bool {
	if err == nil {
		return false
	}
	if err.StatusCode == 401 || err.StatusCode == 403 {
		return true
	}
	body := strings.ToLower(err.Body)
	if strings.Contains(body, "cf_chl") || strings.Contains(body, "challenge-platform") || strings.Contains(body, "__cf_chl") {
		return true
	}
	if strings.Contains(body, "unauthorized") && strings.Contains(body, "access token") {
		return true
	}
	return false
}
