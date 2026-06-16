package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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

func (c *Client) ListAccessApplications(ctx context.Context) ([]AccessApplication, error) {
	url := fmt.Sprintf("%s/accounts/%s/access/apps", apiBase, c.accountID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list access apps records: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Get access app records %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var result struct {
		apiResponse
		Records []AccessApplication `json:"result"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("cloudflare API error: %v", result.Errors)
	}
	return result.Records, nil
}

func (c *Client) ListAccessPolicies(ctx context.Context, appId string) ([]AccessPolicyLists, error) {
	url := fmt.Sprintf("%s/accounts/%s/access/apps/%s/policies", apiBase, c.accountID, appId)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list policies lists: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Lisst policies %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var result struct {
		apiResponse
		Records []AccessPolicyLists `json:"result"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("cloudflare API error: %v", result.Errors)
	}
	return result.Records, nil
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

func (c *Client) CreateAccessApplication(ctx context.Context, name string) (string, error) {
	url := fmt.Sprintf("%s/accounts/%s/access/apps", apiBase, c.accountID)
	payload := &AccessApplication{
		Name:   name,
		Domain: name,
		Type:   "self_hosted",
		Destination: []AccessDestination{
			{
				Type: "public",
				URI:  name,
			},
		},
	}

	payloadByte, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("building application access request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadByte))
	if err != nil {
		return "", fmt.Errorf("building DNS record request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("POST Access Application creation: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	var apiResult struct {
		apiResponse
		Result AccessApplication `json:"result"`
	}
	//var apiResponse apiResponse
	err = json.Unmarshal(body, &apiResult)
	if err != nil {
		return "", fmt.Errorf("Failed to unmarshal the api response: %w", err)
	}

	if !apiResult.Success {
		return "", fmt.Errorf("cloudflare API error: %v", apiResult.Errors)
	}
	//fmt.Println(apiResult.Result)
	return apiResult.Result.ID, nil
}

func (c *Client) CreateAccessPolicies(ctx context.Context, appId, hostname, decision string, emails []string) (string, error) {
	url := fmt.Sprintf("%s/accounts/%s/access/apps/%s/policies", apiBase, c.accountID, appId)
	var AccessEmails []AccessEmailRule
	for _, email := range emails {
		rule := AccessEmailRule{}
		rule.Email.Email = email
		AccessEmails = append(AccessEmails, rule)
	}
	payload := &AccessPolicyRequest{
		Name:     "cto-" + hostname,
		Decision: decision,
		Include:  AccessEmails,
	}

	payloadByte, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("building access policy request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadByte))
	if err != nil {
		return "", fmt.Errorf("creating http request for access policy: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("POST access policy creation: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	var apiResult struct {
		apiResponse
		Result AccessPolicyLists `json:"result"`
	}

	//var apiResponse apiResponse
	err = json.Unmarshal(body, &apiResult)
	if err != nil {
		return "", fmt.Errorf("Failed to unmarshal the api response: %w", err)
	}

	if !apiResult.Success {
		return "", fmt.Errorf("cloudflare API error: %v", apiResult.Errors)
	}
	//fmt.Println(apiResult.Result)
	return apiResult.Result.ID, nil
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
	slog.Info("DNS record created", "hostname", hostname)
	return nil

}

func (c *Client) EnsureAccessApplication(ctx context.Context, hostname string) (string, error) {
	apps, err := c.ListAccessApplications(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list access application: %w", err)
	}

	for _, app := range apps {
		if app.Domain == hostname {
			slog.Info("Access application already exists", "hostname", hostname, "appID", app.ID)
			return app.ID, nil
		}
	}

	appId, err := c.CreateAccessApplication(ctx, hostname)
	if err != nil {
		return "", fmt.Errorf("Failed to create Access Application: %w", err)
	}
	slog.Info("access application created", "hostname", hostname, "appID", appId)
	return appId, nil
}

func (c *Client) UpdateAccessPolicies(ctx context.Context, appId, hostname, decision, policyId string, emails []string) error {
	url := fmt.Sprintf("%s/accounts/%s/access/apps/%s/policies/%s", apiBase, c.accountID, appId, policyId)
	var AccessEmails []AccessEmailRule
	for _, email := range emails {
		rule := AccessEmailRule{}
		rule.Email.Email = email
		AccessEmails = append(AccessEmails, rule)
	}
	payload := &AccessPolicyRequest{
		Name:     "cto-" + hostname,
		Decision: decision,
		Include:  AccessEmails,
	}

	payloadByte, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("building access policy request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(payloadByte))
	if err != nil {
		return fmt.Errorf("creating http request for access policy: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("PUT access policy update: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	var apiResult struct {
		apiResponse
		Result AccessPolicyLists `json:"result"`
	}

	//var apiResponse apiResponse
	err = json.Unmarshal(body, &apiResult)
	if err != nil {
		return fmt.Errorf("Failed to unmarshal the api response: %w", err)
	}

	if !apiResult.Success {
		return fmt.Errorf("cloudflare API error: %v", apiResult.Errors)
	}
	//fmt.Println(apiResult.Result)
	return nil
}

func (c *Client) EnsureAccessPolicy(ctx context.Context, appId, hostname, decision string, emails []string) error {
	policies, err := c.ListAccessPolicies(ctx, appId)
	if err != nil {
		return fmt.Errorf("failed to list access policies: %w", err)
	}
	for _, policy := range policies {
		if policy.Name == "cto-"+hostname {
			err = c.UpdateAccessPolicies(ctx, appId, hostname, decision, policy.ID, emails)
			if err != nil {
				return fmt.Errorf("failed to update the policy %s: %w", policy.ID, err)
			}
			slog.Info("access policy updated", "hostname", hostname, "appID", appId, "policyID", policy.ID)
			return nil
		}
	}

	policyId, err := c.CreateAccessPolicies(ctx, appId, hostname, decision, emails)
	if err != nil {
		return fmt.Errorf("Failed to create Access Policy: %w", err)
	}
	slog.Info("access policy created", "hostname", hostname, "appID", appId, "policyID", policyId)
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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("delete failed with status: %d", resp.StatusCode)
	}

	return nil

}

func (c *Client) DeleteAccessApplication(ctx context.Context, hostname string) error {
	appIds, err := c.ListAccessApplications(ctx)
	if err != nil {
		return fmt.Errorf("failed to list Access Application for hostname %s: %w", hostname, err)
	}

	for _, appId := range appIds {
		if appId.Domain == hostname {
			url := fmt.Sprintf("%s/accounts/%s/access/apps/%s", apiBase, c.accountID, appId.ID)
			req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
			if err != nil {
				return fmt.Errorf("failed to delete: %w", err)
			}

			req.Header.Set("Authorization", "Bearer "+c.apiToken)
			resp, err := c.http.Do(req)
			if err != nil {
				return fmt.Errorf("DELETE Access Application: %w", err)
			}
			defer resp.Body.Close()
			// Accept any 2xx as success, as cloudflare returns 202
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return fmt.Errorf("delete failed with status: %d", resp.StatusCode)
			}
			return nil
		}
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
