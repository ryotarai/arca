package notification

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ryotarai/arca/internal/db"
	"github.com/ryotarai/arca/internal/machine"
)

// SlackService sends Slack notifications for machine lifecycle events.
type SlackService struct {
	store      *db.Store
	httpClient *http.Client
}

// NewSlackService creates a new SlackService.
func NewSlackService(store *db.Store) *SlackService {
	return &SlackService{
		store: store,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// NotifyMachineEvent sends a Slack notification for a machine event to the machine owner.
// It implements machine.Notifier.
func (s *SlackService) NotifyMachineEvent(ctx context.Context, ownerUserID string, event machine.NotificationEvent) {
	config, err := s.loadConfig(ctx)
	if err != nil {
		slog.Warn("slack: failed to load config", "error", err)
		return
	}
	if !config.Enabled || config.BotToken == "" {
		return
	}

	settings, err := s.store.GetUserNotificationSettings(ctx, ownerUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return
		}
		slog.Warn("slack: failed to load user notification settings", "user_id", ownerUserID, "error", err)
		return
	}
	if !settings.SlackEnabled || settings.SlackUserID == "" {
		return
	}

	channel := settings.SlackUserID
	if err := s.sendMessage(ctx, config.BotToken, channel, event); err != nil {
		slog.Warn("slack: failed to send notification", "user_id", ownerUserID, "channel", channel, "error", err)
	}
}

// SendTestMessage sends a test notification to the given channel.
func (s *SlackService) SendTestMessage(ctx context.Context, channelID string) error {
	config, err := s.loadConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load slack config: %w", err)
	}
	if config.BotToken == "" {
		return errors.New("slack bot token is not configured")
	}

	testEvent := machine.NotificationEvent{
		MachineName: "test-machine",
		EventType:   "test",
		Message:     "This is a test notification from Arca.",
	}
	return s.sendMessage(ctx, config.BotToken, channelID, testEvent)
}

func (s *SlackService) loadConfig(ctx context.Context) (db.SlackConfig, error) {
	return s.store.GetSlackConfig(ctx)
}

func (s *SlackService) sendMessage(ctx context.Context, token, channel string, event machine.NotificationEvent) error {
	blocks := buildMessageBlocks(event)

	payload := map[string]interface{}{
		"channel": channel,
		"text":    fmt.Sprintf("[%s] %s – %s", event.EventType, event.MachineName, event.Message),
		"blocks":  blocks,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var slackResp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &slackResp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if !slackResp.OK {
		return fmt.Errorf("slack API error: %s", slackResp.Error)
	}

	return nil
}

func buildMessageBlocks(event machine.NotificationEvent) []map[string]interface{} {
	emoji := eventEmoji(event.EventType)
	title := fmt.Sprintf("%s *%s* – %s", emoji, event.MachineName, formatEventType(event.EventType))

	blocks := []map[string]interface{}{
		{
			"type": "section",
			"text": map[string]interface{}{
				"type": "mrkdwn",
				"text": title,
			},
		},
	}

	if event.Message != "" {
		blocks = append(blocks, map[string]interface{}{
			"type": "context",
			"elements": []map[string]interface{}{
				{
					"type": "mrkdwn",
					"text": event.Message,
				},
			},
		})
	}

	return blocks
}

func formatEventType(eventType string) string {
	switch eventType {
	case "ready":
		return "Machine Ready"
	case "auto_stop":
		return "Auto-Stopped"
	case "job_failed":
		return "Job Failed"
	case "start_requested":
		return "Start Requested"
	case "stop_requested":
		return "Stop Requested"
	case "delete_requested":
		return "Delete Requested"
	case "test":
		return "Test Notification"
	default:
		return strings.ReplaceAll(eventType, "_", " ")
	}
}

func eventEmoji(eventType string) string {
	switch eventType {
	case "ready":
		return ":white_check_mark:"
	case "auto_stop":
		return ":zzz:"
	case "job_failed":
		return ":x:"
	case "start_requested":
		return ":arrow_forward:"
	case "stop_requested":
		return ":stop_button:"
	case "delete_requested":
		return ":wastebasket:"
	case "test":
		return ":bell:"
	default:
		return ":bell:"
	}
}
