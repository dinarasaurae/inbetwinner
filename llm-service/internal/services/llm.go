package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	openai "github.com/sashabaranov/go-openai"

	"github.com/dinarasaurae/inbetwin-llm-service/internal/config"
	"github.com/dinarasaurae/inbetwin-llm-service/internal/models"
	agentsvc "github.com/dinarasaurae/inbetwin-llm-service/internal/services/agent"
	histsvc "github.com/dinarasaurae/inbetwin-llm-service/internal/services/history"
	"github.com/dinarasaurae/inbetwin-llm-service/internal/services/tools"
)

// ChatRequest carries the incoming message and optional routing hints.
type ChatRequest struct {
	// AgentID, if set, pins the specific agent to use. Otherwise the
	// best-matching active agent for the workspace+platform is resolved.
	AgentID     *uuid.UUID `json:"agent_id,omitempty"`
	ChatUserID  string     `json:"chat_user_id"`
	Platform    string     `json:"platform"`
	Message     string     `json:"message"`
	WorkspaceID uuid.UUID  `json:"-"`
}

type ChatResponse struct {
	AgentID     *uuid.UUID `json:"agent_id,omitempty"`
	Response    string     `json:"response"`
	ToolsUsed   []string   `json:"tools_used,omitempty"`
	TotalTokens int        `json:"total_tokens"`
}

// runtimeParams are the per-request LLM parameters resolved from the agent config.
type runtimeParams struct {
	model       string
	temperature float32
	maxTokens   int
	historySize int
	ragTopK     int
	namespaces  []string
	systemPmt   string
}

// fallback defaults when no agent is configured.
var defaultParams = runtimeParams{
	model:       "gpt-4o-mini",
	temperature: 0.7,
	maxTokens:   2000,
	historySize: 20,
	ragTopK:     3,
	namespaces:  nil,
	systemPmt: `Ты умный AI-ассистент для B2B продаж. Отвечай на языке пользователя (по умолчанию русский).
Помогай квалифицировать лидов, отвечать на вопросы о продукте, назначать встречи и сохранять контактные данные.
Будь дружелюбным, профессиональным и конкретным.`,
}

// LLMService orchestrates the agentic tool-calling loop.
type LLMService struct {
	client      *openai.Client
	history     *histsvc.Service
	registry    *tools.Registry
	dispatcher  *tools.Dispatcher
	agentClient *agentsvc.Client
	cfg         *config.Config
	httpClient  *http.Client
}

func NewLLMService(
	cfg *config.Config,
	hist *histsvc.Service,
	reg *tools.Registry,
	disp *tools.Dispatcher,
	agentClient *agentsvc.Client,
) *LLMService {
	return &LLMService{
		client:      openai.NewClient(cfg.OpenAIAPIKey),
		history:     hist,
		registry:    reg,
		dispatcher:  disp,
		agentClient: agentClient,
		cfg:         cfg,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

// ProcessMessage runs the full agentic loop for a single user turn.
func (s *LLMService) ProcessMessage(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	// 1. Resolve agent config from agent-service (best-effort — degrade gracefully).
	params, agentID, allowedToolIDs := s.resolveAgentConfig(ctx, req)

	// 2. Pull chat history.
	histMsgs, _ := s.history.GetHistory(ctx, req.WorkspaceID, req.ChatUserID, params.historySize)

	// 3. Fetch RAG context scoped to this agent's knowledge namespaces.
	ragContext := s.fetchRAGContext(ctx, req.WorkspaceID, req.Message, params.ragTopK, params.namespaces)

	// 4. Resolve tool list (filtered by agent's allowlist).
	hasCalendar := s.registry.HasGoogleCalendar(ctx, req.WorkspaceID)
	availableTools, _ := s.registry.GetTools(ctx, req.WorkspaceID, hasCalendar, allowedToolIDs)
	openAITools := tools.ToOpenAITools(availableTools)

	// 5. Build the initial message array.
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: s.buildSystemPrompt(params.systemPmt, ragContext)},
	}
	messages = append(messages, s.history.ToOpenAIMessages(histMsgs)...)
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: req.Message,
	})

	_ = s.history.AppendMessage(ctx, models.ChatMessage{
		WorkspaceID: req.WorkspaceID,
		ChatUserID:  req.ChatUserID,
		Platform:    req.Platform,
		Role:        models.RoleUser,
		Content:     req.Message,
	})

	// 6. Agentic tool-calling loop.
	var usedTools []string
	totalTokens := 0
	maxIter := s.cfg.MaxToolIterations
	if maxIter <= 0 {
		maxIter = 5
	}

	for i := 0; i < maxIter; i++ {
		compReq := openai.ChatCompletionRequest{
			Model:       params.model,
			Messages:    messages,
			Temperature: params.temperature,
			MaxTokens:   params.maxTokens,
		}
		if len(openAITools) > 0 {
			compReq.Tools = openAITools
		}

		resp, err := s.client.CreateChatCompletion(ctx, compReq)
		if err != nil {
			return nil, fmt.Errorf("openai: %w", err)
		}
		totalTokens += resp.Usage.TotalTokens

		if len(resp.Choices) == 0 {
			break
		}
		choice := resp.Choices[0]
		messages = append(messages, choice.Message)

		if choice.FinishReason == openai.FinishReasonToolCalls {
			for _, tc := range choice.Message.ToolCalls {
				result, err := s.dispatcher.Execute(ctx,
					req.WorkspaceID, req.ChatUserID, req.Platform,
					tc.Function.Name, json.RawMessage(tc.Function.Arguments))
				toolResult := result
				if err != nil {
					toolResult = fmt.Sprintf(`{"error":"%s"}`, err.Error())
				}
				messages = append(messages, s.dispatcher.ToToolMessage(tc, toolResult))
				usedTools = append(usedTools, tc.Function.Name)
			}
			continue
		}

		answer := choice.Message.Content
		_ = s.history.AppendMessage(ctx, models.ChatMessage{
			WorkspaceID: req.WorkspaceID,
			ChatUserID:  req.ChatUserID,
			Platform:    req.Platform,
			Role:        models.RoleAssistant,
			Content:     answer,
			TotalTokens: totalTokens,
		})
		return &ChatResponse{
			AgentID:     agentID,
			Response:    answer,
			ToolsUsed:   usedTools,
			TotalTokens: totalTokens,
		}, nil
	}

	return &ChatResponse{
		AgentID:     agentID,
		Response:    "Превышено максимальное количество шагов обработки.",
		ToolsUsed:   usedTools,
		TotalTokens: totalTokens,
	}, nil
}

// --- agent config resolution ---

// resolveAgentConfig fetches agent params + tool allowlist from agent-service.
// On any error or missing agent it returns defaults (safe degradation).
func (s *LLMService) resolveAgentConfig(ctx context.Context, req ChatRequest) (runtimeParams, *uuid.UUID, map[string]bool) {
	if s.agentClient == nil {
		return defaultParams, nil, nil
	}

	resolveCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var agentCfg *agentsvc.AgentConfig
	var err error

	if req.AgentID != nil {
		agentCfg, err = s.agentClient.GetAgentByID(resolveCtx, req.WorkspaceID, *req.AgentID)
	} else {
		agentCfg, err = s.agentClient.GetDefaultAgent(resolveCtx, req.WorkspaceID, req.Platform)
	}
	if err != nil {
		log.Printf("agent-service resolve: %v (using defaults)", err)
		return defaultParams, nil, nil
	}
	if agentCfg == nil {
		return defaultParams, nil, nil // no agents configured yet
	}

	params := runtimeParams{
		model:       coalesceStr(agentCfg.Model, defaultParams.model),
		temperature: float32(agentCfg.Temperature),
		maxTokens:   coalesceInt(agentCfg.MaxTokens, defaultParams.maxTokens),
		historySize: coalesceInt(agentCfg.MemoryHistorySize, defaultParams.historySize),
		ragTopK:     coalesceInt(agentCfg.RagTopK, defaultParams.ragTopK),
		namespaces:  agentCfg.KnowledgeNamespaces,
		systemPmt:   coalesceStr(agentCfg.SystemPrompt, defaultParams.systemPmt),
	}
	if params.temperature == 0 {
		params.temperature = defaultParams.temperature
	}

	agentID := agentCfg.ID
	agentToolRows, err := s.agentClient.GetAgentTools(resolveCtx, req.WorkspaceID, agentID)
	if err != nil {
		log.Printf("agent-service tools: %v (no tool filter)", err)
		return params, &agentID, nil
	}

	// Build allowlist. If the agent has no tools assigned at all, return nil
	// (Registry will expose builtins only — not all workspace tools).
	if len(agentToolRows) == 0 {
		// no explicit assignments → expose core builtins only
		return params, &agentID, map[string]bool{
			"builtin-save-contact": true,
		}
	}
	allowed := make(map[string]bool, len(agentToolRows))
	for _, t := range agentToolRows {
		allowed[t.ToolID] = true
	}
	return params, &agentID, allowed
}

// --- prompt + RAG ---

func (s *LLMService) buildSystemPrompt(base, ragContext string) string {
	if ragContext == "" {
		return base
	}
	return base + "\n\nКонтекст из базы знаний:\n" + ragContext
}

// fetchRAGContext calls rag-service/rag/search, scoped to the agent's namespaces.
func (s *LLMService) fetchRAGContext(ctx context.Context, workspaceID uuid.UUID, query string, topK int, namespaces []string) string {
	if s.cfg.RAGServiceURL == "" {
		return ""
	}
	if topK <= 0 {
		topK = 3
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"query":        query,
		"workspace_id": workspaceID.String(),
		"top_k":        topK,
		"namespaces":   namespaces,
	})
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		s.cfg.RAGServiceURL+"/rag/search", bytes.NewReader(payload))
	if err != nil {
		return ""
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-User-ID", workspaceID.String())
	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var result struct {
		Results []struct {
			Text string `json:"text"`
		} `json:"results"`
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
	if err := json.Unmarshal(body, &result); err != nil {
		return ""
	}
	var parts []string
	for _, r := range result.Results {
		if r.Text != "" {
			parts = append(parts, r.Text)
		}
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// --- small helpers ---

func coalesceStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func coalesceInt(a, b int) int {
	if a > 0 {
		return a
	}
	return b
}
