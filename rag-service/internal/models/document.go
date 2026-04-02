package models

import (
	"time"

	"github.com/google/uuid"
)

type KnowledgeDocument struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	NamespaceID uuid.UUID `json:"namespace_id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	Content     string    `json:"content,omitempty"`
	ChunkSize   int       `json:"chunk_size"`
	ChunkCount  int       `json:"chunk_count"`
	Status      string    `json:"status"`
	EmbedModel  string    `json:"embed_model"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type KnowledgeChunk struct {
	ID          uuid.UUID `json:"id"`
	DocumentID  uuid.UUID `json:"document_id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	ChunkIndex  int       `json:"chunk_index"`
	Text        string    `json:"text"`
	TokenCount  int       `json:"token_count"`
	PineconeID  string    `json:"pinecone_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type CreateDocumentRequest struct {
	NamespaceID uuid.UUID `json:"namespace_id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	Content     string    `json:"content"`
	ChunkSize   int       `json:"chunk_size"`
}
