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

// PushNotificationClient calls auth-service's internal push endpoint.
// It lives in social-service so that vk_agent can trigger pushes
// when a new message arrives without knowing Firebase details.
type PushNotificationClient struct {
	authServiceURL string
	httpClient     *http.Client
}

func NewPushNotificationClient(authServiceURL string) *PushNotificationClient {
	return &PushNotificationClient{
		authServiceURL: authServiceURL,
		httpClient:     &http.Client{Timeout: 5 * time.Second},
	}
}

type InternalPushRequest struct {
	UserID   string            `json:"user_id"`
	Type     string            `json:"type"`
	Title    string            `json:"title"`
	Body     string            `json:"body"`
	Data     map[string]string `json:"data,omitempty"`
	DeepLink string            `json:"deep_link,omitempty"`
}

// SendToUser sends a push notification to all devices of a workspace owner.
// Non-fatal — logs on error so the main message flow is never blocked.
func (c *PushNotificationClient) SendToUser(ctx context.Context, userID uuid.UUID, req InternalPushRequest) {
	if c.authServiceURL == "" {
		return
	}
	req.UserID = userID.String()

	raw, err := json.Marshal(req)
	if err != nil {
		log.Printf("[push] marshal: %v", err)
		return
	}

	url := fmt.Sprintf("%s/internal/notifications/push", c.authServiceURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		log.Printf("[push] build request: %v", err)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		log.Printf("[push] send: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("[push] auth-service returned %d", resp.StatusCode)
	}
}
