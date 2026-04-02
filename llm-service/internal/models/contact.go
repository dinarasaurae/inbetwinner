package models

import (
	"time"

	"github.com/google/uuid"
)

type Contact struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	ChatUserID  string    `json:"chat_user_id"`
	Platform    string    `json:"platform"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	Phone       string    `json:"phone"`
	Company     string    `json:"company"`
	Notes       string    `json:"notes"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
