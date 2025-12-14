package chatgpt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

type AccountStats struct {
	SeatsInUse     int    `json:"seats_in_use"`
	SeatsEntitled  int    `json:"seats_entitled"`
	PlanType       string `json:"plan_type"`
	ActiveStart    string `json:"active_start"`
	ActiveUntil    string `json:"active_until"`
	BillingPeriod  string `json:"billing_period"`
	WillRenew      bool   `json:"will_renew"`
	IsDelinquent   bool   `json:"is_delinquent"`
}

type InvitesResponse struct {
	Total int `json:"total"`
}

func (c *Client) buildHeaders(accountID, authToken string) http.Header {
	h := http.Header{}
	h.Set("Accept", "*/*")
	h.Set("Accept-Language", "zh-CN,zh;q=0.9")
	h.Set("Authorization", authToken)
	h.Set("ChatGPT-Account-ID", accountID)
	h.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	return h
}

func (c *Client) GetAccountStats(ctx context.Context, accountID, authToken string) (*AccountStats, error) {
	url := fmt.Sprintf("https://chatgpt.com/backend-api/subscriptions?account_id=%s", accountID)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header = c.buildHeaders(accountID, authToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var stats AccountStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

func (c *Client) GetPendingInvites(ctx context.Context, accountID, authToken string) (int, error) {
	url := fmt.Sprintf("https://chatgpt.com/backend-api/accounts/%s/invites?offset=0&limit=1&query=", accountID)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header = c.buildHeaders(accountID, authToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var invites InvitesResponse
	if err := json.NewDecoder(resp.Body).Decode(&invites); err != nil {
		return 0, err
	}
	return invites.Total, nil
}
