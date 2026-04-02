package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/dinarasaurae/inbetwin-llm-service/internal/config"
	"github.com/dinarasaurae/inbetwin-llm-service/internal/models"
	histsvc "github.com/dinarasaurae/inbetwin-llm-service/internal/services/history"
	"github.com/dinarasaurae/inbetwin-llm-service/internal/services/tools"
	"github.com/google/uuid"
	openai "github.com/sashabaranov/go-openai"
)

type LLMService struct {
	client     *openai.Client
	history    *histsvc.Service
	registry   *tools.Registry
	dispatcher *tools.Dispatcher
	cfg        *config.Config
	httpClient *http.Client
}

type ChatRequest struct {
	ChatUserID  string    `json:"chat_user_id"`
	Platform    string    `json:"platform"`
	Message     string    `json:"message"`
	WorkspaceID uuid.UUID `json:"-"`
}

type ChatResponse struct {
	Response    string   `json:"response"`
	ToolsUsed   []string `json:"tools_used,omitempty"`
	TotalTokens int      `json:"total_tokens"`
}

func NewLLMService(cfg *config.Config, hist *histsvc.Service, reg *tools.Registry, disp *tools.Dispatcher) *LLMService {
	return &LLMService{
		client:     openai.NewClient(cfg.OpenAIAPIKey),
		history:    hist,
		registry:   reg,
		dispatcher: disp,
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *LLMService) ProcessMessage(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	histMsgs, _ := s.history.GetHistory(ctx, req.WorkspaceID, req.ChatUserID, s.cfg.HistorySize)

	ragContext := s.fetchRAGContext(ctx, req.WorkspaceID, req.Message)

	hasCalendar := s.registry.HasGoogleCalendar(ctx, req.WorkspaceID)
	availableTools, _ := s.registry.GetTools(ctx, req.WorkspaceID, hasCalendar)
	openAITools := tools.ToOpenAITools(availableTools)

	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: s.buildSystemPrompt(ragContext)},
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

	var usedTools []string
	totalTokens := 0

	for i := 0; i < s.cfg.MaxToolIterations; i++ {
		compReq := openai.ChatCompletionRequest{
			Model:       s.cfg.OpenAIModel,
			Messages:    messages,
			Temperature: s.cfg.OpenAITemp,
			MaxTokens:   s.cfg.OpenAIMaxTokens,
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
			Response:    answer,
			ToolsUsed:   usedTools,
			TotalTokens: totalTokens,
		}, nil
	}

	return &ChatResponse{
		Response:    "Превышено максимальное количество шагов обработки.",
		ToolsUsed:   usedTools,
		TotalTokens: totalTokens,
	}, nil
}

func (s *LLMService) buildSystemPrompt(ragContext string) string {
	base := `Ты умный AI-ассистент для B2B продаж. Отвечай на языке пользователя (по умолчанию русский).
Помогай квалифицировать лидов, отвечать на вопросы о продукте, назначать встречи и сохранять контактные данные.
Будь дружелюбным, профессиональным и конкретным.`
	if ragContext != "" {
		base += "\n\nКонтекст из базы знаний:\n" + ragContext
	}
	return base
}

func (s *LLMService) fetchRAGContext(ctx context.Context, workspaceID uuid.UUID, query string) string {
	if s.cfg.RAGServiceURL == "" {
		return ""
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"query":        query,
		"workspace_id": workspaceID.String(),
		"top_k":        3,
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
	combined := ""
	for i, r := range result.Results {
		if r.Text == "" {
			continue
		}
		if i > 0 {
			combined += "\n\n---\n\n"
		}
		combined += r.Text
	}
	return combined
}
