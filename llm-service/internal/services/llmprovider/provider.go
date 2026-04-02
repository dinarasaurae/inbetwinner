package llmprovider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	openai "github.com/sashabaranov/go-openai"

	"github.com/dinarasaurae/inbetwin-llm-service/internal/config"
)

// ChatCompletionProvider is the provider strategy used by llm-service.
// The transport shape stays OpenAI-compatible, but provider selection is decoupled
// from the orchestration logic via this interface.
type ChatCompletionProvider interface {
	CreateChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
	Name() string
}

type openAICompatibleProvider struct {
	name   string
	client *openai.Client
}

func (p *openAICompatibleProvider) CreateChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	return p.client.CreateChatCompletion(ctx, req)
}

func (p *openAICompatibleProvider) Name() string {
	return p.name
}

type disabledProvider struct {
	name   string
	reason string
}

func (p *disabledProvider) CreateChatCompletion(context.Context, openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	return openai.ChatCompletionResponse{}, errors.New(p.reason)
}

func (p *disabledProvider) Name() string {
	return p.name
}

// NewChatCompletionProvider is a factory that resolves the provider strategy
// from config. All supported providers currently use an OpenAI-compatible API.
func NewChatCompletionProvider(cfg *config.Config) ChatCompletionProvider {
	providerName := normalizeProvider(cfg.LLMProvider)
	if cfg.LLMAPIKey == "" {
		return &disabledProvider{
			name:   providerName,
			reason: fmt.Sprintf("%s provider is not configured: missing LLM_API_KEY", providerName),
		}
	}

	clientCfg := openai.DefaultConfig(cfg.LLMAPIKey)
	if baseURL := resolveBaseURL(providerName, cfg.LLMBaseURL); baseURL != "" {
		clientCfg.BaseURL = baseURL
	}

	return &openAICompatibleProvider{
		name:   providerName,
		client: openai.NewClientWithConfig(clientCfg),
	}
}

func normalizeProvider(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "openai":
		return "openai"
	case "openai_compatible", "openai-compatible":
		return "openai_compatible"
	case "groq":
		return "groq"
	case "openrouter":
		return "openrouter"
	case "huggingface", "hf":
		return "huggingface"
	default:
		return strings.ToLower(strings.TrimSpace(name))
	}
}

func resolveBaseURL(providerName, explicitBaseURL string) string {
	if explicitBaseURL != "" {
		return explicitBaseURL
	}
	switch providerName {
	case "groq":
		return "https://api.groq.com/openai/v1"
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	case "huggingface":
		return "https://router.huggingface.co/v1"
	case "openai_compatible":
		return ""
	default:
		return ""
	}
}
