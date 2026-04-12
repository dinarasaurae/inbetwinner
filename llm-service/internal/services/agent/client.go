// Package agent provides an HTTP client for the agent-service.
// llm-service uses it to resolve agent configuration (system prompt,
// model params, RAG settings, allowed tool list) at message-processing time.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// AgentConfig is the subset of agent-service's Agent model that
// llm-service cares about at runtime.
type AgentConfig struct {
	ID                  uuid.UUID `json:"id"`
	WorkspaceID         uuid.UUID `json:"workspace_id"`
	Name                string    `json:"name"`
	SystemPrompt        string    `json:"system_prompt"`
	Model               string    `json:"model"`
	Temperature         float64   `json:"temperature"`
	MaxTokens           int       `json:"max_tokens"`
	LanguageCode        string    `json:"language_code"`
	MemoryHistorySize   int       `json:"memory_history_size"`
	RagTopK             int       `json:"rag_top_k"`
	KnowledgeNamespaces []string  `json:"knowledge_namespaces"`
	Status              string    `json:"status"`
	Platform            string    `json:"platform"`
}

// AgentTool mirrors agent-service's AgentTool row.
type AgentTool struct {
	ToolID    string `json:"tool_id"`
	ToolName  string `json:"tool_name"`
	IsEnabled bool   `json:"is_enabled"`
	Priority  int    `json:"priority"`
}

// Client is a thin HTTP client for agent-service endpoints.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a Client pointing at baseURL (e.g. "http://agent-service:3003").
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// GetDefaultAgent returns the best-matching active agent for a workspace.
// Preference order: exact platform match → "all" platform → any active → first in list.
// Returns nil (no error) when the workspace has no agents yet.
func (c *Client) GetDefaultAgent(ctx context.Context, workspaceID uuid.UUID, platform string) (*AgentConfig, error) {
	agents, err := c.listAgents(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	// exact platform match (active)
	for i := range agents {
		a := &agents[i]
		if a.Status == "active" && a.Platform == platform {
			return a, nil
		}
	}
	// "all" platform (active)
	for i := range agents {
		a := &agents[i]
		if a.Status == "active" && a.Platform == "all" {
			return a, nil
		}
	}
	// any active
	for i := range agents {
		a := &agents[i]
		if a.Status == "active" {
			return a, nil
		}
	}
	// fall back to first (might be inactive/draft)
	if len(agents) > 0 {
		return &agents[0], nil
	}
	return nil, nil
}

// ListActiveAgents returns all active agents for a workspace on the given platform.
// Used by the router agent to pick between multiple configured agents.
func (c *Client) ListActiveAgents(ctx context.Context, workspaceID uuid.UUID, platform string) ([]AgentConfig, error) {
	all, err := c.listAgents(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	var out []AgentConfig
	for _, a := range all {
		if a.Status != "active" {
			continue
		}
		if a.Platform == platform || a.Platform == "all" || platform == "" {
			out = append(out, a)
		}
	}
	return out, nil
}

// GetAgentByID fetches a specific agent. Returns nil if not found (404).
func (c *Client) GetAgentByID(ctx context.Context, workspaceID, agentID uuid.UUID) (*AgentConfig, error) {
	url := fmt.Sprintf("%s/agent/agents/%s", c.baseURL, agentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-User-ID", workspaceID.String())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("agent-service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
	var a AgentConfig
	if err := json.Unmarshal(body, &a); err != nil {
		return nil, fmt.Errorf("agent-service decode: %w", err)
	}
	return &a, nil
}

// GetAgentTools returns the enabled tool assignments for an agent.
// Returns an empty slice (no error) when the agent has no tools assigned.
func (c *Client) GetAgentTools(ctx context.Context, workspaceID, agentID uuid.UUID) ([]AgentTool, error) {
	url := fmt.Sprintf("%s/agent/agents/%s/tools", c.baseURL, agentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-User-ID", workspaceID.String())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("agent-service tools: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
	var ts []AgentTool
	if err := json.Unmarshal(body, &ts); err != nil {
		return nil, fmt.Errorf("agent-service tools decode: %w", err)
	}

	// filter to enabled only
	var out []AgentTool
	for _, t := range ts {
		if t.IsEnabled {
			out = append(out, t)
		}
	}
	return out, nil
}

// --- private helpers ---

func (c *Client) listAgents(ctx context.Context, workspaceID uuid.UUID) ([]AgentConfig, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/agent/agents", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-User-ID", workspaceID.String())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("agent-service list: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var agents []AgentConfig
	if err := json.Unmarshal(body, &agents); err != nil {
		return nil, fmt.Errorf("agent-service list decode: %w", err)
	}
	return agents, nil
}
