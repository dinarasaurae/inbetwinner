package models

import (
	"time"

	"github.com/google/uuid"
)

type Agent struct {
	ID                  uuid.UUID `json:"id"`
	WorkspaceID         uuid.UUID `json:"workspace_id"`
	Name                string    `json:"name"`
	Slug                string    `json:"slug"`
	DisplayName         string    `json:"display_name"`
	SystemPrompt        string    `json:"system_prompt"`
	Model               string    `json:"model"`
	Temperature         float64   `json:"temperature"`
	MaxTokens           int       `json:"max_tokens"`
	LanguageCode        string    `json:"language_code"`
	MemoryHistorySize   int       `json:"memory_history_size"`
	RagTopK             int       `json:"rag_top_k"`
	KnowledgeNamespaces []string  `json:"knowledge_namespaces"`
	UseSummary          bool      `json:"use_summary"`
	SummarizeEvery      int       `json:"summarize_every"`
	Status              string    `json:"status"`
	Platform            string    `json:"platform"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type CreateAgentRequest struct {
	Name                string   `json:"name"`
	DisplayName         string   `json:"display_name"`
	SystemPrompt        string   `json:"system_prompt"`
	Model               string   `json:"model"`
	Temperature         float64  `json:"temperature"`
	MaxTokens           int      `json:"max_tokens"`
	LanguageCode        string   `json:"language_code"`
	MemoryHistorySize   int      `json:"memory_history_size"`
	RagTopK             int      `json:"rag_top_k"`
	KnowledgeNamespaces []string `json:"knowledge_namespaces"`
	Platform            string   `json:"platform"`
}
