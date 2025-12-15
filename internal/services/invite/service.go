package invite

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"team-invite/internal/config"
)

type Account struct {
	ID    string
	Token string
}

type Service struct {
	scriptPath string
}

type SendError struct {
	StatusCode int
	Body       string
}

func (e *SendError) Error() string {
	return fmt.Sprintf("invite send failed with status %d", e.StatusCode)
}

func New(scriptPath string) *Service {
	scriptPath = strings.TrimSpace(scriptPath)
	if scriptPath == "" {
		scriptPath = "/app/send_invite.py"
	}
	return &Service{scriptPath: scriptPath}
}

// Backwards compatibility for older call sites.
func NewWithProxy(accountID, authToken, proxyURL, proxySecret string) *Service {
	_ = accountID
	_ = authToken
	_ = proxyURL
	_ = proxySecret
	return New("/app/send_invite.py")
}

// SendWithAccount sends invite using the provided account credentials from DB.
func (s *Service) SendWithAccount(ctx context.Context, account config.InviteAccount, email string) error {
	if s == nil {
		return &SendError{StatusCode: 0, Body: "invite service not configured"}
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return fmt.Errorf("email is required")
	}
	id := strings.TrimSpace(account.AccountID)
	token := strings.TrimSpace(account.AuthorizationToken)
	if id == "" || token == "" {
		return &SendError{StatusCode: 0, Body: "invite service not configured"}
	}

	return s.sendWithAccount(ctx, Account{ID: id, Token: token}, email)
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
