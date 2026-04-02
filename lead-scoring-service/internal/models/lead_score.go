package models

import (
	"time"

	"github.com/google/uuid"
)

type LeadScore struct {
	ID           uuid.UUID `json:"id"`
	WorkspaceID  uuid.UUID `json:"workspace_id"`
	LeadID       string    `json:"lead_id"`
	Platform     string    `json:"platform"`
	Score        int       `json:"score"`
	ScoreLabel   string    `json:"score_label"`
	LastMessage  string    `json:"last_message"`
	MessageCount int       `json:"message_count"`
	UpdatedAt    time.Time `json:"updated_at"`
	CreatedAt    time.Time `json:"created_at"`
}

type ScoreRequest struct {
	LeadID      string    `json:"lead_id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	Platform    string    `json:"platform"`
	Message     string    `json:"message"`
}

type ScoreResponse struct {
	LeadID     string   `json:"lead_id"`
	Score      int      `json:"score"`
	ScoreLabel string   `json:"score_label"`
	Delta      int      `json:"delta"`
	IsHot      bool     `json:"is_hot"`
	Signals    []string `json:"signals"`
}
