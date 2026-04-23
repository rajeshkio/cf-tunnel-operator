package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const apiBase = "https://api.cloudflare.com/client/v4"

type Client struct {
	accountID string
	tunnelID  string
	apiToken  string
	http      *http.Client
}

func NewClient(accountID, tunnelID, apiToken string) *Client {
	return &Client{
		accountID: accountID,
		tunnelID:  tunnelID,
		apiToken:  apiToken,
		http:      &(http.Client{Timeout: 30 * time.Second}),
	}
}

type apiResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

func (c *Client) GetTunnelConfig(ctx context.Context) (*TunnelConfig, error) {
	url := fmt.Sprintf("%s/accounts/%s/cfd_tunnel/%s/configurations", apiBase, c.accountID, c.tunnelID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-type", "Application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET tunnel config: %w", err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var result struct {
		apiResponse
		Result struct {
			Config TunnelConfig `json:"config"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("cloudflare API error: %v", result.Errors)
	}

	return &result.Result.Config, nil
}

func (c *Client) PutTunnelConfig(ctx context.Context, config TunnelConfig) error {
	url := fmt.Sprintf("%s/accounts/%s/cfd_tunnel/%s/configurations", apiBase, c.accountID, c.tunnelID)

	payload, err := json.Marshal(struct {
		Config TunnelConfig `json:"config"`
	}{Config: config})

	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("PUT tunnel config: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	var result apiResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("cloudflare API error: %v", result.Errors)
	}

	return nil
}
