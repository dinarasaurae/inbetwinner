package models

import (
	"time"

	"github.com/google/uuid"
)

type AmoCRMIntegration struct {
	ID           uuid.UUID `json:"id"`
	WorkspaceID  uuid.UUID `json:"workspace_id"`
	Subdomain    string    `json:"subdomain"`
	AccountID    int64     `json:"account_id,omitempty"`
	Email        string    `json:"email,omitempty"`
	IsActive     bool      `json:"is_active"`
	TokenExpiry  time.Time `json:"token_expiry"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
