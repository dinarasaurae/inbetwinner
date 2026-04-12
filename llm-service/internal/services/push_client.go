package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// LLMPushClient calls auth-service internal endpoint to send a push notification
// to the workspace owner (business user) about important events.
type LLMPushClient struct {
	url        string
	httpClient *http.Client
}

func NewLLMPushClient(url string) *LLMPushClient {
	return &LLMPushClient{
		url:        url,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// NotificationType constants matching the mobile app.
const (
	NotifHotLead    = "HOT_LEAD"
	NotifAgentStuck = "AGENT_STUCK"
)

type pushPayload struct {
	UserID   string            `json:"user_id"`
	Type     string            `json:"type"`
	Title    string            `json:"title"`
	Body     string            `json:"body"`
	Data     map[string]string `json:"data,omitempty"`
	DeepLink string            `json:"deep_link,omitempty"`
}

// SendToWorkspaceOwner sends a push notification to the workspace owner (identified by workspaceID = ownerUserID).
func (c *LLMPushClient) SendToWorkspaceOwner(ctx context.Context, workspaceOwnerID uuid.UUID, notifType, title, body string, data map[string]string) error {
	if c == nil || c.url == "" {
		return nil
	}
	payload, _ := json.Marshal(pushPayload{
		UserID: workspaceOwnerID.String(),
		Type:   notifType,
		Title:  title,
		Body:   body,
		Data:   data,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.url+"/internal/notifications/push", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("push service returned %d", resp.StatusCode)
	}
	return nil
}

// SendAsync sends a push notification in a background goroutine. Non-fatal.
func (c *LLMPushClient) SendAsync(workspaceOwnerID uuid.UUID, notifType, title, body string, data map[string]string) {
	if c == nil || c.url == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.SendToWorkspaceOwner(ctx, workspaceOwnerID, notifType, title, body, data); err != nil {
			log.Printf("[push] async send error: %v", err)
		}
	}()
}
