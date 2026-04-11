package models

import (
	"time"

	"github.com/google/uuid"
)

type KnowledgeTable struct {
	ID            uuid.UUID  `json:"id"`
	WorkspaceID   uuid.UUID  `json:"workspace_id"`
	NamespaceID   uuid.UUID  `json:"namespace_id"`
	Name          string     `json:"name"`
	SpreadsheetID string     `json:"spreadsheet_id"`
	SheetName     string     `json:"sheet_name"`
	RowCount      int        `json:"row_count"`
	Status        string     `json:"status"`
	LastSyncAt    *time.Time `json:"last_sync_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

type SheetsSyncRequest struct {
	SpreadsheetID    string    `json:"spreadsheet_id"`
	SheetName        string    `json:"sheet_name"`
	NamespaceID      uuid.UUID `json:"namespace_id"`
	APIKey           string    `json:"api_key,omitempty"`        // legacy: plain API key
	UseOAuth         bool      `json:"use_oauth,omitempty"`      // use stored Google OAuth token
	TableName        string    `json:"table_name"`
	EnableEmbedding  bool      `json:"enable_embedding,omitempty"` // embed rows into Pinecone (costs tokens)
}
