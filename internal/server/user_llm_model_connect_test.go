package server

import (
	"context"
	"encoding/hex"
	"testing"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/crypto"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

func TestUserLLMModel_CRUD(t *testing.T) {
	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)

	key := hex.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	enc, err := crypto.NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	service := newUserConnectService(store, authenticator, enc)

	if _, _, err := authenticator.Register(ctx, "user@example.com", "password-1234"); err != nil {
		t.Fatalf("register: %v", err)
	}
	token := loginToken(t, authenticator, "user@example.com", "password-1234")

	// List should be empty initially
	listResp, err := service.ListUserLLMModels(ctx, authRequest(arcav1.ListUserLLMModelsRequest{}, token))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got := len(listResp.Msg.GetModels()); got != 0 {
		t.Fatalf("models len = %d, want 0", got)
	}

	// Create a model
	createResp, err := service.CreateUserLLMModel(ctx, authRequest(arcav1.CreateUserLLMModelRequest{
		ConfigName:       "my-gpt4",
		EndpointType:     "openai_chat",
		CustomEndpoint:   "https://custom.api.com",
		ModelName:        "gpt-4o",
		ApiKey:           "sk-secret-key",
		MaxContextTokens: 128000,
	}, token))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	model := createResp.Msg.GetModel()
	if model.GetConfigName() != "my-gpt4" {
		t.Fatalf("config_name = %q, want %q", model.GetConfigName(), "my-gpt4")
	}
	if model.GetEndpointType() != "openai_chat" {
		t.Fatalf("endpoint_type = %q, want %q", model.GetEndpointType(), "openai_chat")
	}
	if model.GetModelName() != "gpt-4o" {
		t.Fatalf("model_name = %q, want %q", model.GetModelName(), "gpt-4o")
	}
	if !model.GetHasApiKey() {
		t.Fatal("has_api_key should be true")
	}
	if model.GetMaxContextTokens() != 128000 {
		t.Fatalf("max_context_tokens = %d, want 128000", model.GetMaxContextTokens())
	}
	modelID := model.GetId()

	// List should now contain the model
	listResp, err = service.ListUserLLMModels(ctx, authRequest(arcav1.ListUserLLMModelsRequest{}, token))
	if err != nil {
		t.Fatalf("list after create: %v", err)
	}
	if got := len(listResp.Msg.GetModels()); got != 1 {
		t.Fatalf("models len = %d, want 1", got)
	}

	// Update the model (empty api_key should keep existing)
	updateResp, err := service.UpdateUserLLMModel(ctx, authRequest(arcav1.UpdateUserLLMModelRequest{
		Id:               modelID,
		ConfigName:       "my-gpt4-updated",
		EndpointType:     "anthropic",
		ModelName:        "claude-3-opus",
		MaxContextTokens: 200000,
	}, token))
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	updated := updateResp.Msg.GetModel()
	if updated.GetConfigName() != "my-gpt4-updated" {
		t.Fatalf("updated config_name = %q, want %q", updated.GetConfigName(), "my-gpt4-updated")
	}
	if updated.GetEndpointType() != "anthropic" {
		t.Fatalf("updated endpoint_type = %q, want %q", updated.GetEndpointType(), "anthropic")
	}
	if !updated.GetHasApiKey() {
		t.Fatal("updated should still have api key")
	}

	// Delete the model
	_, err = service.DeleteUserLLMModel(ctx, authRequest(arcav1.DeleteUserLLMModelRequest{Id: modelID}, token))
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	// List should be empty again
	listResp, err = service.ListUserLLMModels(ctx, authRequest(arcav1.ListUserLLMModelsRequest{}, token))
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if got := len(listResp.Msg.GetModels()); got != 0 {
		t.Fatalf("models len = %d, want 0", got)
	}
}

func TestUserLLMModel_Validation(t *testing.T) {
	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)

	key := hex.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	enc, err := crypto.NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	service := newUserConnectService(store, authenticator, enc)

	if _, _, err := authenticator.Register(ctx, "user@example.com", "password-1234"); err != nil {
		t.Fatalf("register: %v", err)
	}
	token := loginToken(t, authenticator, "user@example.com", "password-1234")

	tests := []struct {
		name     string
		req      arcav1.CreateUserLLMModelRequest
		wantCode connect.Code
	}{
		{
			name:     "empty config_name",
			req:      arcav1.CreateUserLLMModelRequest{EndpointType: "openai_chat", ModelName: "gpt-4o"},
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name:     "invalid endpoint_type",
			req:      arcav1.CreateUserLLMModelRequest{ConfigName: "test", EndpointType: "invalid", ModelName: "gpt-4o"},
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name:     "empty model_name",
			req:      arcav1.CreateUserLLMModelRequest{ConfigName: "test", EndpointType: "openai_chat"},
			wantCode: connect.CodeInvalidArgument,
		},
	}

	for i := range tests {
		t.Run(tests[i].name, func(t *testing.T) {
			req := connect.NewRequest(&tests[i].req)
			req.Header().Set("Cookie", sessionCookieName+"="+token)
			_, err := service.CreateUserLLMModel(ctx, req)
			if err == nil {
				t.Fatal("expected error")
			}
			if got := connect.CodeOf(err); got != tests[i].wantCode {
				t.Fatalf("code = %v, want %v", got, tests[i].wantCode)
			}
		})
	}
}

func TestUserLLMModel_DuplicateConfigName(t *testing.T) {
	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)

	key := hex.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	enc, err := crypto.NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	service := newUserConnectService(store, authenticator, enc)

	if _, _, err := authenticator.Register(ctx, "user@example.com", "password-1234"); err != nil {
		t.Fatalf("register: %v", err)
	}
	token := loginToken(t, authenticator, "user@example.com", "password-1234")

	// Create first model
	_, err = service.CreateUserLLMModel(ctx, authRequest(arcav1.CreateUserLLMModelRequest{
		ConfigName:   "my-model",
		EndpointType: "openai_chat",
		ModelName:    "gpt-4o",
	}, token))
	if err != nil {
		t.Fatalf("create first: %v", err)
	}

	// Create second model with same config_name
	_, err = service.CreateUserLLMModel(ctx, authRequest(arcav1.CreateUserLLMModelRequest{
		ConfigName:   "my-model",
		EndpointType: "anthropic",
		ModelName:    "claude-3-opus",
	}, token))
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	if got := connect.CodeOf(err); got != connect.CodeAlreadyExists {
		t.Fatalf("code = %v, want %v", got, connect.CodeAlreadyExists)
	}
}

func TestUserLLMModel_NoEncryptor(t *testing.T) {
	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)

	// No encryptor
	service := newUserConnectService(store, authenticator, nil)

	if _, _, err := authenticator.Register(ctx, "user@example.com", "password-1234"); err != nil {
		t.Fatalf("register: %v", err)
	}
	token := loginToken(t, authenticator, "user@example.com", "password-1234")

	_, err := service.CreateUserLLMModel(ctx, authRequest(arcav1.CreateUserLLMModelRequest{
		ConfigName:   "test",
		EndpointType: "openai_chat",
		ModelName:    "gpt-4o",
	}, token))
	if err == nil {
		t.Fatal("expected error when encryptor is nil")
	}
	if got := connect.CodeOf(err); got != connect.CodeFailedPrecondition {
		t.Fatalf("code = %v, want %v", got, connect.CodeFailedPrecondition)
	}
}

func TestUserLLMModel_OwnerIsolation(t *testing.T) {
	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)

	key := hex.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	enc, err := crypto.NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}

	service := newUserConnectService(store, authenticator, enc)

	if _, _, err := authenticator.Register(ctx, "user1@example.com", "password-1234"); err != nil {
		t.Fatalf("register user1: %v", err)
	}
	if _, _, err := authenticator.Register(ctx, "user2@example.com", "password-1234"); err != nil {
		t.Fatalf("register user2: %v", err)
	}
	token1 := loginToken(t, authenticator, "user1@example.com", "password-1234")
	token2 := loginToken(t, authenticator, "user2@example.com", "password-1234")

	// User1 creates a model
	createResp, err := service.CreateUserLLMModel(ctx, authRequest(arcav1.CreateUserLLMModelRequest{
		ConfigName:   "user1-model",
		EndpointType: "openai_chat",
		ModelName:    "gpt-4o",
	}, token1))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	modelID := createResp.Msg.GetModel().GetId()

	// User2 should not see it
	listResp, err := service.ListUserLLMModels(ctx, authRequest(arcav1.ListUserLLMModelsRequest{}, token2))
	if err != nil {
		t.Fatalf("list user2: %v", err)
	}
	if got := len(listResp.Msg.GetModels()); got != 0 {
		t.Fatalf("user2 should see 0 models, got %d", got)
	}

	// User2 should not be able to delete user1's model
	_, err = service.DeleteUserLLMModel(ctx, authRequest(arcav1.DeleteUserLLMModelRequest{Id: modelID}, token2))
	if err == nil {
		t.Fatal("user2 should not be able to delete user1's model")
	}
	if got := connect.CodeOf(err); got != connect.CodeNotFound {
		t.Fatalf("code = %v, want %v", got, connect.CodeNotFound)
	}
}
