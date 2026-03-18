package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLLMTokenExecutor_GetToken(t *testing.T) {
	// Create a helper script that outputs a valid token JSON
	dir := t.TempDir()
	script := filepath.Join(dir, "token.sh")
	err := os.WriteFile(script, []byte(`#!/bin/sh
cat <<'EOF'
{"token": "test-token-abc", "expire_at": 9999999999}
EOF
`), 0755)
	if err != nil {
		t.Fatal(err)
	}

	executor := NewLLMTokenExecutor()
	ctx := context.Background()

	token, err := executor.GetToken(ctx, "model-1", script, "user@example.com", "user-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "test-token-abc" {
		t.Fatalf("expected token 'test-token-abc', got %q", token)
	}
}

func TestLLMTokenExecutor_CacheHit(t *testing.T) {
	dir := t.TempDir()
	counterFile := filepath.Join(dir, "count")
	os.WriteFile(counterFile, []byte("0"), 0644)

	script := filepath.Join(dir, "token.sh")
	// Script increments counter each call
	err := os.WriteFile(script, []byte(`#!/bin/sh
COUNT=$(cat `+counterFile+`)
COUNT=$((COUNT + 1))
echo $COUNT > `+counterFile+`
echo '{"token": "cached-token", "expire_at": 9999999999}'
`), 0755)
	if err != nil {
		t.Fatal(err)
	}

	executor := NewLLMTokenExecutor()
	ctx := context.Background()

	// First call
	_, err = executor.GetToken(ctx, "model-1", script, "user@example.com", "user-1")
	if err != nil {
		t.Fatal(err)
	}

	// Second call should use cache
	token, err := executor.GetToken(ctx, "model-1", script, "user@example.com", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if token != "cached-token" {
		t.Fatalf("expected 'cached-token', got %q", token)
	}

	// Verify command was only called once
	data, _ := os.ReadFile(counterFile)
	if string(data) != "1\n" {
		t.Fatalf("expected command to be called once, counter file: %q", string(data))
	}
}

func TestLLMTokenExecutor_CacheExpiry(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "token.sh")
	// Return a token that expires in the past
	err := os.WriteFile(script, []byte(`#!/bin/sh
echo '{"token": "expired-token", "expire_at": 1}'
`), 0755)
	if err != nil {
		t.Fatal(err)
	}

	executor := NewLLMTokenExecutor()
	ctx := context.Background()

	// First call
	token, err := executor.GetToken(ctx, "model-1", script, "user@example.com", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if token != "expired-token" {
		t.Fatal("expected expired-token")
	}

	// Update script to return new token
	os.WriteFile(script, []byte(`#!/bin/sh
echo '{"token": "fresh-token", "expire_at": 9999999999}'
`), 0755)

	// Should re-execute since cache is expired
	token, err = executor.GetToken(ctx, "model-1", script, "user@example.com", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if token != "fresh-token" {
		t.Fatalf("expected fresh-token after expiry, got %q", token)
	}
}

func TestLLMTokenExecutor_DifferentUsersGetDifferentCacheKeys(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "token.sh")
	// Read stdin and use user_id in token
	err := os.WriteFile(script, []byte(`#!/bin/sh
INPUT=$(cat)
USER_ID=$(echo "$INPUT" | grep -o '"user_id":"[^"]*"' | cut -d'"' -f4)
echo "{\"token\": \"token-for-${USER_ID}\", \"expire_at\": 9999999999}"
`), 0755)
	if err != nil {
		t.Fatal(err)
	}

	executor := NewLLMTokenExecutor()
	ctx := context.Background()

	token1, err := executor.GetToken(ctx, "model-1", script, "user1@example.com", "user-1")
	if err != nil {
		t.Fatal(err)
	}

	token2, err := executor.GetToken(ctx, "model-1", script, "user2@example.com", "user-2")
	if err != nil {
		t.Fatal(err)
	}

	if token1 == token2 {
		t.Fatalf("expected different tokens for different users, both got %q", token1)
	}
}

func TestLLMTokenExecutor_CommandFailure(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fail.sh")
	os.WriteFile(script, []byte(`#!/bin/sh
echo "something went wrong" >&2
exit 1
`), 0755)

	executor := NewLLMTokenExecutor()
	ctx := context.Background()

	_, err := executor.GetToken(ctx, "model-1", script, "user@example.com", "user-1")
	if err == nil {
		t.Fatal("expected error from failing command")
	}
}

func TestLLMTokenExecutor_EmptyToken(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "empty.sh")
	os.WriteFile(script, []byte(`#!/bin/sh
echo '{"token": "", "expire_at": 9999999999}'
`), 0755)

	executor := NewLLMTokenExecutor()
	ctx := context.Background()

	_, err := executor.GetToken(ctx, "model-1", script, "user@example.com", "user-1")
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestLLMTokenExecutor_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "bad.sh")
	os.WriteFile(script, []byte(`#!/bin/sh
echo 'not json'
`), 0755)

	executor := NewLLMTokenExecutor()
	ctx := context.Background()

	_, err := executor.GetToken(ctx, "model-1", script, "user@example.com", "user-1")
	if err == nil {
		t.Fatal("expected error for invalid JSON output")
	}
}
