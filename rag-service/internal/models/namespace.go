package models

import (
	"time"

	"github.com/google/uuid"
)

type Namespace struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`        // "qa","doc","web","table"
	Scope       string    `json:"scope"`       // "workspace","global"
	PineconeNS  string    `json:"pinecone_ns"`
	Description string    `json:"description"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateNamespaceRequest struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Scope       string `json:"scope"`
	Description string `json:"description"`
}
