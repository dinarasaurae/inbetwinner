package models

import (
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleTool      Role = "tool"
)

type ChatMessage struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	ChatUserID  string    `json:"chat_user_id"`
	Platform    string    `json:"platform"`
	Role        Role      `json:"role"`
	Content     string    `json:"content"`
	ToolName    string    `json:"tool_name,omitempty"`
	ToolCallID  string    `json:"tool_call_id,omitempty"`
	TotalTokens int       `json:"total_tokens"`
	CreatedAt   time.Time `json:"created_at"`
}
