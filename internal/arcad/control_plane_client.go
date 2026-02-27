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
	temporaryExposureEndpoint     = "/api/internal/machine/exposure"
	temporaryVerifyTicketEndpoint = "/api/internal/machine/verify-ticket"
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
	machineID    string
	machineToken string
	httpClient   *http.Client
}

func NewHTTPControlPlaneClient(baseURL, machineID, machineToken string, httpClient *http.Client) *HTTPControlPlaneClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &HTTPControlPlaneClient{
		baseURL:      strings.TrimRight(baseURL, "/"),
		machineID:    machineID,
		machineToken: machineToken,
		httpClient:   httpClient,
	}
}

func (c *HTTPControlPlaneClient) GetExposureByHost(ctx context.Context, host string) (Exposure, error) {
	reqURL := c.baseURL + temporaryExposureEndpoint + "?host=" + url.QueryEscape(host)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return Exposure{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.machineToken)
	req.Header.Set("X-Arca-Machine-ID", c.machineID)
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
	var exposure Exposure
	if err := json.NewDecoder(resp.Body).Decode(&exposure); err != nil {
		return Exposure{}, fmt.Errorf("decode exposure: %w", err)
	}
	if exposure.Host == "" {
		exposure.Host = host
	}
	return exposure, nil
}

func (c *HTTPControlPlaneClient) VerifyTicket(ctx context.Context, host, ticket string) (TicketClaims, error) {
	payload := map[string]string{
		"machine_id": c.machineID,
		"host":       host,
		"ticket":     ticket,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return TicketClaims{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+temporaryVerifyTicketEndpoint, bytes.NewReader(body))
	if err != nil {
		return TicketClaims{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.machineToken)
	req.Header.Set("Content-Type", "application/json")
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
	var claims TicketClaims
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		return TicketClaims{}, fmt.Errorf("decode ticket claims: %w", err)
	}
	if claims.ExpiresAt.IsZero() {
		claims.ExpiresAt = time.Now().Add(8 * time.Hour)
	}
	return claims, nil
}

func (c *HTTPControlPlaneClient) AuthorizeURL(target string) string {
	return c.baseURL + "/auth/authorize?target=" + url.QueryEscape(target)
}
