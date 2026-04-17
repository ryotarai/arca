package arcad

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
	"github.com/ryotarai/arca/internal/gen/arca/v1/arcav1connect"
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
	UserEmail string
	ExpiresAt time.Time
}

// MachineLLMModel represents an LLM model configuration for a machine.
type MachineLLMModel struct {
	ConfigName       string `json:"config_name"`
	EndpointType     string `json:"endpoint_type"`
	CustomEndpoint   string `json:"custom_endpoint"`
	ModelName        string `json:"model_name"`
	APIKey           string `json:"api_key"`
	MaxContextTokens int32  `json:"max_context_tokens"`
}

type ControlPlaneClient interface {
	GetExposureByHost(context.Context, string) (Exposure, error)
	ExchangeArcadSession(context.Context, string, string) (ArcadSessionClaims, error)
	ValidateArcadSession(context.Context, string, string, string) (ArcadSessionClaims, error)
	ReportMachineReadiness(ctx context.Context, ready bool, reason, containerID, arcadVersion string) (bool, error)
	GetMachineLLMModels(ctx context.Context) ([]MachineLLMModel, error)
	GetMachineAgentGuideline(ctx context.Context) (string, error)
	AuthorizeURL(string) string
}

type HTTPControlPlaneClient struct {
	baseURL      string
	authorizeURL string
	machineID    string
	machineToken string
	httpClient   *http.Client

	exposureClient arcav1connect.ExposureServiceClient
	ticketClient   arcav1connect.TicketServiceClient
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

	authInterceptor := newMachineAuthInterceptor(machineID, machineToken)
	opts := []connect.ClientOption{connect.WithInterceptors(authInterceptor)}

	return &HTTPControlPlaneClient{
		baseURL:        baseURL,
		authorizeURL:   authorizeURL,
		machineID:      machineID,
		machineToken:   machineToken,
		httpClient:     httpClient,
		exposureClient: arcav1connect.NewExposureServiceClient(httpClient, baseURL, opts...),
		ticketClient:   arcav1connect.NewTicketServiceClient(httpClient, baseURL, opts...),
	}
}

func (c *HTTPControlPlaneClient) GetExposureByHost(ctx context.Context, host string) (Exposure, error) {
	resp, err := c.exposureClient.GetMachineExposureByHostname(ctx, connect.NewRequest(&arcav1.GetMachineExposureByHostnameRequest{
		Hostname: host,
	}))
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return Exposure{}, ErrExposureNotFound
		}
		return Exposure{}, fmt.Errorf("exposure lookup failed: %w", err)
	}
	exposure := resp.Msg.GetExposure()
	if exposure == nil {
		return Exposure{}, fmt.Errorf("missing exposure in response")
	}
	out := Exposure{
		Host:   strings.TrimSpace(exposure.GetHostname()),
		Target: strings.TrimSpace(exposure.GetService()),
	}
	if out.Host == "" {
		out.Host = host
	}
	return out, nil
}

func (c *HTTPControlPlaneClient) ExchangeArcadSession(ctx context.Context, host, token string) (ArcadSessionClaims, error) {
	resp, err := c.ticketClient.ExchangeArcadSession(ctx, connect.NewRequest(&arcav1.ExchangeArcadSessionRequest{
		Token:    strings.TrimSpace(token),
		Hostname: strings.TrimSpace(host),
	}))
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeUnauthenticated, connect.CodePermissionDenied, connect.CodeInvalidArgument:
			return ArcadSessionClaims{}, ErrInvalidTicket
		}
		return ArcadSessionClaims{}, fmt.Errorf("arcad session exchange failed: %w", err)
	}

	msg := resp.Msg
	sessionID := strings.TrimSpace(msg.GetSessionId())
	user := msg.GetUser()
	userID := ""
	userEmail := ""
	if user != nil {
		userID = strings.TrimSpace(user.GetId())
		userEmail = strings.TrimSpace(user.GetEmail())
	}
	if sessionID == "" || userID == "" {
		return ArcadSessionClaims{}, fmt.Errorf("invalid arcad session exchange response")
	}

	expiresAtUnix := msg.GetExpiresAtUnix()
	expiresAt := time.Unix(expiresAtUnix, 0).UTC()
	if expiresAtUnix <= 0 {
		expiresAt = time.Now().Add(8 * time.Hour)
	}
	return ArcadSessionClaims{SessionID: sessionID, UserID: userID, UserEmail: userEmail, ExpiresAt: expiresAt}, nil
}

func (c *HTTPControlPlaneClient) ValidateArcadSession(ctx context.Context, host, path, sessionID string) (ArcadSessionClaims, error) {
	resp, err := c.ticketClient.ValidateArcadSession(ctx, connect.NewRequest(&arcav1.ValidateArcadSessionRequest{
		SessionId: strings.TrimSpace(sessionID),
		Hostname:  strings.TrimSpace(host),
		Path:      strings.TrimSpace(path),
	}))
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeUnauthenticated, connect.CodePermissionDenied, connect.CodeInvalidArgument, connect.CodeNotFound:
			return ArcadSessionClaims{}, ErrInvalidSession
		}
		return ArcadSessionClaims{}, fmt.Errorf("arcad session validation failed: %w", err)
	}

	user := resp.Msg.GetUser()
	if user == nil || strings.TrimSpace(user.GetId()) == "" {
		return ArcadSessionClaims{}, fmt.Errorf("invalid arcad session validation response")
	}
	return ArcadSessionClaims{
		UserID:    strings.TrimSpace(user.GetId()),
		UserEmail: strings.TrimSpace(user.GetEmail()),
	}, nil
}

func (c *HTTPControlPlaneClient) GetMachineLLMModels(ctx context.Context) ([]MachineLLMModel, error) {
	resp, err := c.exposureClient.GetMachineLLMModels(ctx, connect.NewRequest(&arcav1.GetMachineLLMModelsRequest{}))
	if err != nil {
		return nil, fmt.Errorf("get machine llm models failed: %w", err)
	}
	models := make([]MachineLLMModel, 0, len(resp.Msg.GetModels()))
	for _, m := range resp.Msg.GetModels() {
		models = append(models, MachineLLMModel{
			ConfigName:       m.GetConfigName(),
			EndpointType:     m.GetEndpointType(),
			CustomEndpoint:   m.GetCustomEndpoint(),
			ModelName:        m.GetModelName(),
			APIKey:           m.GetApiKey(),
			MaxContextTokens: m.GetMaxContextTokens(),
		})
	}
	return models, nil
}

func (c *HTTPControlPlaneClient) GetMachineAgentGuideline(ctx context.Context) (string, error) {
	resp, err := c.exposureClient.GetMachineAgentGuideline(ctx, connect.NewRequest(&arcav1.GetMachineAgentGuidelineRequest{}))
	if err != nil {
		return "", fmt.Errorf("get machine agent guideline failed: %w", err)
	}
	return resp.Msg.GetGuideline(), nil
}

func (c *HTTPControlPlaneClient) AuthorizeURL(target string) string {
	return c.authorizeURL + "?target=" + url.QueryEscape(target)
}

func (c *HTTPControlPlaneClient) ReportMachineReadiness(ctx context.Context, ready bool, reason, containerID, arcadVersion string) (bool, error) {
	resp, err := c.exposureClient.ReportMachineReadiness(ctx, connect.NewRequest(&arcav1.ReportMachineReadinessRequest{
		Ready:        ready,
		Reason:       strings.TrimSpace(reason),
		MachineId:    c.machineID,
		ContainerId:  strings.TrimSpace(containerID),
		ArcadVersion: strings.TrimSpace(arcadVersion),
	}))
	if err != nil {
		switch connect.CodeOf(err) {
		case connect.CodeUnauthenticated, connect.CodePermissionDenied, connect.CodeInvalidArgument:
			return false, fmt.Errorf("report machine readiness unauthorized: %w", err)
		}
		return false, fmt.Errorf("report machine readiness failed: %w", err)
	}
	return resp.Msg.GetAccepted(), nil
}

// Compile-time guard ensuring HTTPControlPlaneClient implements ControlPlaneClient.
var _ ControlPlaneClient = (*HTTPControlPlaneClient)(nil)
