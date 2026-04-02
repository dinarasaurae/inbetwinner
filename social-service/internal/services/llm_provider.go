package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	openai "github.com/sashabaranov/go-openai"

	"github.com/dinarasaurae/inbetwin-social-service/internal/config"
)

// ChatCompletionProvider is the strategy used by the legacy VK draft composer.
// Provider-specific wiring is isolated here so the VK domain logic is not tied
// to a single vendor.
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

type disabledChatProvider struct {
	name   string
	reason string
}

func (p *disabledChatProvider) CreateChatCompletion(context.Context, openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	return openai.ChatCompletionResponse{}, errors.New(p.reason)
}

func (p *disabledChatProvider) Name() string {
	return p.name
}

func NewChatCompletionProvider(cfg *config.Config) ChatCompletionProvider {
	providerName := normalizeProviderName(cfg.LLMProvider)
	if cfg.LLMAPIKey == "" {
		return &disabledChatProvider{
			name:   providerName,
			reason: fmt.Sprintf("%s provider is not configured: missing LLM_API_KEY", providerName),
		}
	}

	clientCfg := openai.DefaultConfig(cfg.LLMAPIKey)
	if baseURL := resolveProviderBaseURL(providerName, cfg.LLMBaseURL); baseURL != "" {
		clientCfg.BaseURL = baseURL
	}

	return &openAICompatibleProvider{
		name:   providerName,
		client: openai.NewClientWithConfig(clientCfg),
	}
}

func normalizeProviderName(name string) string {
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

func resolveProviderBaseURL(providerName, explicitBaseURL string) string {
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
