package services

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/dinarasaurae/inbetwin-agent-service/internal/database"
	"github.com/dinarasaurae/inbetwin-agent-service/internal/models"
	"github.com/google/uuid"
)

type AgentService struct{ db *database.DB }

func NewAgentService(db *database.DB) *AgentService { return &AgentService{db: db} }

var slugRe = regexp.MustCompile(`[^a-z0-9-]`)

func generateSlug(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "-")
	s = slugRe.ReplaceAllString(s, "")
	return strings.Trim(s, "-")
}

func applyDefaults(req *models.CreateAgentRequest) {
	if req.Model == "" {
		req.Model = "gpt-4o-mini"
	}
	if req.Temperature == 0 {
		req.Temperature = 0.7
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 2000
	}
	if req.LanguageCode == "" {
		req.LanguageCode = "ru"
	}
	if req.MemoryHistorySize == 0 {
		req.MemoryHistorySize = 20
	}
	if req.RagTopK == 0 {
		req.RagTopK = 5
	}
	if req.Platform == "" {
		req.Platform = "all"
	}
	if req.KnowledgeNamespaces == nil {
		req.KnowledgeNamespaces = []string{}
	}
}

func (s *AgentService) Create(ctx context.Context, workspaceID uuid.UUID, req models.CreateAgentRequest) (*models.Agent, error) {
	applyDefaults(&req)
	slug := generateSlug(req.Name)
	nsJSON, _ := json.Marshal(req.KnowledgeNamespaces)

	a := &models.Agent{}
	var nsBytes []byte
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO agents
			(workspace_id, name, slug, display_name, system_prompt, model, temperature,
			 max_tokens, language_code, memory_history_size, rag_top_k,
			 knowledge_namespaces, platform)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id, workspace_id, name, slug, display_name, system_prompt, model,
		          temperature, max_tokens, language_code, memory_history_size, rag_top_k,
		          knowledge_namespaces, use_summary, summarize_every, status, platform,
		          created_at, updated_at`,
		workspaceID, req.Name, slug, req.DisplayName, req.SystemPrompt, req.Model, req.Temperature,
		req.MaxTokens, req.LanguageCode, req.MemoryHistorySize, req.RagTopK, nsJSON, req.Platform,
	).Scan(&a.ID, &a.WorkspaceID, &a.Name, &a.Slug, &a.DisplayName, &a.SystemPrompt,
		&a.Model, &a.Temperature, &a.MaxTokens, &a.LanguageCode, &a.MemoryHistorySize,
		&a.RagTopK, &nsBytes, &a.UseSummary, &a.SummarizeEvery, &a.Status, &a.Platform,
		&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert agent: %w", err)
	}
	_ = json.Unmarshal(nsBytes, &a.KnowledgeNamespaces)
	return a, nil
}

func (s *AgentService) GetByID(ctx context.Context, id, workspaceID uuid.UUID) (*models.Agent, error) {
	a := &models.Agent{}
	var nsBytes []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, name, slug, display_name, system_prompt, model,
		       temperature, max_tokens, language_code, memory_history_size, rag_top_k,
		       knowledge_namespaces, use_summary, summarize_every, status, platform,
		       created_at, updated_at
		FROM agents WHERE id=$1 AND workspace_id=$2`, id, workspaceID,
	).Scan(&a.ID, &a.WorkspaceID, &a.Name, &a.Slug, &a.DisplayName, &a.SystemPrompt,
		&a.Model, &a.Temperature, &a.MaxTokens, &a.LanguageCode, &a.MemoryHistorySize,
		&a.RagTopK, &nsBytes, &a.UseSummary, &a.SummarizeEvery, &a.Status, &a.Platform,
		&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(nsBytes, &a.KnowledgeNamespaces)
	return a, nil
}

func (s *AgentService) List(ctx context.Context, workspaceID uuid.UUID) ([]models.Agent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workspace_id, name, slug, display_name, system_prompt, model,
		       temperature, max_tokens, language_code, memory_history_size, rag_top_k,
		       knowledge_namespaces, use_summary, summarize_every, status, platform,
		       created_at, updated_at
		FROM agents WHERE workspace_id=$1 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Agent
	for rows.Next() {
		var a models.Agent
		var nsBytes []byte
		if err := rows.Scan(&a.ID, &a.WorkspaceID, &a.Name, &a.Slug, &a.DisplayName, &a.SystemPrompt,
			&a.Model, &a.Temperature, &a.MaxTokens, &a.LanguageCode, &a.MemoryHistorySize,
			&a.RagTopK, &nsBytes, &a.UseSummary, &a.SummarizeEvery, &a.Status, &a.Platform,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(nsBytes, &a.KnowledgeNamespaces)
		out = append(out, a)
	}
	return out, nil
}

func (s *AgentService) Update(ctx context.Context, id, workspaceID uuid.UUID, req models.CreateAgentRequest) (*models.Agent, error) {
	applyDefaults(&req)
	nsJSON, _ := json.Marshal(req.KnowledgeNamespaces)
	a := &models.Agent{}
	var nsBytes []byte
	err := s.db.QueryRowContext(ctx, `
		UPDATE agents SET
			name=$1, display_name=$2, system_prompt=$3, model=$4, temperature=$5,
			max_tokens=$6, language_code=$7, memory_history_size=$8, rag_top_k=$9,
			knowledge_namespaces=$10, platform=$11, updated_at=NOW()
		WHERE id=$12 AND workspace_id=$13
		RETURNING id, workspace_id, name, slug, display_name, system_prompt, model,
		          temperature, max_tokens, language_code, memory_history_size, rag_top_k,
		          knowledge_namespaces, use_summary, summarize_every, status, platform,
		          created_at, updated_at`,
		req.Name, req.DisplayName, req.SystemPrompt, req.Model, req.Temperature,
		req.MaxTokens, req.LanguageCode, req.MemoryHistorySize, req.RagTopK,
		nsJSON, req.Platform, id, workspaceID,
	).Scan(&a.ID, &a.WorkspaceID, &a.Name, &a.Slug, &a.DisplayName, &a.SystemPrompt,
		&a.Model, &a.Temperature, &a.MaxTokens, &a.LanguageCode, &a.MemoryHistorySize,
		&a.RagTopK, &nsBytes, &a.UseSummary, &a.SummarizeEvery, &a.Status, &a.Platform,
		&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("update: %w", err)
	}
	_ = json.Unmarshal(nsBytes, &a.KnowledgeNamespaces)
	return a, nil
}

func (s *AgentService) Delete(ctx context.Context, id, workspaceID uuid.UUID) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM agents WHERE id=$1 AND workspace_id=$2`, id, workspaceID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("agent not found")
	}
	return nil
}

func (s *AgentService) AssignTool(ctx context.Context, agentID uuid.UUID, toolID string, priority int) error {
	// Derive tool name from ID
	toolName := toolID
	if len(toolID) > 8 && toolID[:8] == "builtin-" {
		toolName = toolID[8:]
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_tools (agent_id, tool_id, tool_name, priority)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (agent_id, tool_id) DO UPDATE SET priority=$4, is_enabled=true`,
		agentID, toolID, toolName, priority)
	return err
}

func (s *AgentService) UnassignTool(ctx context.Context, agentID uuid.UUID, toolID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM agent_tools WHERE agent_id=$1 AND tool_id=$2`, agentID, toolID)
	return err
}

func (s *AgentService) GetTools(ctx context.Context, agentID uuid.UUID) ([]models.AgentTool, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT agent_id, tool_id, tool_name, priority, is_enabled, created_at
		FROM agent_tools WHERE agent_id=$1 ORDER BY priority, tool_name`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.AgentTool
	for rows.Next() {
		var t models.AgentTool
		if err := rows.Scan(&t.AgentID, &t.ToolID, &t.ToolName, &t.Priority, &t.IsEnabled, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}
