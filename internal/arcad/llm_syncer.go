package arcad

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

// LLMSyncer periodically fetches LLM model configurations from the control
// plane and synchronises them to the local Shelley instance via its REST API.
type LLMSyncer struct {
	client     ControlPlaneClient
	shelleyURL string
	interval   time.Duration
	httpClient *http.Client
	lastHash   string
}

// NewLLMSyncer creates a new syncer. shelleyPort is the local Shelley HTTP port.
func NewLLMSyncer(client ControlPlaneClient, shelleyPort string, interval time.Duration) *LLMSyncer {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &LLMSyncer{
		client:     client,
		shelleyURL: "http://127.0.0.1:" + shelleyPort,
		interval:   interval,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Run starts the sync loop. It performs an initial sync immediately, then
// re-syncs on the configured interval.
func (s *LLMSyncer) Run(ctx context.Context) {
	s.syncOnce(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncOnce(ctx)
		}
	}
}

// SyncOnce performs a single sync cycle (exported for use in setup).
func (s *LLMSyncer) SyncOnce(ctx context.Context) {
	s.syncOnce(ctx)
}

func (s *LLMSyncer) syncOnce(ctx context.Context) {
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	desired, err := s.client.GetMachineLLMModels(fetchCtx)
	if err != nil {
		log.Printf("llm-syncer: fetch models from control plane: %v", err)
		return
	}

	hash := modelsHash(desired)
	if hash == s.lastHash {
		return
	}

	if err := s.reconcile(ctx, desired); err != nil {
		log.Printf("llm-syncer: reconcile failed: %v", err)
		return
	}

	s.lastHash = hash
	log.Printf("llm-syncer: synced %d model(s) to shelley", len(desired))
}

// shelleyModel represents a model as returned by Shelley's REST API.
type shelleyModel struct {
	ModelID      string `json:"model_id"`
	DisplayName  string `json:"display_name"`
	ProviderType string `json:"provider_type"`
	Endpoint     string `json:"endpoint"`
	APIKey       string `json:"api_key"`
	ModelName    string `json:"model_name"`
	MaxTokens    int    `json:"max_tokens"`
	Tags         string `json:"tags"`
}

// shelleyCreateRequest is the JSON body for creating/updating a Shelley model.
type shelleyCreateRequest struct {
	DisplayName  string `json:"display_name"`
	ProviderType string `json:"provider_type"`
	Endpoint     string `json:"endpoint"`
	APIKey       string `json:"api_key"`
	ModelName    string `json:"model_name"`
	MaxTokens    int    `json:"max_tokens"`
	Tags         string `json:"tags"`
}

const shelleyManagedTag = "arca-managed"

func (s *LLMSyncer) reconcile(ctx context.Context, desired []MachineLLMModel) error {
	existing, err := s.listShelleyModels(ctx)
	if err != nil {
		return fmt.Errorf("list shelley models: %w", err)
	}

	// Index managed models by display_name.
	managed := make(map[string]shelleyModel)
	for _, m := range existing {
		if isManagedModel(m) {
			managed[m.DisplayName] = m
		}
	}

	// Build desired set keyed by config_name (which maps to display_name).
	desiredByName := make(map[string]MachineLLMModel, len(desired))
	for _, m := range desired {
		desiredByName[m.ConfigName] = m
	}

	// Delete models no longer desired.
	for name, m := range managed {
		if _, ok := desiredByName[name]; !ok {
			if err := s.deleteShelleyModel(ctx, m.ModelID); err != nil {
				log.Printf("llm-syncer: delete model %s (%s): %v", m.ModelID, name, err)
			}
		}
	}

	// Create or update desired models.
	for _, dm := range desired {
		req := toShelleyRequest(dm)
		if existing, ok := managed[dm.ConfigName]; ok {
			if needsUpdate(existing, req) {
				if err := s.updateShelleyModel(ctx, existing.ModelID, req); err != nil {
					log.Printf("llm-syncer: update model %s: %v", dm.ConfigName, err)
				}
			}
		} else {
			if err := s.createShelleyModel(ctx, req); err != nil {
				log.Printf("llm-syncer: create model %s: %v", dm.ConfigName, err)
			}
		}
	}

	return nil
}

func toShelleyRequest(m MachineLLMModel) shelleyCreateRequest {
	return shelleyCreateRequest{
		DisplayName:  m.ConfigName,
		ProviderType: mapEndpointToProvider(m.EndpointType),
		Endpoint:     m.CustomEndpoint,
		APIKey:       m.APIKey,
		ModelName:    m.ModelName,
		MaxTokens:    int(m.MaxContextTokens),
		Tags:         shelleyManagedTag,
	}
}

// mapEndpointToProvider maps arca endpoint_type to Shelley provider_type.
func mapEndpointToProvider(endpointType string) string {
	switch endpointType {
	case "openai_chat":
		return "openai"
	case "openai_response":
		return "openai-responses"
	case "anthropic":
		return "anthropic"
	case "google_gemini":
		return "gemini"
	default:
		return endpointType
	}
}

func isManagedModel(m shelleyModel) bool {
	for _, tag := range strings.Split(m.Tags, ",") {
		if strings.TrimSpace(tag) == shelleyManagedTag {
			return true
		}
	}
	return false
}

func needsUpdate(existing shelleyModel, desired shelleyCreateRequest) bool {
	return existing.ProviderType != desired.ProviderType ||
		existing.Endpoint != desired.Endpoint ||
		existing.ModelName != desired.ModelName ||
		existing.MaxTokens != desired.MaxTokens ||
		desired.APIKey != "" // always update when API key is provided
}

func (s *LLMSyncer) listShelleyModels(ctx context.Context) ([]shelleyModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.shelleyURL+"/api/custom-models", nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list custom-models: status %d: %s", resp.StatusCode, string(body))
	}
	var models []shelleyModel
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return nil, fmt.Errorf("decode custom-models: %w", err)
	}
	return models, nil
}

func (s *LLMSyncer) createShelleyModel(ctx context.Context, model shelleyCreateRequest) error {
	body, err := json.Marshal(model)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.shelleyURL+"/api/custom-models", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create custom-model: status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (s *LLMSyncer) updateShelleyModel(ctx context.Context, modelID string, model shelleyCreateRequest) error {
	body, err := json.Marshal(model)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.shelleyURL+"/api/custom-models/"+modelID, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update custom-model %s: status %d: %s", modelID, resp.StatusCode, string(respBody))
	}
	return nil
}

func (s *LLMSyncer) deleteShelleyModel(ctx context.Context, modelID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.shelleyURL+"/api/custom-models/"+modelID, nil)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete custom-model %s: status %d: %s", modelID, resp.StatusCode, string(respBody))
	}
	return nil
}

// modelsHash computes a deterministic hash of the desired model set for
// change detection.
func modelsHash(models []MachineLLMModel) string {
	sorted := make([]MachineLLMModel, len(models))
	copy(sorted, models)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ConfigName < sorted[j].ConfigName
	})
	h := sha256.New()
	for _, m := range sorted {
		fmt.Fprintf(h, "%s|%s|%s|%s|%s|%d\n",
			m.ConfigName, m.EndpointType, m.CustomEndpoint,
			m.ModelName, m.APIKey, m.MaxContextTokens)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
