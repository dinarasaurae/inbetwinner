package models

import (
	"time"

	"github.com/google/uuid"
)

type WebSource struct {
	ID           uuid.UUID  `json:"id"`
	WorkspaceID  uuid.UUID  `json:"workspace_id"`
	NamespaceID  uuid.UUID  `json:"namespace_id"`
	URL          string     `json:"url"`
	Title        string     `json:"title"`
	Status       string     `json:"status"` // pending | indexing | indexed | error
	ErrorMessage string     `json:"error_message,omitempty"`
	ChunkCount   int        `json:"chunk_count"`
	LastCrawlAt  *time.Time `json:"last_crawl_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

type WebSourceCreateRequest struct {
	NamespaceID uuid.UUID `json:"namespace_id"`
	URL         string    `json:"url"`
}
