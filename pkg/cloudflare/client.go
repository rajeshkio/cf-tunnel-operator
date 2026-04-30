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
	zoneID    string
}

func NewClient(accountID, tunnelID, apiToken, zoneID string) *Client {
	return &Client{
		accountID: accountID,
		tunnelID:  tunnelID,
		apiToken:  apiToken,
		http:      &(http.Client{Timeout: 30 * time.Second}),
		zoneID:    zoneID,
	}
}

type apiResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

func (c *Client) ListDNSRecords(ctx context.Context, hostname string) (*DNSRecords, error) {
	url := fmt.Sprintf("%s/zones/%s/dns_records", apiBase, c.zoneID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list dns records: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	q := req.URL.Query()
	q.Add("name", hostname)
	req.URL.RawQuery = q.Encode()
	//fmt.Println(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET DNS records: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var result struct {
		apiResponse
		Records []DNSRecords `json:"result"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("cloudflare API error: %v", result.Errors)
	}
	for i, record := range result.Records {
		if record.Name == hostname {
			return &result.Records[i], nil
		}
	}
	return nil, nil
}

func (c *Client) CreateDNSRecord(ctx context.Context, hostname string) error {
	url := fmt.Sprintf("%s/zones/%s/dns_records", apiBase, c.zoneID)
	payload := &DNSRecordRequests{
		Name:    hostname,
		Type:    "CNAME",
		TTL:     1,
		Content: c.tunnelID + ".cfargotunnel.com",
		Proxied: true,
	}

	payloadByte, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadByte))
	if err != nil {
		return fmt.Errorf("building DNS record request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("POST DNS records: %w", err)
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
func (c *Client) EnsureDNSRecord(ctx context.Context, hostname string) error {
	dnsRecord, err := c.ListDNSRecords(ctx, hostname)
	if err != nil {
		return fmt.Errorf("failed to list the dns record: %w", err)
	}
	if dnsRecord == nil {
		if err := c.CreateDNSRecord(ctx, hostname); err != nil {
			return fmt.Errorf("failed to create DNS record: %w", err)
		}
		return nil
	}
	return nil

}

func (c *Client) DeleteDNSRecord(ctx context.Context, hostname string) error {
	recordID, err := c.ListDNSRecords(ctx, hostname)
	if err != nil {
		return fmt.Errorf("failed to list DNS record for hostname %s: %w", hostname, err)
	}
	if recordID == nil {
		return fmt.Errorf("DNS record not found for hostname: %s", hostname)
	}
	url := fmt.Sprintf("%s/zones/%s/dns_records/%s", apiBase, c.zoneID, recordID.Id)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to delete: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE DNS records: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("delete failed with status: %d", resp.StatusCode)
	}

	return nil

}

func (c *Client) GetTunnelConfig(ctx context.Context) (*TunnelConfig, error) {
	url := fmt.Sprintf("%s/accounts/%s/cfd_tunnel/%s/configurations", apiBase, c.accountID, c.tunnelID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-type", "application/json")

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
