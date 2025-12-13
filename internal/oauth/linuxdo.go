package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"team-invite/internal/config"
)

type Client struct {
	cfg        config.LinuxDoOAuth
	httpClient *http.Client
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

type Profile struct {
	ID             string
	Username       string
	Name           string
	AvatarTemplate string
	TrustLevel     int
	Active         bool
	Silenced       bool
}

type rawProfile struct {
	ID             interface{} `json:"id"`
	Username       string      `json:"username"`
	Name           string      `json:"name"`
	AvatarTemplate string      `json:"avatar_template"`
	TrustLevel     int         `json:"trust_level"`
	Active         bool        `json:"active"`
	Silenced       bool        `json:"silenced"`
}

func NewClient(cfg config.LinuxDoOAuth) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) AuthURL(state string) string {
	values := url.Values{
		"client_id":     {c.cfg.ClientID},
		"redirect_uri":  {c.cfg.RedirectURI},
		"response_type": {"code"},
		"scope":         {"read"},
		"state":         {state},
	}
	return fmt.Sprintf("%s?%s", c.cfg.AuthorizeURL, values.Encode())
}

func (c *Client) Exchange(ctx context.Context, code string) (*TokenResponse, error) {
	form := url.Values{
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
		"redirect_uri":  {c.cfg.RedirectURI},
		"grant_type":    {"authorization_code"},
		"code":          {code},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("token exchange failed: %s", resp.Status)
	}
	var token TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, err
	}
	return &token, nil
}

func (c *Client) FetchProfile(ctx context.Context, token string) (Profile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.UserInfoURL, nil)
	if err != nil {
		return Profile{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Profile{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return Profile{}, fmt.Errorf("profile fetch failed: %s", resp.Status)
	}
	var raw rawProfile
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return Profile{}, err
	}
	profile := Profile{
		Username:       raw.Username,
		Name:           raw.Name,
		AvatarTemplate: raw.AvatarTemplate,
		TrustLevel:     raw.TrustLevel,
		Active:         raw.Active,
		Silenced:       raw.Silenced,
		ID:             normalizeID(raw.ID),
	}
	return profile, nil
}

func normalizeID(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case json.Number:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}
