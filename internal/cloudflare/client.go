package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const defaultBaseURL = "https://api.cloudflare.com/client/v4"

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Client struct {
	baseURL    string
	httpClient HTTPClient
}

type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e APIError) Error() string {
	if e.Code == 0 {
		return e.Message
	}
	return fmt.Sprintf("cloudflare error %d: %s", e.Code, e.Message)
}

type TokenVerification struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type Tunnel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Zone struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Account struct {
		ID string `json:"id"`
	} `json:"account"`
}

type IngressRule struct {
	Hostname string `json:"hostname,omitempty"`
	Service  string `json:"service"`
}

type responseEnvelope[T any] struct {
	Success bool       `json:"success"`
	Errors  []APIError `json:"errors"`
	Result  T          `json:"result"`
}

type rawEnvelope struct {
	Success bool            `json:"success"`
	Errors  []APIError      `json:"errors"`
	Result  json.RawMessage `json:"result"`
}

type dnsRecord struct {
	ID string `json:"id"`
}

func NewClient(httpClient HTTPClient) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{baseURL: defaultBaseURL, httpClient: httpClient}
}

func NewClientWithBaseURL(httpClient HTTPClient, baseURL string) *Client {
	c := NewClient(httpClient)
	c.baseURL = strings.TrimSuffix(baseURL, "/")
	return c
}

func (c *Client) VerifyToken(ctx context.Context, apiToken string) (TokenVerification, error) {
	var out responseEnvelope[TokenVerification]
	if err := c.doJSON(ctx, http.MethodGet, "/user/tokens/verify", apiToken, nil, &out); err != nil {
		return TokenVerification{}, err
	}
	return out.Result, nil
}

func (c *Client) VerifyAccountToken(ctx context.Context, apiToken, accountID string) error {
	path := fmt.Sprintf("/accounts/%s/cfd_tunnel?page=1&per_page=1", url.PathEscape(accountID))
	return c.doJSON(ctx, http.MethodGet, path, apiToken, nil, nil)
}

func (c *Client) VerifyZoneAccess(ctx context.Context, apiToken, zoneID string) error {
	path := fmt.Sprintf("/zones/%s", url.PathEscape(zoneID))
	return c.doJSON(ctx, http.MethodGet, path, apiToken, nil, nil)
}

func (c *Client) GetZone(ctx context.Context, apiToken, zoneID string) (Zone, error) {
	var out responseEnvelope[Zone]
	path := fmt.Sprintf("/zones/%s", url.PathEscape(zoneID))
	if err := c.doJSON(ctx, http.MethodGet, path, apiToken, nil, &out); err != nil {
		return Zone{}, err
	}
	return out.Result, nil
}

func (c *Client) CreateTunnel(ctx context.Context, apiToken, accountID, tunnelName string) (Tunnel, error) {
	payload := map[string]string{"name": tunnelName, "config_src": "cloudflare"}
	var out responseEnvelope[Tunnel]
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/accounts/%s/cfd_tunnel", url.PathEscape(accountID)), apiToken, payload, &out); err != nil {
		return Tunnel{}, err
	}
	return out.Result, nil
}

func (c *Client) CreateTunnelToken(ctx context.Context, apiToken, accountID, tunnelID string) (string, error) {
	var out responseEnvelope[struct {
		Token string `json:"token"`
	}]
	path := fmt.Sprintf("/accounts/%s/cfd_tunnel/%s/token", url.PathEscape(accountID), url.PathEscape(tunnelID))
	if err := c.doJSON(ctx, http.MethodPost, path, apiToken, map[string]any{}, &out); err != nil {
		return "", err
	}
	return out.Result.Token, nil
}

func (c *Client) UpsertDNSCNAME(ctx context.Context, apiToken, zoneID, hostname, target string, proxied bool) error {
	queryPath := fmt.Sprintf("/zones/%s/dns_records?type=CNAME&name=%s", url.PathEscape(zoneID), url.QueryEscape(hostname))
	var listOut responseEnvelope[[]dnsRecord]
	if err := c.doJSON(ctx, http.MethodGet, queryPath, apiToken, nil, &listOut); err != nil {
		return err
	}

	payload := map[string]any{
		"type":    "CNAME",
		"name":    hostname,
		"content": target,
		"proxied": proxied,
	}
	if len(listOut.Result) > 0 {
		updatePath := fmt.Sprintf("/zones/%s/dns_records/%s", url.PathEscape(zoneID), url.PathEscape(listOut.Result[0].ID))
		return c.doJSON(ctx, http.MethodPut, updatePath, apiToken, payload, nil)
	}
	createPath := fmt.Sprintf("/zones/%s/dns_records", url.PathEscape(zoneID))
	return c.doJSON(ctx, http.MethodPost, createPath, apiToken, payload, nil)
}

func (c *Client) UpdateTunnelIngress(ctx context.Context, apiToken, accountID, tunnelID string, rules []IngressRule) error {
	ingress := make([]map[string]string, 0, len(rules)+1)
	for _, rule := range rules {
		ingress = append(ingress, map[string]string{"hostname": rule.Hostname, "service": rule.Service})
	}
	ingress = append(ingress, map[string]string{"service": "http_status:404"})

	payload := map[string]any{
		"config": map[string]any{
			"ingress": ingress,
		},
	}
	path := fmt.Sprintf("/accounts/%s/cfd_tunnel/%s/configurations", url.PathEscape(accountID), url.PathEscape(tunnelID))
	return c.doJSON(ctx, http.MethodPut, path, apiToken, payload, nil)
}

func (c *Client) doJSON(ctx context.Context, method, path, apiToken string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("cloudflare request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cloudflare request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if len(respBody) == 0 {
		return nil
	}

	var envelope rawEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if !envelope.Success {
		return envelopeError(envelope.Errors)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err == nil {
		return nil
	}
	if len(envelope.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, out); err != nil {
		return fmt.Errorf("decode result: %w", err)
	}

	return nil
}

func envelopeError(errors []APIError) error {
	if len(errors) == 0 {
		return APIError{Message: "cloudflare request failed"}
	}
	return errors[0]
}
