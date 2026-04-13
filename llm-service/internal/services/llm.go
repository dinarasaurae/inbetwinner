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
	"github.com/dinarasaurae/inbetwin-llm-service/internal/services/llmprovider"
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
var staticDefaultParams = runtimeParams{
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

func defaultRuntimeParams(cfg *config.Config) runtimeParams {
	params := staticDefaultParams
	if cfg == nil {
		return params
	}
	if cfg.LLMModel != "" {
		params.model = cfg.LLMModel
	}
	if cfg.LLMTemp != 0 {
		params.temperature = cfg.LLMTemp
	}
	if cfg.LLMMaxTokens > 0 {
		params.maxTokens = cfg.LLMMaxTokens
	}
	if cfg.HistorySize > 0 {
		params.historySize = cfg.HistorySize
	}
	return params
}

// LLMService orchestrates the agentic tool-calling loop.
type LLMService struct {
	provider    llmprovider.ChatCompletionProvider
	history     *histsvc.Service
	registry    *tools.Registry
	dispatcher  *tools.Dispatcher
	agentClient *agentsvc.Client
	scoring     *ScoringClient
	push        *LLMPushClient
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
		provider:    llmprovider.NewChatCompletionProvider(cfg),
		history:     hist,
		registry:    reg,
		dispatcher:  disp,
		agentClient: agentClient,
		scoring:     NewScoringClient(cfg.LeadScoringServiceURL),
		push:        NewLLMPushClient(cfg.AuthServiceURL),
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
		{Role: openai.ChatMessageRoleSystem, Content: s.buildSystemPromptWithMemory(params.systemPmt, ragContext, req.Platform, req.ChatUserID)},
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

		resp, err := s.provider.CreateChatCompletion(ctx, compReq)
		if err != nil {
			return nil, fmt.Errorf("%s provider: %w", s.provider.Name(), err)
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
			// If call_operator was triggered, send AGENT_STUCK push and stop the loop.
			if containsTool(usedTools, "call_operator") {
				s.push.SendAsync(req.WorkspaceID, NotifAgentStuck,
					"⚠️ Требуется ваше внимание",
					fmt.Sprintf("Агент не смог ответить пользователю %s (%s)", req.ChatUserID, req.Platform),
					map[string]string{
						"chat_user_id": req.ChatUserID,
						"platform":     req.Platform,
					},
				)
				// continue the loop so the LLM can say "operator is coming"
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

		// Async lead scoring — fire and forget; triggers HOT_LEAD push if score > threshold.
		s.scoring.ScoreAsync(req.WorkspaceID, req.ChatUserID, req.Platform, req.Message, func(score *ScoreResult) {
			s.push.SendAsync(req.WorkspaceID, NotifHotLead,
				"🔥 Горячий лид!",
				fmt.Sprintf("Пользователь %s (%s) набрал %d баллов", req.ChatUserID, req.Platform, score.Score),
				map[string]string{
					"chat_user_id": req.ChatUserID,
					"platform":     req.Platform,
					"score":        fmt.Sprintf("%d", score.Score),
				},
			)
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
// When multiple active agents exist for the workspace+platform, a cheap router
// LLM call is made to pick the best one.
func (s *LLMService) resolveAgentConfig(ctx context.Context, req ChatRequest) (runtimeParams, *uuid.UUID, map[string]bool) {
	defaultParams := defaultRuntimeParams(s.cfg)
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
		// Try router: if multiple active agents, pick the best one via LLM.
		agentCfg, err = s.routeAgent(resolveCtx, req)
		if err != nil || agentCfg == nil {
			agentCfg, err = s.agentClient.GetDefaultAgent(resolveCtx, req.WorkspaceID, req.Platform)
		}
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
			"builtin-save-contact":  true,
			"builtin-call-operator": true,
		}
	}
	allowed := make(map[string]bool, len(agentToolRows))
	for _, t := range agentToolRows {
		allowed[t.ToolID] = true
	}
	return params, &agentID, allowed
}

// routeAgent picks the best agent when a workspace has more than one active agent
// for the given platform. It uses a cheap model (gpt-4o-mini, temp=0) to
// classify the incoming message and select the most relevant agent by name.
// Returns nil (no error) when there is only one candidate — caller falls back to GetDefaultAgent.
func (s *LLMService) routeAgent(ctx context.Context, req ChatRequest) (*agentsvc.AgentConfig, error) {
	if s.agentClient == nil {
		return nil, nil
	}
	agents, err := s.agentClient.ListActiveAgents(ctx, req.WorkspaceID, req.Platform)
	if err != nil || len(agents) <= 1 {
		return nil, nil // single agent or error → let caller use GetDefaultAgent
	}

	// Build a compact description list for the router prompt.
	var descLines []string
	for i, a := range agents {
		desc := a.Name
		if a.SystemPrompt != "" && len(a.SystemPrompt) > 120 {
			desc += ": " + a.SystemPrompt[:120] + "…"
		} else if a.SystemPrompt != "" {
			desc += ": " + a.SystemPrompt
		}
		descLines = append(descLines, fmt.Sprintf("%d. %s", i+1, desc))
	}

	routerPrompt := fmt.Sprintf(`You are a router. Choose the most appropriate agent number for the user message.
Agents:
%s

Reply with ONLY the agent number (1, 2, 3, …). No explanation.`, strings.Join(descLines, "\n"))

	routerReq := openai.ChatCompletionRequest{
		Model:       "gpt-4o-mini",
		Temperature: 0.0,
		MaxTokens:   5,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: routerPrompt},
			{Role: openai.ChatMessageRoleUser, Content: req.Message},
		},
	}
	resp, err := s.provider.CreateChatCompletion(ctx, routerReq)
	if err != nil || len(resp.Choices) == 0 {
		return nil, nil
	}
	raw := strings.TrimSpace(resp.Choices[0].Message.Content)
	for i, a := range agents {
		if raw == fmt.Sprintf("%d", i+1) {
			copy := a
			return &copy, nil
		}
	}
	return nil, nil // unrecognised answer → fall through to default
}

// --- prompt + RAG ---

func (s *LLMService) buildSystemPrompt(base, ragContext string) string {
	return s.buildSystemPromptWithMemory(base, ragContext, "", "")
}

// buildSystemPromptWithMemory adds RAG context plus a light memory header
// (platform + chat user identifier) so the LLM knows who it's talking to.
func (s *LLMService) buildSystemPromptWithMemory(base, ragContext, platform, chatUserID string) string {
	sb := strings.Builder{}
	sb.WriteString(base)

	if platform != "" || chatUserID != "" {
		sb.WriteString("\n\n--- Контекст диалога ---")
		if platform != "" {
			sb.WriteString("\nПлатформа: " + platform)
		}
		if chatUserID != "" {
			sb.WriteString("\nID пользователя: " + chatUserID)
		}
		sb.WriteString("\n--- Конец контекста ---")
	}

	if ragContext != "" {
		sb.WriteString("\n\nКонтекст из базы знаний:\n" + ragContext)
	}
	return sb.String()
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
		"access_mode":  "AUTO_QUERY", // only search namespaces with access_mode=AUTO_QUERY
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

// ── Social / VK orchestration ────────────────────────────────────────────────

// SocialMessageRequest carries a VK inbound message plus the integration's
// policy context, sent from social-service to this endpoint.
type SocialMessageRequest struct {
	WorkspaceID   uuid.UUID         `json:"workspace_id"`
	IntegrationID string            `json:"integration_id"`
	ChatUserID    string            `json:"chat_user_id"`
	Platform      string            `json:"platform"`
	Message       string            `json:"message"`
	Context       *SocialMsgContext `json:"context,omitempty"`
}

type SocialMsgContext struct {
	ToneOfVoice       string   `json:"tone_of_voice,omitempty"`
	SafeIntents       []string `json:"safe_intents,omitempty"`
	ForbiddenPromises []string `json:"forbidden_promises,omitempty"`
	EscalationPolicy  string   `json:"escalation_policy,omitempty"`
	AutoReplyEnabled  bool     `json:"auto_reply_enabled"`
	BusinessSnapshot  string   `json:"business_snapshot,omitempty"`
	// DigitalTwin is the enriched lead profile sent from social-service.
	// When present it is injected into the system prompt so the agent can
	// address the user by name, reference their city, occupation, or
	// Pinterest interests.
	DigitalTwin *DigitalTwinContext `json:"digital_twin,omitempty"`
}

// DigitalTwinContext mirrors social-service's DigitalTwin struct.
// Kept as a flat struct (no import cycle) — serialised over the wire.
type DigitalTwinContext struct {
	FirstName       string   `json:"first_name,omitempty"`
	LastName        string   `json:"last_name,omitempty"`
	City            string   `json:"city,omitempty"`
	Country         string   `json:"country,omitempty"`
	Company         string   `json:"company,omitempty"`
	JobTitle        string   `json:"job_title,omitempty"`
	Bio             string   `json:"bio,omitempty"`
	Status          string   `json:"status,omitempty"`
	FollowersCount  int      `json:"followers_count,omitempty"`
	PinterestURL    string   `json:"pinterest_url,omitempty"`
	PinterestBoards []string `json:"pinterest_boards,omitempty"`
	FacebookName    string   `json:"facebook_name,omitempty"`
	LeadScore       int      `json:"lead_score,omitempty"`
}

// summaryForLLM formats the twin as a compact profile block for the system prompt.
func (d *DigitalTwinContext) summaryForLLM() string {
	if d == nil {
		return ""
	}
	var lines []string

	if name := strings.TrimSpace(d.FirstName + " " + d.LastName); name != "" {
		lines = append(lines, "Имя: "+name)
	}
	var locParts []string
	if d.City != "" {
		locParts = append(locParts, d.City)
	}
	if d.Country != "" {
		locParts = append(locParts, d.Country)
	}
	if len(locParts) > 0 {
		lines = append(lines, "Местоположение: "+strings.Join(locParts, ", "))
	}
	if d.Company != "" || d.JobTitle != "" {
		occ := d.JobTitle
		if d.Company != "" {
			if occ != "" {
				occ += " в " + d.Company
			} else {
				occ = d.Company
			}
		}
		lines = append(lines, "Работа: "+occ)
	}
	if d.Bio != "" {
		bio := d.Bio
		if len(bio) > 200 {
			bio = bio[:197] + "..."
		}
		lines = append(lines, "О себе: "+bio)
	}
	if d.Status != "" {
		lines = append(lines, "Статус: \""+d.Status+"\"")
	}
	if d.FollowersCount > 0 {
		lines = append(lines, fmt.Sprintf("Подписчиков: %d", d.FollowersCount))
	}
	if len(d.PinterestBoards) > 0 {
		boards := d.PinterestBoards
		if len(boards) > 6 {
			boards = boards[:6]
		}
		lines = append(lines, "Pinterest интересы: "+strings.Join(boards, ", "))
	} else if d.PinterestURL != "" {
		lines = append(lines, "Pinterest: "+d.PinterestURL)
	}
	if d.FacebookName != "" {
		lines = append(lines, "Facebook: "+d.FacebookName)
	}
	if d.LeadScore > 0 {
		lines = append(lines, fmt.Sprintf("Оценка лида: %d/100", d.LeadScore))
	}

	if len(lines) == 0 {
		return ""
	}
	return "--- Профиль клиента ---\n" +
		strings.Join(lines, "\n") +
		"\n--- Конец профиля ---"
}

// VKOrchestrationDecision is the structured response returned to social-service.
type VKOrchestrationDecision struct {
	Mode              string     `json:"mode"` // "draft" | "auto_reply" | "escalate"
	DraftText         string     `json:"draft_text"`
	Confidence        float64    `json:"confidence"`
	Intent            string     `json:"intent"`
	SafeIntent        bool       `json:"safe_intent"`
	Rationale         string     `json:"rationale"`
	KnowledgeSnippets []string   `json:"knowledge_snippets"`
	UsedTools         []string   `json:"used_tools,omitempty"`
	PromptTokens      int        `json:"prompt_tokens"`
	CompletionTokens  int        `json:"completion_tokens"`
	TokensUsed        int        `json:"tokens_used"`
	AgentID           *uuid.UUID `json:"agent_id,omitempty"`
}

// vkDecisionSchema is the function schema that forces the model to always
// respond with a structured reply decision (tool_choice="required").
var vkDecisionSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "mode":       {"type": "string", "enum": ["draft", "auto_reply", "escalate"]},
    "draft_text": {"type": "string"},
    "confidence": {"type": "number", "minimum": 0, "maximum": 1},
    "intent":     {"type": "string"},
    "safe_intent":{"type": "boolean"},
    "rationale":  {"type": "string"}
  },
  "required": ["mode", "draft_text", "confidence", "intent", "safe_intent", "rationale"]
}`)

// ProcessSocialMessage runs the VK inbound-message orchestration:
//  1. Resolves agent config from agent-service (graceful fallback).
//  2. Fetches RAG context scoped to agent namespaces.
//  3. Uses function calling with tool_choice="required" to get a structured
//     reply decision: mode, draft_text, confidence, intent, safe_intent, rationale.
//  4. Returns VKOrchestrationDecision.
func (s *LLMService) ProcessSocialMessage(ctx context.Context, req SocialMessageRequest) (*VKOrchestrationDecision, error) {
	// Synthesise a ChatRequest for agent resolution.
	chatReq := ChatRequest{
		WorkspaceID: req.WorkspaceID,
		ChatUserID:  req.ChatUserID,
		Platform:    req.Platform,
		Message:     req.Message,
	}
	params, agentID, _ := s.resolveAgentConfig(ctx, chatReq)

	// RAG context.
	ragContext := s.fetchRAGContext(ctx, req.WorkspaceID, req.Message, params.ragTopK, params.namespaces)

	// Build the snippets list for the response (plain strings).
	snippets := []string{}
	if ragContext != "" {
		for _, part := range strings.Split(ragContext, "\n\n---\n\n") {
			if p := strings.TrimSpace(part); p != "" {
				snippets = append(snippets, p)
			}
		}
	}

	// Build the system prompt, layering agent config + integration policy + memory context.
	systemPrompt := s.buildVKSystemPromptWithMemory(params.systemPmt, req.Context, ragContext, req.Platform, req.ChatUserID)

	decisionTool := openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "vk_reply_decision",
			Description: "Generate a structured reply decision for a VK inbound message.",
			Parameters:  vkDecisionSchema,
		},
	}

	compReq := openai.ChatCompletionRequest{
		Model: params.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: req.Message},
		},
		Temperature:       params.temperature,
		MaxTokens:         600,
		Tools:             []openai.Tool{decisionTool},
		ToolChoice:        openai.ToolChoice{Type: openai.ToolTypeFunction, Function: openai.ToolFunction{Name: "vk_reply_decision"}},
		ParallelToolCalls: false,
	}

	resp, err := s.provider.CreateChatCompletion(ctx, compReq)
	if err != nil {
		log.Printf("[vk-agent][llm] tool-call path failed for provider=%s model=%s: %v", s.provider.Name(), params.model, err)
		decision, fallbackErr := s.processSocialMessageJSONFallback(ctx, params, req, agentID, ragContext, snippets)
		if fallbackErr != nil {
			return nil, fmt.Errorf("%s social provider: %w", s.provider.Name(), err)
		}
		return decision, nil
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai social: no choices")
	}

	choice := resp.Choices[0]
	promptTok := resp.Usage.PromptTokens
	completionTok := resp.Usage.CompletionTokens
	totalTok := resp.Usage.TotalTokens

	// Parse the tool call arguments.
	var args struct {
		Mode       string  `json:"mode"`
		DraftText  string  `json:"draft_text"`
		Confidence float64 `json:"confidence"`
		Intent     string  `json:"intent"`
		SafeIntent bool    `json:"safe_intent"`
		Rationale  string  `json:"rationale"`
	}
	if len(choice.Message.ToolCalls) > 0 {
		if err := json.Unmarshal([]byte(choice.Message.ToolCalls[0].Function.Arguments), &args); err != nil {
			log.Printf("[vk-agent][llm] tool args parse failed, retrying via JSON fallback: %v", err)
			decision, fallbackErr := s.processSocialMessageJSONFallback(ctx, params, req, agentID, ragContext, snippets)
			if fallbackErr != nil {
				return nil, fmt.Errorf("parse decision args: %w", err)
			}
			return decision, nil
		}
	} else {
		parsed, err := parseVKDecisionContent(choice.Message.Content)
		if err != nil {
			// Fallback: model returned plain text instead of a tool call.
			args.Mode = "draft"
			args.DraftText = strings.TrimSpace(choice.Message.Content)
			args.Confidence = 0.5
			args.Intent = "unknown"
			args.SafeIntent = false
			args.Rationale = "plain-text fallback"
		} else {
			args.Mode = parsed.Mode
			args.DraftText = parsed.DraftText
			args.Confidence = parsed.Confidence
			args.Intent = parsed.Intent
			args.SafeIntent = parsed.SafeIntent
			args.Rationale = parsed.Rationale
		}
	}

	// Override mode based on integration policy: if context says auto_reply
	// is disabled, downgrade auto_reply → draft.
	if args.Mode == "auto_reply" && req.Context != nil && !req.Context.AutoReplyEnabled {
		args.Mode = "draft"
	}

	decision := &VKOrchestrationDecision{
		Mode:              args.Mode,
		DraftText:         args.DraftText,
		Confidence:        args.Confidence,
		Intent:            args.Intent,
		SafeIntent:        args.SafeIntent,
		Rationale:         args.Rationale,
		KnowledgeSnippets: snippets,
		PromptTokens:      promptTok,
		CompletionTokens:  completionTok,
		TokensUsed:        totalTok,
		AgentID:           agentID,
	}

	// If the agent decided to escalate, send AGENT_STUCK push to the workspace owner.
	if args.Mode == "escalate" {
		s.push.SendAsync(req.WorkspaceID, NotifAgentStuck,
			"⚠️ Требуется ваше внимание",
			fmt.Sprintf("Агент не смог ответить пользователю %s (%s): %s", req.ChatUserID, req.Platform, args.Rationale),
			map[string]string{
				"chat_user_id": req.ChatUserID,
				"platform":     req.Platform,
			},
		)
	}

	// Async lead scoring — fires HOT_LEAD push if score is above threshold.
	s.scoring.ScoreAsync(req.WorkspaceID, req.ChatUserID, req.Platform, req.Message, func(score *ScoreResult) {
		s.push.SendAsync(req.WorkspaceID, NotifHotLead,
			"🔥 Горячий лид!",
			fmt.Sprintf("Пользователь %s (%s) набрал %d баллов", req.ChatUserID, req.Platform, score.Score),
			map[string]string{
				"chat_user_id": req.ChatUserID,
				"platform":     req.Platform,
				"score":        fmt.Sprintf("%d", score.Score),
			},
		)
	})

	return decision, nil
}

func (s *LLMService) processSocialMessageJSONFallback(
	ctx context.Context,
	params runtimeParams,
	req SocialMessageRequest,
	agentID *uuid.UUID,
	ragContext string,
	snippets []string,
) (*VKOrchestrationDecision, error) {
	systemPrompt := s.buildVKJSONSystemPrompt(params.systemPmt, req.Context, ragContext)
	compReq := openai.ChatCompletionRequest{
		Model: params.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: req.Message},
		},
		Temperature: params.temperature,
		MaxTokens:   600,
	}

	resp, err := s.provider.CreateChatCompletion(ctx, compReq)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai social json fallback: no choices")
	}

	parsed, err := parseVKDecisionContent(resp.Choices[0].Message.Content)
	if err != nil {
		return nil, fmt.Errorf("parse social json fallback: %w", err)
	}
	if parsed.Mode == "auto_reply" && req.Context != nil && !req.Context.AutoReplyEnabled {
		parsed.Mode = "draft"
	}

	return &VKOrchestrationDecision{
		Mode:              parsed.Mode,
		DraftText:         parsed.DraftText,
		Confidence:        parsed.Confidence,
		Intent:            parsed.Intent,
		SafeIntent:        parsed.SafeIntent,
		Rationale:         parsed.Rationale,
		KnowledgeSnippets: snippets,
		PromptTokens:      resp.Usage.PromptTokens,
		CompletionTokens:  resp.Usage.CompletionTokens,
		TokensUsed:        resp.Usage.TotalTokens,
		AgentID:           agentID,
	}, nil
}

func (s *LLMService) buildVKSystemPromptWithMemory(agentBase string, ctx *SocialMsgContext, ragContext, platform, chatUserID string) string {
	base := s.buildVKSystemPrompt(agentBase, ctx, ragContext)
	sb := strings.Builder{}
	sb.WriteString(base)

	// Inject digital twin profile — highest-value personalisation signal.
	var twin *DigitalTwinContext
	if ctx != nil {
		twin = ctx.DigitalTwin
	}
	if twinSummary := twin.summaryForLLM(); twinSummary != "" {
		sb.WriteString("\n\n" + twinSummary)
	}

	// Append lightweight session context (platform + user ID).
	if platform != "" || chatUserID != "" {
		sb.WriteString("\n\n--- Контекст сессии ---")
		if platform != "" {
			sb.WriteString("\nПлатформа: " + platform)
		}
		if chatUserID != "" {
			sb.WriteString("\nID пользователя: " + chatUserID)
		}
		sb.WriteString("\n--- Конец контекста ---")
	}
	return sb.String()
}

func (s *LLMService) buildVKSystemPrompt(agentBase string, ctx *SocialMsgContext, ragContext string) string {
	sb := strings.Builder{}
	sb.WriteString(agentBase)
	sb.WriteString("\n\nТы отвечаешь от лица VK-сообщества бизнеса.\n")
	sb.WriteString("Отвечай полностью на языке пользователя. Если сообщение на русском, не используй английские слова и фразы без явной необходимости.\n")

	if ctx != nil {
		tone := ctx.ToneOfVoice
		if tone == "" {
			tone = "спокойный, полезный, вежливый"
		}
		sb.WriteString("Тон: " + tone + ".\n")

		if ctx.BusinessSnapshot != "" {
			sb.WriteString("Контекст бизнеса: " + ctx.BusinessSnapshot + "\n")
		}
		if len(ctx.ForbiddenPromises) > 0 {
			sb.WriteString("Нельзя обещать: " + strings.Join(ctx.ForbiddenPromises, "; ") + ".\n")
		}
		if ctx.EscalationPolicy != "" {
			sb.WriteString("Политика эскалации: " + ctx.EscalationPolicy + "\n")
		}
		if len(ctx.SafeIntents) > 0 {
			sb.WriteString("Безопасные интенты для auto_reply: " + strings.Join(ctx.SafeIntents, ", ") + "\n")
		}
	}

	if ragContext != "" {
		sb.WriteString("\nКонтекст из базы знаний:\n" + ragContext + "\n")
	}

	sb.WriteString(`
Вызови функцию vk_reply_decision с полями:
- mode: "draft" (требует проверки менеджером) | "auto_reply" (можно отправить автоматически) | "escalate" (передать человеку)
- draft_text: текст ответа на русском, коротко и конкретно
- confidence: вероятность 0..1
- intent: классификация намерения (faq, hours, basic_prices, qualification, handoff, unknown)
- safe_intent: true если можно отвечать без проверки
- rationale: краткое объяснение решения
`)
	return sb.String()
}

func (s *LLMService) buildVKJSONSystemPrompt(agentBase string, ctx *SocialMsgContext, ragContext string) string {
	sb := strings.Builder{}
	sb.WriteString(s.buildVKSystemPrompt(agentBase, ctx, ragContext))

	sb.WriteString(`

Вместо вызова функции верни только один JSON-объект без markdown, без code fences и без пояснений.
Строгая схема ответа:
{"mode":"draft|auto_reply|escalate","draft_text":"...","confidence":0.0,"intent":"faq|hours|basic_prices|qualification|handoff|unknown","safe_intent":true,"rationale":"..."}
`)
	return sb.String()
}

// --- small helpers ---

type vkDecisionArgs struct {
	Mode       string  `json:"mode"`
	DraftText  string  `json:"draft_text"`
	Confidence float64 `json:"confidence"`
	Intent     string  `json:"intent"`
	SafeIntent bool    `json:"safe_intent"`
	Rationale  string  `json:"rationale"`
}

func parseVKDecisionContent(content string) (*vkDecisionArgs, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, fmt.Errorf("empty content")
	}
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```json")
		trimmed = strings.TrimPrefix(trimmed, "```JSON")
		trimmed = strings.TrimPrefix(trimmed, "```")
		trimmed = strings.TrimSuffix(trimmed, "```")
		trimmed = strings.TrimSpace(trimmed)
	}

	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start == -1 || end == -1 || end < start {
		return nil, fmt.Errorf("json object not found")
	}

	var parsed vkDecisionArgs
	if err := json.Unmarshal([]byte(trimmed[start:end+1]), &parsed); err != nil {
		return nil, err
	}
	if parsed.Mode == "" {
		return nil, fmt.Errorf("mode is required")
	}
	if parsed.Intent == "" {
		parsed.Intent = "unknown"
	}
	if parsed.Rationale == "" {
		parsed.Rationale = "json fallback"
	}
	return &parsed, nil
}

func containsTool(used []string, name string) bool {
	for _, t := range used {
		if t == name {
			return true
		}
	}
	return false
}

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
