package models

import (
	"time"

	"github.com/google/uuid"
)

type QAPair struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	NamespaceID uuid.UUID `json:"namespace_id"`
	Question    string    `json:"question"`
	Answer      string    `json:"answer"`
	Tags        []string  `json:"tags"`
	IsStrict    bool      `json:"is_strict"`
	PineconeID  string    `json:"pinecone_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateQARequest struct {
	NamespaceID uuid.UUID `json:"namespace_id"`
	Question    string    `json:"question"`
	Answer      string    `json:"answer"`
	Tags        []string  `json:"tags"`
	IsStrict    bool      `json:"is_strict"`
}
