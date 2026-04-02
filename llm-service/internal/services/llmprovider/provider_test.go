package llmprovider

// Tests for the provider-neutral ChatCompletionProvider factory.
//
// These are pure unit tests — no DB, no network, no external services.
// They verify that NewChatCompletionProvider correctly resolves the provider
// strategy from config fields (LLM_PROVIDER, LLM_API_KEY, LLM_BASE_URL).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	openai "github.com/sashabaranov/go-openai"

	"github.com/dinarasaurae/inbetwin-llm-service/internal/config"
)

// buildCfg is a helper to build a minimal Config for provider tests.
func buildCfg(provider, apiKey, baseURL string) *config.Config {
	return &config.Config{
		LLMProvider: provider,
		LLMAPIKey:   apiKey,
		LLMBaseURL:  baseURL,
	}
}

// ── Provider name normalisation ───────────────────────────────────────────────

func TestNormalizeProvider_OpenAI(t *testing.T) {
	cases := []string{"openai", "OpenAI", "OPENAI", ""}
	for _, in := range cases {
		got := normalizeProvider(in)
		if got != "openai" {
			t.Errorf("normalizeProvider(%q) = %q, want openai", in, got)
		}
	}
}

func TestNormalizeProvider_KnownProviders(t *testing.T) {
	cases := map[string]string{
		"groq":            "groq",
		"openrouter":      "openrouter",
		"huggingface":     "huggingface",
		"hf":              "huggingface",
		"openai_compatible": "openai_compatible",
		"openai-compatible": "openai_compatible",
	}
	for in, want := range cases {
		got := normalizeProvider(in)
		if got != want {
			t.Errorf("normalizeProvider(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeProvider_CustomPassthrough(t *testing.T) {
	// Unknown providers are lowercased and returned as-is.
	got := normalizeProvider("myCustomProvider")
	if got != "mycustomprovider" {
		t.Errorf("normalizeProvider(custom) = %q, want mycustomprovider", got)
	}
}

// ── Base URL resolution ───────────────────────────────────────────────────────

func TestResolveBaseURL_ExplicitOverridesBuiltin(t *testing.T) {
	explicit := "https://my.proxy/v1"
	got := resolveBaseURL("groq", explicit)
	if got != explicit {
		t.Errorf("explicit URL should win: got %q", got)
	}
}

func TestResolveBaseURL_GroqBuiltin(t *testing.T) {
	got := resolveBaseURL("groq", "")
	want := "https://api.groq.com/openai/v1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveBaseURL_OpenRouterBuiltin(t *testing.T) {
	got := resolveBaseURL("openrouter", "")
	want := "https://openrouter.ai/api/v1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveBaseURL_HuggingFaceBuiltin(t *testing.T) {
	got := resolveBaseURL("huggingface", "")
	want := "https://router.huggingface.co/v1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveBaseURL_OpenAIEmpty(t *testing.T) {
	// OpenAI uses the SDK default; resolveBaseURL returns "" so the SDK handles it.
	got := resolveBaseURL("openai", "")
	if got != "" {
		t.Errorf("openai should return empty base URL, got %q", got)
	}
}

// ── Factory: disabled provider ────────────────────────────────────────────────

func TestFactory_NoAPIKey_ReturnsDisabledProvider(t *testing.T) {
	cfg := buildCfg("openai", "", "")
	p := NewChatCompletionProvider(cfg)
	if p.Name() != "openai" {
		t.Errorf("name: got %q, want openai", p.Name())
	}
	_, err := p.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{})
	if err == nil {
		t.Error("expected error from disabled provider, got nil")
	}
}

func TestFactory_Groq_NoAPIKey_Disabled(t *testing.T) {
	cfg := buildCfg("groq", "", "")
	p := NewChatCompletionProvider(cfg)
	_, err := p.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{})
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// ── Factory: provider names ───────────────────────────────────────────────────

func TestFactory_OpenAI_Name(t *testing.T) {
	cfg := buildCfg("openai", "sk-test", "")
	p := NewChatCompletionProvider(cfg)
	if p.Name() != "openai" {
		t.Errorf("Name() = %q, want openai", p.Name())
	}
}

func TestFactory_Groq_Name(t *testing.T) {
	cfg := buildCfg("groq", "gsk_test", "")
	p := NewChatCompletionProvider(cfg)
	if p.Name() != "groq" {
		t.Errorf("Name() = %q, want groq", p.Name())
	}
}

func TestFactory_OpenRouter_Name(t *testing.T) {
	cfg := buildCfg("openrouter", "sk-or-test", "")
	p := NewChatCompletionProvider(cfg)
	if p.Name() != "openrouter" {
		t.Errorf("Name() = %q, want openrouter", p.Name())
	}
}

func TestFactory_CustomBaseURL_Name(t *testing.T) {
	cfg := buildCfg("openai_compatible", "sk-custom", "https://my.llm.server/v1")
	p := NewChatCompletionProvider(cfg)
	if p.Name() != "openai_compatible" {
		t.Errorf("Name() = %q, want openai_compatible", p.Name())
	}
}

// ── End-to-end: provider routes requests to the correct base URL ─────────────

// TestProviderNeutral_RoutesToConfiguredBaseURL spins up a local httptest
// server simulating an OpenAI-compatible endpoint and verifies that a provider
// created with a custom LLM_BASE_URL actually sends requests there.
// This proves that switching providers (openai → groq/openrouter/custom)
// only requires a config change — no code change needed.
func TestProviderNeutral_RoutesToConfiguredBaseURL(t *testing.T) {
	// Track that the request arrived at the test server.
	var gotRequest bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequest = true
		// Return a minimal valid OpenAI response so the SDK doesn't error.
		resp := openai.ChatCompletionResponse{
			ID:     "chatcmpl-test",
			Object: "chat.completion",
			Choices: []openai.ChatCompletionChoice{
				{
					Index: 0,
					Message: openai.ChatCompletionMessage{
						Role:    openai.ChatMessageRoleAssistant,
						Content: "Hello from mock provider",
					},
					FinishReason: openai.FinishReasonStop,
				},
			},
			Usage: openai.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer srv.Close()

	// Provider = openai_compatible with our test server as base URL.
	cfg := buildCfg("openai_compatible", "sk-test", srv.URL+"/v1")
	p := NewChatCompletionProvider(cfg)

	if p.Name() != "openai_compatible" {
		t.Fatalf("provider name: got %q, want openai_compatible", p.Name())
	}

	req := openai.ChatCompletionRequest{
		Model: "gpt-4o-mini",
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: "test"},
		},
	}
	resp, err := p.CreateChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateChatCompletion: %v", err)
	}
	if !gotRequest {
		t.Error("provider did not route the request to the configured base URL")
	}
	if len(resp.Choices) == 0 {
		t.Error("expected at least one choice in response")
	}
	if resp.Choices[0].Message.Content != "Hello from mock provider" {
		t.Errorf("content: got %q", resp.Choices[0].Message.Content)
	}
}

// TestProviderNeutral_SameLogic_DifferentProviders verifies that the same
// factory code produces correctly-named providers for openai/groq/openrouter —
// only the config changes, not the calling code.
func TestProviderNeutral_SameLogic_DifferentProviders(t *testing.T) {
	providerCases := []struct {
		envProvider string
		wantName    string
	}{
		{"openai", "openai"},
		{"groq", "groq"},
		{"openrouter", "openrouter"},
		{"huggingface", "huggingface"},
		{"openai_compatible", "openai_compatible"},
	}

	for _, tc := range providerCases {
		t.Run(fmt.Sprintf("provider=%s", tc.envProvider), func(t *testing.T) {
			cfg := buildCfg(tc.envProvider, "sk-test-key", "")
			p := NewChatCompletionProvider(cfg)
			if p.Name() != tc.wantName {
				t.Errorf("Name() = %q, want %q", p.Name(), tc.wantName)
			}
			// Provider must be functional (not disabled) when key is present.
			// We don't make real API calls; we just verify it's not a disabledProvider.
			if _, ok := p.(*disabledProvider); ok {
				t.Errorf("provider %q with API key should not be disabled", tc.envProvider)
			}
		})
	}
}
