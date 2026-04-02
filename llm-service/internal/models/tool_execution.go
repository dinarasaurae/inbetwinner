package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ExecutionStatus string

const (
	ExecPending ExecutionStatus = "pending"
	ExecRunning ExecutionStatus = "running"
	ExecSuccess ExecutionStatus = "success"
	ExecFailed  ExecutionStatus = "failed"
)

type ToolExecution struct {
	ID          uuid.UUID       `json:"id"`
	ToolName    string          `json:"tool_name"`
	WorkspaceID uuid.UUID       `json:"workspace_id"`
	ChatUserID  string          `json:"chat_user_id"`
	Arguments   json.RawMessage `json:"arguments"`
	Status      ExecutionStatus `json:"status"`
	Result      json.RawMessage `json:"result,omitempty"`
	ErrorMsg    string          `json:"error_message,omitempty"`
	StartedAt   time.Time       `json:"started_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	DurationMs  int             `json:"duration_ms"`
}
