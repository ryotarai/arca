package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// LLMTokenExecutor executes external commands to obtain LLM API tokens and caches results.
type LLMTokenExecutor struct {
	mu    sync.Mutex
	cache map[string]*cachedToken
}

type cachedToken struct {
	token    string
	expireAt time.Time
}

type tokenCommandInput struct {
	Email  string `json:"email"`
	UserID string `json:"user_id"`
}

type tokenCommandOutput struct {
	Token    string `json:"token"`
	ExpireAt int64  `json:"expire_at"`
}

func NewLLMTokenExecutor() *LLMTokenExecutor {
	return &LLMTokenExecutor{
		cache: make(map[string]*cachedToken),
	}
}

// GetToken executes the given command to retrieve a token for the specified user.
// Results are cached until expire_at. The command receives user info on stdin and
// must output JSON {"token": "...", "expire_at": <unix_timestamp>} on stdout.
func (e *LLMTokenExecutor) GetToken(ctx context.Context, modelID, command, userEmail, userID string) (string, error) {
	cacheKey := modelID + ":" + userID

	e.mu.Lock()
	if cached, ok := e.cache[cacheKey]; ok && time.Now().Before(cached.expireAt) {
		token := cached.token
		e.mu.Unlock()
		return token, nil
	}
	e.mu.Unlock()

	token, expireAt, err := e.executeCommand(ctx, command, userEmail, userID)
	if err != nil {
		return "", err
	}

	e.mu.Lock()
	e.cache[cacheKey] = &cachedToken{
		token:    token,
		expireAt: expireAt,
	}
	e.mu.Unlock()

	return token, nil
}

func (e *LLMTokenExecutor) executeCommand(ctx context.Context, command, userEmail, userID string) (string, time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", time.Time{}, fmt.Errorf("empty token command")
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)

	inputJSON, err := json.Marshal(tokenCommandInput{
		Email:  userEmail,
		UserID: userID,
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("marshal stdin input: %w", err)
	}
	cmd.Stdin = bytes.NewReader(inputJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", time.Time{}, fmt.Errorf("token command failed: %w, stderr: %s", err, stderr.String())
	}

	var output tokenCommandOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return "", time.Time{}, fmt.Errorf("parse token command output: %w", err)
	}

	if output.Token == "" {
		return "", time.Time{}, fmt.Errorf("token command returned empty token")
	}

	expireAt := time.Unix(output.ExpireAt, 0)
	if output.ExpireAt == 0 {
		// Default to 5 minutes if no expiry specified
		expireAt = time.Now().Add(5 * time.Minute)
	}

	return output.Token, expireAt, nil
}
