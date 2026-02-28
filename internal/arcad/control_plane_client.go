package arcad

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	getExposureByHostnameEndpoint = "/arca.v1.TunnelService/GetMachineExposureByHostname"
	verifyTicketEndpoint          = "/arca.v1.TicketService/VerifyTicket"
)

// Exposure describes host routing and visibility.
type Exposure struct {
	Host   string `json:"host"`
	Target string `json:"target"`
	Public bool   `json:"public"`
}

func (e Exposure) targetURL() (*url.URL, error) {
	target := strings.TrimSpace(e.Target)
	if target == "" {
		return nil, fmt.Errorf("empty target")
	}
	if !strings.Contains(target, "://") {
		target = "http://" + target
	}
	u, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("parse exposure target %q: %w", e.Target, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid target %q", e.Target)
	}
	return u, nil
}

type TicketClaims struct {
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ControlPlaneClient interface {
	GetExposureByHost(context.Context, string) (Exposure, error)
	VerifyTicket(context.Context, string, string) (TicketClaims, error)
	AuthorizeURL(string) string
}

type HTTPControlPlaneClient struct {
	baseURL      string
	authorizeURL string
	machineID    string
	machineToken string
	httpClient   *http.Client
}

func NewHTTPControlPlaneClient(baseURL, authorizeURL, machineID, machineToken string, httpClient *http.Client) *HTTPControlPlaneClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	baseURL = strings.TrimRight(baseURL, "/")
	authorizeURL = strings.TrimSpace(authorizeURL)
	if authorizeURL == "" {
		authorizeURL = baseURL + "/console/authorize"
	}
	return &HTTPControlPlaneClient{
		baseURL:      baseURL,
		authorizeURL: authorizeURL,
		machineID:    machineID,
		machineToken: machineToken,
		httpClient:   httpClient,
	}
}

func (c *HTTPControlPlaneClient) GetExposureByHost(ctx context.Context, host string) (Exposure, error) {
	payload := map[string]string{"hostname": host}
	body, err := json.Marshal(payload)
	if err != nil {
		return Exposure{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+getExposureByHostnameEndpoint, bytes.NewReader(body))
	if err != nil {
		return Exposure{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Arca-Machine-ID", c.machineID)
	if strings.TrimSpace(c.machineToken) != "" {
		req.Header.Set("Authorization", "Bearer "+c.machineToken)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Exposure{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return Exposure{}, ErrExposureNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return Exposure{}, fmt.Errorf("exposure lookup failed: status %d", resp.StatusCode)
	}
	var decoded struct {
		Exposure *struct {
			Hostname string `json:"hostname"`
			Service  string `json:"service"`
			Public   bool   `json:"public"`
		} `json:"exposure"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return Exposure{}, fmt.Errorf("decode exposure: %w", err)
	}
	if decoded.Exposure == nil {
		return Exposure{}, fmt.Errorf("missing exposure in response")
	}
	exposure := Exposure{
		Host:   strings.TrimSpace(decoded.Exposure.Hostname),
		Target: strings.TrimSpace(decoded.Exposure.Service),
		Public: decoded.Exposure.Public,
	}
	if exposure.Host == "" {
		exposure.Host = host
	}
	return exposure, nil
}

func (c *HTTPControlPlaneClient) VerifyTicket(ctx context.Context, host, ticket string) (TicketClaims, error) {
	_ = host
	payload := map[string]string{
		"ticket": ticket,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return TicketClaims{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+verifyTicketEndpoint, bytes.NewReader(body))
	if err != nil {
		return TicketClaims{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Arca-Machine-ID", c.machineID)
	if strings.TrimSpace(c.machineToken) != "" {
		req.Header.Set("Authorization", "Bearer "+c.machineToken)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TicketClaims{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusBadRequest {
		return TicketClaims{}, ErrInvalidTicket
	}
	if resp.StatusCode != http.StatusOK {
		return TicketClaims{}, fmt.Errorf("ticket verification failed: status %d", resp.StatusCode)
	}
	var decoded struct {
		User *struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return TicketClaims{}, fmt.Errorf("decode ticket claims: %w", err)
	}
	if decoded.User == nil || strings.TrimSpace(decoded.User.ID) == "" {
		return TicketClaims{}, fmt.Errorf("invalid ticket response")
	}
	claims := TicketClaims{
		UserID:    strings.TrimSpace(decoded.User.ID),
		ExpiresAt: time.Now().Add(8 * time.Hour),
	}
	if claims.ExpiresAt.IsZero() {
		claims.ExpiresAt = time.Now().Add(8 * time.Hour)
	}
	return claims, nil
}

func (c *HTTPControlPlaneClient) AuthorizeURL(target string) string {
	return c.authorizeURL + "?target=" + url.QueryEscape(target)
}
