package config

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type LinuxDoOAuth struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	AuthorizeURL string
	TokenURL     string
	UserInfoURL  string
}

type InviteAccount struct {
	AccountID          string `json:"accountId"`
	AuthorizationToken string `json:"authorizationToken"`
}

type Config struct {
	EnvFilePath       string
	HTTPPort          string
	PostgresURL       string
	JWTSecret         string
	JWTIssuer         string
	AdminPassword     string
	AdminAllowedIPs   []string
	AdminTOTPSecret   string
	PrizeConfigTTL    time.Duration
	QuotaScheduleTick time.Duration
	AppBaseURL        string
	InviteAccountID   string
	InviteAuthToken   string
	InviteProxyURL    string
	InviteProxySecret string
	InviteAccounts    []InviteAccount
	InviteStrategy    string
	InviteActiveID    string
	TurnstileSiteKey  string
	TurnstileSecret   string

	OAuth LinuxDoOAuth
}

func Load() (*Config, error) {
	envPath := resolveEnvPath()
	if envPath != "" {
		_ = godotenv.Load(envPath)
	} else {
		envPath = ".env"
		_ = godotenv.Load()
	}

	cfg := &Config{
		EnvFilePath:       getEnv("ENV_FILE_PATH", envPath),
		HTTPPort:          getEnv("HTTP_PORT", "8080"),
		PostgresURL:       os.Getenv("POSTGRES_URL"),
		JWTSecret:         os.Getenv("JWT_SECRET"),
		JWTIssuer:         getEnv("JWT_ISSUER", "team-invite"),
		AdminPassword:     os.Getenv("ADMIN_PASSWORD"),
		AdminAllowedIPs:   splitCSV(os.Getenv("ADMIN_ALLOWED_IPS")),
		AdminTOTPSecret:   os.Getenv("ADMIN_TOTP_SECRET"),
		PrizeConfigTTL:    getDuration("PRIZE_CACHE_TTL", 30*time.Second),
		QuotaScheduleTick: getDuration("QUOTA_SCHEDULER_TICK", 5*time.Second),
		AppBaseURL:        getEnv("APP_BASE_URL", "http://localhost:5173"),
		InviteAccountID:   os.Getenv("ACCOUNT_ID"),
		InviteAuthToken:   os.Getenv("AUTHORIZATION_TOKEN"),
		InviteProxyURL:    os.Getenv("CF_PROXY_URL"),
		InviteProxySecret: os.Getenv("CF_PROXY_SECRET"),
		InviteStrategy:    getEnv("INVITE_STRATEGY", "primary"),
		InviteActiveID:    os.Getenv("INVITE_ACTIVE_ACCOUNT_ID"),
		TurnstileSiteKey:  os.Getenv("CF_TURNSTILE_SITE_KEY"),
		TurnstileSecret:   os.Getenv("CF_TURNSTILE_SECRET_KEY"),
		OAuth: LinuxDoOAuth{
			ClientID:     os.Getenv("LINUXDO_CLIENT_ID"),
			ClientSecret: os.Getenv("LINUXDO_CLIENT_SECRET"),
			RedirectURI:  os.Getenv("LINUXDO_REDIRECT_URI"),
			AuthorizeURL: getEnv("LINUXDO_AUTHORIZE_URL", "https://connect.linux.do/oauth2/authorize"),
			TokenURL:     getEnv("LINUXDO_TOKEN_URL", "https://connect.linux.do/oauth2/token"),
			UserInfoURL:  getEnv("LINUXDO_USERINFO_URL", "https://connect.linux.do/api/user"),
		},
	}

	if cfg.PostgresURL == "" {
		return nil, errors.New("POSTGRES_URL is required")
	}
	if cfg.JWTSecret == "" {
		return nil, errors.New("JWT_SECRET is required")
	}
	if cfg.AdminPassword == "" {
		return nil, errors.New("ADMIN_PASSWORD is required")
	}
	if cfg.AdminTOTPSecret == "" {
		return nil, errors.New("ADMIN_TOTP_SECRET is required for admin login")
	}
	if cfg.OAuth.ClientID == "" || cfg.OAuth.ClientSecret == "" || cfg.OAuth.RedirectURI == "" {
		return nil, errors.New("Linux.do OAuth env vars are incomplete")
	}

	accounts, err := parseInviteAccounts(os.Getenv("INVITE_ACCOUNTS"))
	if err != nil {
		return nil, err
	}
	// Backwards compatible: allow single ACCOUNT_ID/AUTHORIZATION_TOKEN.
	if len(accounts) == 0 && cfg.InviteAccountID != "" && cfg.InviteAuthToken != "" {
		accounts = append(accounts, InviteAccount{
			AccountID:          cfg.InviteAccountID,
			AuthorizationToken: cfg.InviteAuthToken,
		})
	}
	cfg.InviteAccounts = prioritizeInviteAccounts(accounts, cfg.InviteActiveID)

	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	items := strings.Split(raw, ",")
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func getDuration(key string, def time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return d
	}
	if v, err := strconv.Atoi(raw); err == nil {
		return time.Duration(v) * time.Second
	}
	return def
}

func resolveEnvPath() string {
	if path := os.Getenv("ENV_FILE_PATH"); path != "" {
		return path
	}
	candidates := []string{".env", "local-only/.env"}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func parseInviteAccounts(raw string) ([]InviteAccount, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	// JSON array form:
	//   [{"accountId":"...","authorizationToken":"..."}, ...]
	if strings.HasPrefix(raw, "[") {
		var items []InviteAccount
		if err := json.Unmarshal([]byte(raw), &items); err != nil {
			return nil, err
		}
		return sanitizeInviteAccounts(items), nil
	}

	// Simple form: "accountId|token;accountId2|token2"
	parts := strings.Split(raw, ";")
	out := make([]InviteAccount, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "|", 2)
		if len(kv) != 2 {
			return nil, errors.New("INVITE_ACCOUNTS must be JSON or 'accountId|token;accountId|token'")
		}
		out = append(out, InviteAccount{
			AccountID:          strings.TrimSpace(kv[0]),
			AuthorizationToken: strings.TrimSpace(kv[1]),
		})
	}
	return sanitizeInviteAccounts(out), nil
}

func sanitizeInviteAccounts(items []InviteAccount) []InviteAccount {
	out := make([]InviteAccount, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		id := strings.TrimSpace(item.AccountID)
		token := strings.TrimSpace(item.AuthorizationToken)
		if id == "" || token == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, InviteAccount{AccountID: id, AuthorizationToken: token})
	}
	return out
}

func prioritizeInviteAccounts(items []InviteAccount, activeID string) []InviteAccount {
	activeID = strings.TrimSpace(activeID)
	if activeID == "" || len(items) <= 1 {
		return items
	}
	for i, item := range items {
		if item.AccountID == activeID {
			if i == 0 {
				return items
			}
			out := make([]InviteAccount, 0, len(items))
			out = append(out, items[i])
			out = append(out, items[:i]...)
			out = append(out, items[i+1:]...)
			return out
		}
	}
	return items
}
