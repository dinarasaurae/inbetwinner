package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/dinarasaurae/inbetwin-social-service/internal/models"
)

// VKProcessContext carries per-integration policy context sent to llm-service.
type VKProcessContext struct {
	ToneOfVoice       string   `json:"tone_of_voice,omitempty"`
	SafeIntents       []string `json:"safe_intents,omitempty"`
	ForbiddenPromises []string `json:"forbidden_promises,omitempty"`
	EscalationPolicy  string   `json:"escalation_policy,omitempty"`
	AutoReplyEnabled  bool     `json:"auto_reply_enabled"`
	BusinessSnapshot  string   `json:"business_snapshot,omitempty"`
}

// VKProcessRequest is the payload posted to /llm/social/vk/process.
type VKProcessRequest struct {
	IntegrationID string            `json:"integration_id"`
	ChatUserID    string            `json:"chat_user_id"`
	Platform      string            `json:"platform"`
	Message       string            `json:"message"`
	Context       *VKProcessContext `json:"context,omitempty"`
}

// VKLLMClient is a thin HTTP client for the llm-service VK orchestration endpoint.
type VKLLMClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewVKLLMClient returns a client pointing at baseURL (e.g. "http://llm-service:3005").
func NewVKLLMClient(baseURL string) *VKLLMClient {
	return &VKLLMClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 8 * time.Second},
	}
}

// ProcessVKMessage calls POST {baseURL}/llm/social/vk/process and returns
// the structured orchestration decision.
func (c *VKLLMClient) ProcessVKMessage(ctx context.Context, workspaceID uuid.UUID, req VKProcessRequest) (*models.VKOrchestrationDecision, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/llm/social/vk/process", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-User-ID", workspaceID.String())

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llm-service: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("llm-service HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var decision models.VKOrchestrationDecision
	if err := json.Unmarshal(respBody, &decision); err != nil {
		return nil, fmt.Errorf("llm-service decode: %w", err)
	}
	return &decision, nil
}
