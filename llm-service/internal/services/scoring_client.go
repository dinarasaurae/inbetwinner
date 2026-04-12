package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// ScoringClient calls lead-scoring-service to score an inbound message.
type ScoringClient struct {
	url        string
	httpClient *http.Client
}

func NewScoringClient(url string) *ScoringClient {
	return &ScoringClient{
		url:        url,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

type scoringRequest struct {
	WorkspaceID string `json:"workspace_id"`
	ChatUserID  string `json:"chat_user_id"`
	Platform    string `json:"platform"`
	Message     string `json:"message"`
}

// ScoreResult is the parsed response from the scoring service.
type ScoreResult struct {
	Score  int    `json:"score"`
	IsHot  bool   `json:"is_hot"`
	UserID string `json:"user_id,omitempty"`
}

// ScoreMessage synchronously sends a scoring request and returns the result.
func (c *ScoringClient) ScoreMessage(ctx context.Context, workspaceID uuid.UUID, chatUserID, platform, message string) (*ScoreResult, error) {
	if c == nil || c.url == "" {
		return nil, nil
	}
	payload, _ := json.Marshal(scoringRequest{
		WorkspaceID: workspaceID.String(),
		ChatUserID:  chatUserID,
		Platform:    platform,
		Message:     message,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.url+"/leads/scoring/score", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("scoring service returned %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var result ScoreResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ScoreAsync scores a message in the background and calls onHot when score is above threshold.
// Safe to call with a nil receiver.
func (c *ScoringClient) ScoreAsync(workspaceID uuid.UUID, chatUserID, platform, message string, onHot func(*ScoreResult)) {
	if c == nil || c.url == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		score, err := c.ScoreMessage(ctx, workspaceID, chatUserID, platform, message)
		if err != nil {
			log.Printf("[scoring] async score error: %v", err)
			return
		}
		if score != nil && score.IsHot && onHot != nil {
			onHot(score)
		}
	}()
}
