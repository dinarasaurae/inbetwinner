package models

import (
	"time"

	"github.com/google/uuid"
)

// ZohoIntegration holds the per-workspace Zoho CRM OAuth credentials.
type ZohoIntegration struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	// api_domain returned by Zoho's token endpoint (datacenter-specific).
	// Default: https://www.zohoapis.com (US). EU: https://www.zohoapis.eu etc.
	APIDomain    string    `json:"api_domain"`
	OrgID        string    `json:"org_id"`
	OrgName      string    `json:"org_name"`
	IsActive     bool      `json:"is_active"`
	TokenExpiry  time.Time `json:"token_expiry"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
