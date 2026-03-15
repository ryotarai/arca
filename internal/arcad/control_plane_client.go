package arcad

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	getExposureByHostnameEndpoint  = "/arca.v1.TunnelService/GetMachineExposureByHostname"
	exchangeArcadSessionEndpoint   = "/arca.v1.TicketService/ExchangeArcadSession"
	validateArcadSessionEndpoint   = "/arca.v1.TicketService/ValidateArcadSession"
	reportMachineReadinessEndpoint = "/arca.v1.TunnelService/ReportMachineReadiness"
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

type ArcadSessionClaims struct {
	SessionID string
	UserID    string
	ExpiresAt time.Time
}

type ControlPlaneClient interface {
	GetExposureByHost(context.Context, string) (Exposure, error)
	ExchangeArcadSession(context.Context, string, string) (ArcadSessionClaims, error)
	ValidateArcadSession(context.Context, string, string, string) (ArcadSessionClaims, error)
	ReportMachineReadiness(ctx context.Context, ready bool, reason, containerID, arcadVersion string) (bool, error)
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

func (c *HTTPControlPlaneClient) ExchangeArcadSession(ctx context.Context, host, token string) (ArcadSessionClaims, error) {
	payload := map[string]string{
		"token":    strings.TrimSpace(token),
		"hostname": strings.TrimSpace(host),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ArcadSessionClaims{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+exchangeArcadSessionEndpoint, bytes.NewReader(body))
	if err != nil {
		return ArcadSessionClaims{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Arca-Machine-ID", c.machineID)
	if strings.TrimSpace(c.machineToken) != "" {
		req.Header.Set("Authorization", "Bearer "+c.machineToken)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ArcadSessionClaims{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusBadRequest {
		return ArcadSessionClaims{}, ErrInvalidTicket
	}
	if resp.StatusCode != http.StatusOK {
		return ArcadSessionClaims{}, fmt.Errorf("arcad session exchange failed: status %d", resp.StatusCode)
	}
	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return ArcadSessionClaims{}, fmt.Errorf("decode arcad session exchange: %w", err)
	}
	sessionID := firstNonEmptyString(
		stringValue(decoded["sessionId"]),
		stringValue(decoded["session_id"]),
	)
	userID := userIDFromPayload(decoded)
	if sessionID == "" || userID == "" {
		return ArcadSessionClaims{}, fmt.Errorf("invalid arcad session exchange response")
	}
	expiresAtUnix, _ := firstInt64Value(
		decoded["expiresAtUnix"],
		decoded["expires_at_unix"],
	)
	expiresAt := time.Unix(expiresAtUnix, 0).UTC()
	if expiresAtUnix <= 0 {
		expiresAt = time.Now().Add(8 * time.Hour)
	}
	return ArcadSessionClaims{SessionID: sessionID, UserID: userID, ExpiresAt: expiresAt}, nil
}

func (c *HTTPControlPlaneClient) ValidateArcadSession(ctx context.Context, host, path, sessionID string) (ArcadSessionClaims, error) {
	payload := map[string]string{
		"session_id": strings.TrimSpace(sessionID),
		"hostname":   strings.TrimSpace(host),
		"path":       strings.TrimSpace(path),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ArcadSessionClaims{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+validateArcadSessionEndpoint, bytes.NewReader(body))
	if err != nil {
		return ArcadSessionClaims{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Arca-Machine-ID", c.machineID)
	if strings.TrimSpace(c.machineToken) != "" {
		req.Header.Set("Authorization", "Bearer "+c.machineToken)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ArcadSessionClaims{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound {
		return ArcadSessionClaims{}, ErrInvalidSession
	}
	if resp.StatusCode != http.StatusOK {
		return ArcadSessionClaims{}, fmt.Errorf("arcad session validation failed: status %d", resp.StatusCode)
	}
	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return ArcadSessionClaims{}, fmt.Errorf("decode arcad session validation: %w", err)
	}
	userID := userIDFromPayload(decoded)
	if userID == "" {
		return ArcadSessionClaims{}, fmt.Errorf("invalid arcad session validation response")
	}
	return ArcadSessionClaims{UserID: userID}, nil
}

func (c *HTTPControlPlaneClient) AuthorizeURL(target string) string {
	return c.authorizeURL + "?target=" + url.QueryEscape(target)
}

func (c *HTTPControlPlaneClient) ReportMachineReadiness(ctx context.Context, ready bool, reason, containerID, arcadVersion string) (bool, error) {
	payload := map[string]any{
		"ready":         ready,
		"reason":        strings.TrimSpace(reason),
		"machine_id":    c.machineID,
		"container_id":  strings.TrimSpace(containerID),
		"arcad_version": strings.TrimSpace(arcadVersion),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+reportMachineReadinessEndpoint, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Arca-Machine-ID", c.machineID)
	if strings.TrimSpace(c.machineToken) != "" {
		req.Header.Set("Authorization", "Bearer "+c.machineToken)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusBadRequest {
		return false, fmt.Errorf("report machine readiness unauthorized: status %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("report machine readiness failed: status %d", resp.StatusCode)
	}

	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return false, fmt.Errorf("decode report machine readiness: %w", err)
	}
	accepted, ok := decoded["accepted"].(bool)
	if !ok {
		return false, fmt.Errorf("invalid report machine readiness response")
	}
	return accepted, nil
}

func userIDFromPayload(payload map[string]any) string {
	user, ok := payload["user"].(map[string]any)
	if !ok {
		return ""
	}
	return stringValue(user["id"])
}

func stringValue(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstInt64Value(values ...any) (int64, bool) {
	for _, value := range values {
		if parsed, ok := int64Value(value); ok {
			return parsed, true
		}
	}
	return 0, false
}

func int64Value(v any) (int64, bool) {
	switch value := v.(type) {
	case float64:
		return int64(value), true
	case int64:
		return value, true
	case int:
		return int64(value), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}
