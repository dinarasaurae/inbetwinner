package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ToolType string

const (
	ToolTypeBuiltin     ToolType = "builtin"
	ToolTypeHTTPWebhook ToolType = "http_webhook"
)

type Tool struct {
	ID              uuid.UUID       `json:"id"`
	WorkspaceID     uuid.UUID       `json:"workspace_id"`
	Name            string          `json:"name"`
	DisplayName     string          `json:"display_name"`
	Description     string          `json:"description"`
	Type            ToolType        `json:"type"`
	Schema          json.RawMessage `json:"schema"`
	IsEnabled       bool            `json:"is_enabled"`
	TimeoutSecs     int             `json:"timeout_seconds"`
	ExecutionConfig json.RawMessage `json:"execution_config"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type CreateToolRequest struct {
	Name            string          `json:"name"`
	DisplayName     string          `json:"display_name"`
	Description     string          `json:"description"`
	Type            ToolType        `json:"type"`
	Schema          json.RawMessage `json:"schema"`
	TimeoutSecs     int             `json:"timeout_seconds"`
	ExecutionConfig json.RawMessage `json:"execution_config"`
}
