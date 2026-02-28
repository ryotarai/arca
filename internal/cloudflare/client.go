package cloudflare

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	cf "github.com/cloudflare/cloudflare-go"
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

var ErrTunnelNotFound = errors.New("cloudflare tunnel not found")

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
	api, err := c.apiForToken(apiToken)
	if err != nil {
		return TokenVerification{}, err
	}
	out, err := api.VerifyAPIToken(ctx)
	if err != nil {
		return TokenVerification{}, toAPIError(err)
	}
	return TokenVerification{ID: out.ID, Status: out.Status}, nil
}

func (c *Client) VerifyAccountToken(ctx context.Context, apiToken, accountID string) error {
	api, err := c.apiForToken(apiToken)
	if err != nil {
		return err
	}
	_, _, err = api.ListTunnels(ctx, cf.AccountIdentifier(accountID), cf.TunnelListParams{
		ResultInfo: cf.ResultInfo{Page: 1, PerPage: 1},
	})
	return toAPIError(err)
}

func (c *Client) VerifyZoneAccess(ctx context.Context, apiToken, zoneID string) error {
	api, err := c.apiForToken(apiToken)
	if err != nil {
		return err
	}
	_, err = api.ZoneDetails(ctx, zoneID)
	return toAPIError(err)
}

func (c *Client) GetZone(ctx context.Context, apiToken, zoneID string) (Zone, error) {
	api, err := c.apiForToken(apiToken)
	if err != nil {
		return Zone{}, err
	}
	zone, err := api.ZoneDetails(ctx, zoneID)
	if err != nil {
		return Zone{}, toAPIError(err)
	}
	return Zone{ID: zone.ID, Name: zone.Name, Account: struct {
		ID string `json:"id"`
	}{ID: zone.Account.ID}}, nil
}

func (c *Client) CreateTunnel(ctx context.Context, apiToken, accountID, tunnelName string) (Tunnel, error) {
	api, err := c.apiForToken(apiToken)
	if err != nil {
		return Tunnel{}, err
	}
	out, err := api.CreateTunnel(ctx, cf.AccountIdentifier(accountID), cf.TunnelCreateParams{
		Name:      tunnelName,
		Secret:    tunnelSecret(),
		ConfigSrc: "cloudflare",
	})
	if err != nil {
		return Tunnel{}, toAPIError(err)
	}
	return Tunnel{ID: out.ID, Name: out.Name}, nil
}

func (c *Client) GetTunnelByName(ctx context.Context, apiToken, accountID, tunnelName string) (Tunnel, error) {
	api, err := c.apiForToken(apiToken)
	if err != nil {
		return Tunnel{}, err
	}
	isDeleted := false
	out, _, err := api.ListTunnels(ctx, cf.AccountIdentifier(accountID), cf.TunnelListParams{
		Name:      tunnelName,
		IsDeleted: &isDeleted,
		ResultInfo: cf.ResultInfo{
			Page:    1,
			PerPage: 100,
		},
	})
	if err != nil {
		return Tunnel{}, toAPIError(err)
	}
	for _, tunnel := range out {
		if strings.EqualFold(strings.TrimSpace(tunnel.Name), strings.TrimSpace(tunnelName)) {
			return Tunnel{ID: tunnel.ID, Name: tunnel.Name}, nil
		}
	}
	return Tunnel{}, ErrTunnelNotFound
}

func (c *Client) CreateTunnelToken(ctx context.Context, apiToken, accountID, tunnelID string) (string, error) {
	api, err := c.apiForToken(apiToken)
	if err != nil {
		return "", err
	}
	token, err := api.GetTunnelToken(ctx, cf.AccountIdentifier(accountID), tunnelID)
	if err != nil {
		return "", toAPIError(err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("empty tunnel token")
	}
	return token, nil
}

func (c *Client) UpsertDNSCNAME(ctx context.Context, apiToken, zoneID, hostname, target string, proxied bool) error {
	api, err := c.apiForToken(apiToken)
	if err != nil {
		return err
	}
	listOut, _, err := api.ListDNSRecords(ctx, cf.ZoneIdentifier(zoneID), cf.ListDNSRecordsParams{
		Type: "CNAME",
		Name: hostname,
		ResultInfo: cf.ResultInfo{
			Page:    1,
			PerPage: 1,
		},
	})
	if err != nil {
		return toAPIError(err)
	}
	if len(listOut) > 0 {
		_, err = api.UpdateDNSRecord(ctx, cf.ZoneIdentifier(zoneID), cf.UpdateDNSRecordParams{
			ID:      listOut[0].ID,
			Type:    "CNAME",
			Name:    hostname,
			Content: target,
			Proxied: &proxied,
		})
		return toAPIError(err)
	}
	_, err = api.CreateDNSRecord(ctx, cf.ZoneIdentifier(zoneID), cf.CreateDNSRecordParams{
		Type:    "CNAME",
		Name:    hostname,
		Content: target,
		Proxied: &proxied,
	})
	return toAPIError(err)
}

func (c *Client) UpdateTunnelIngress(ctx context.Context, apiToken, accountID, tunnelID string, rules []IngressRule) error {
	api, err := c.apiForToken(apiToken)
	if err != nil {
		return err
	}

	ingress := make([]cf.UnvalidatedIngressRule, 0, len(rules)+1)
	for _, rule := range rules {
		ingress = append(ingress, cf.UnvalidatedIngressRule{Hostname: rule.Hostname, Service: rule.Service})
	}
	ingress = append(ingress, cf.UnvalidatedIngressRule{Service: "http_status:404"})

	_, err = api.UpdateTunnelConfiguration(ctx, cf.AccountIdentifier(accountID), cf.TunnelConfigurationParams{
		TunnelID: tunnelID,
		Config: cf.TunnelConfiguration{
			Ingress: ingress,
		},
	})
	return toAPIError(err)
}

func envelopeError(errors []APIError) error {
	if len(errors) == 0 {
		return APIError{Message: "cloudflare request failed"}
	}
	return errors[0]
}

func (c *Client) apiForToken(apiToken string) (*cf.API, error) {
	opts := []cf.Option{}
	if c.httpClient != nil {
		httpClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return c.httpClient.Do(req)
		})}
		opts = append(opts, cf.HTTPClient(httpClient))
	}
	if trimmed := strings.TrimSpace(c.baseURL); trimmed != "" && trimmed != defaultBaseURL {
		opts = append(opts, cf.BaseURL(trimmed))
	}
	api, err := cf.NewWithAPIToken(apiToken, opts...)
	if err != nil {
		return nil, err
	}
	return api, nil
}

func tunnelSecret() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func toAPIError(err error) error {
	if err == nil {
		return nil
	}
	var cfErr *cf.Error
	if errors.As(err, &cfErr) {
		if len(cfErr.Errors) > 0 {
			return envelopeError([]APIError{{Code: cfErr.Errors[0].Code, Message: cfErr.Errors[0].Message}})
		}
		if len(cfErr.ErrorCodes) > 0 {
			msg := "cloudflare request failed"
			if len(cfErr.ErrorMessages) > 0 {
				msg = cfErr.ErrorMessages[0]
			}
			return envelopeError([]APIError{{Code: cfErr.ErrorCodes[0], Message: msg}})
		}
	}
	return err
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
