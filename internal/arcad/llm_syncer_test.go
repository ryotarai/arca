package arcad

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestMapEndpointToProvider(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"openai_chat", "openai"},
		{"openai_response", "openai-responses"},
		{"anthropic", "anthropic"},
		{"google_gemini", "gemini"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		got := mapEndpointToProvider(tt.input)
		if got != tt.want {
			t.Errorf("mapEndpointToProvider(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestModelsHash_Deterministic(t *testing.T) {
	models := []MachineLLMModel{
		{ConfigName: "b", EndpointType: "anthropic", ModelName: "claude-3"},
		{ConfigName: "a", EndpointType: "openai_chat", ModelName: "gpt-4o"},
	}
	h1 := modelsHash(models)
	h2 := modelsHash(models)
	if h1 != h2 {
		t.Fatalf("hash not deterministic: %s != %s", h1, h2)
	}

	// Reversed order should produce the same hash (sorted internally).
	reversed := []MachineLLMModel{models[1], models[0]}
	h3 := modelsHash(reversed)
	if h1 != h3 {
		t.Fatalf("hash order-dependent: %s != %s", h1, h3)
	}
}

func TestModelsHash_DifferentOnChange(t *testing.T) {
	m1 := []MachineLLMModel{{ConfigName: "a", ModelName: "gpt-4o"}}
	m2 := []MachineLLMModel{{ConfigName: "a", ModelName: "gpt-4o-mini"}}
	if modelsHash(m1) == modelsHash(m2) {
		t.Fatal("different models should produce different hashes")
	}
}

func TestIsManagedModel(t *testing.T) {
	tests := []struct {
		tags string
		want bool
	}{
		{"arca-managed", true},
		{"foo,arca-managed,bar", true},
		{"", false},
		{"other-tag", false},
	}
	for _, tt := range tests {
		got := isManagedModel(shelleyModel{Tags: tt.tags})
		if got != tt.want {
			t.Errorf("isManagedModel(tags=%q) = %v, want %v", tt.tags, got, tt.want)
		}
	}
}

type llmStubControlPlane struct {
	models []MachineLLMModel
	err    error
}

func (s *llmStubControlPlane) GetExposureByHost(_ context.Context, _ string) (Exposure, error) {
	return Exposure{}, nil
}
func (s *llmStubControlPlane) ExchangeArcadSession(_ context.Context, _, _ string) (ArcadSessionClaims, error) {
	return ArcadSessionClaims{}, nil
}
func (s *llmStubControlPlane) ValidateArcadSession(_ context.Context, _, _, _ string) (ArcadSessionClaims, error) {
	return ArcadSessionClaims{}, nil
}
func (s *llmStubControlPlane) ReportMachineReadiness(_ context.Context, _ bool, _, _, _ string) (bool, error) {
	return true, nil
}
func (s *llmStubControlPlane) GetMachineLLMModels(_ context.Context) ([]MachineLLMModel, error) {
	return s.models, s.err
}
func (s *llmStubControlPlane) GetMachineAgentGuideline(_ context.Context) (string, error) {
	return "", nil
}
func (s *llmStubControlPlane) AuthorizeURL(_ string) string { return "" }

func TestNewLLMSyncer_ShelleyURL(t *testing.T) {
	syncer := NewLLMSyncer(nil, "21032", 0)
	want := "http://127.0.0.1:21032/__arca/shelley"
	if syncer.shelleyURL != want {
		t.Errorf("shelleyURL = %q, want %q", syncer.shelleyURL, want)
	}
}

func TestLLMSyncer_CreatesAndDeletesModels(t *testing.T) {
	var mu sync.Mutex
	var created []shelleyCreateRequest
	var deleted []string
	existingModels := []shelleyModel{
		{ModelID: "custom-old", DisplayName: "old-model", Tags: "arca-managed", ProviderType: "openai"},
	}

	shelley := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/custom-models":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(existingModels)
		case r.Method == http.MethodPost && r.URL.Path == "/api/custom-models":
			var req shelleyCreateRequest
			json.NewDecoder(r.Body).Decode(&req)
			created = append(created, req)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"model_id": "custom-new"})
		case r.Method == http.MethodDelete:
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer shelley.Close()

	// Extract port from test server URL.
	cp := &llmStubControlPlane{
		models: []MachineLLMModel{
			{ConfigName: "new-model", EndpointType: "anthropic", ModelName: "claude-3", APIKey: "sk-test", MaxContextTokens: 200000},
		},
	}

	syncer := NewLLMSyncer(cp, "", 0)
	syncer.shelleyURL = shelley.URL

	syncer.SyncOnce(context.Background())

	mu.Lock()
	defer mu.Unlock()

	if len(created) != 1 {
		t.Fatalf("expected 1 create, got %d", len(created))
	}
	if created[0].DisplayName != "new-model" {
		t.Errorf("created display_name = %q, want %q", created[0].DisplayName, "new-model")
	}
	if created[0].ProviderType != "anthropic" {
		t.Errorf("created provider_type = %q, want %q", created[0].ProviderType, "anthropic")
	}
	if created[0].Tags != "arca-managed" {
		t.Errorf("created tags = %q, want %q", created[0].Tags, "arca-managed")
	}
	if len(deleted) != 1 {
		t.Fatalf("expected 1 delete, got %d", len(deleted))
	}
	if deleted[0] != "/api/custom-models/custom-old" {
		t.Errorf("deleted path = %q, want %q", deleted[0], "/api/custom-models/custom-old")
	}
}

func TestLLMSyncer_SkipsWhenHashUnchanged(t *testing.T) {
	callCount := 0
	shelley := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]shelleyModel{})
	}))
	defer shelley.Close()

	cp := &llmStubControlPlane{
		models: []MachineLLMModel{
			{ConfigName: "model-a", EndpointType: "openai_chat", ModelName: "gpt-4o"},
		},
	}

	syncer := NewLLMSyncer(cp, "", 0)
	syncer.shelleyURL = shelley.URL

	syncer.SyncOnce(context.Background())
	firstCallCount := callCount

	// Second sync should skip because hash hasn't changed.
	syncer.SyncOnce(context.Background())
	if callCount != firstCallCount {
		t.Errorf("expected no additional shelley calls on second sync, got %d total", callCount)
	}
}
