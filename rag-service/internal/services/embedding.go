package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// Embedder is the interface satisfied by all embedding backends.
// The distinction between EmbedBatch and EmbedQuery matters for
// asymmetric models (e.g. Cohere) that use different encoders for
// indexing vs. retrieval. For symmetric models (OpenAI) both methods
// call the same endpoint.
type Embedder interface {
	// EmbedBatch embeds texts for indexing (documents, QA pairs, web pages).
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	// EmbedQuery embeds a single search query.
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
}

// ── OpenAI (or any OpenAI-compatible endpoint) ────────────────────────────────

// EmbeddingService wraps the OpenAI embeddings API.
// The name is kept for backward compatibility; new code should use Embedder.
type EmbeddingService struct {
	client *openai.Client
	model  string
}

func NewEmbeddingService(apiKey, model string) *EmbeddingService {
	return &EmbeddingService{
		client: openai.NewClient(apiKey),
		model:  model,
	}
}

func (s *EmbeddingService) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	const batchSize = 100
	var all [][]float32
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		resp, err := s.client.CreateEmbeddings(ctx, openai.EmbeddingRequestStrings{
			Input: texts[i:end],
			Model: openai.EmbeddingModel(s.model),
		})
		if err != nil {
			return nil, fmt.Errorf("openai embed: %w", err)
		}
		for _, d := range resp.Data {
			all = append(all, d.Embedding)
		}
	}
	return all, nil
}

// EmbedQuery for OpenAI is identical to EmbedBatch — no input_type distinction.
func (s *EmbeddingService) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vecs, err := s.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

// EmbedText is an alias for EmbedQuery kept for call-sites that predate the
// Embedder interface.
func (s *EmbeddingService) EmbedText(ctx context.Context, text string) ([]float32, error) {
	return s.EmbedQuery(ctx, text)
}

// ── Cohere ────────────────────────────────────────────────────────────────────

// CohereEmbeddingService calls the Cohere v2 /embed endpoint.
// It uses input_type=search_document for indexing and search_query for
// retrieval — this asymmetry is key to Cohere's quality advantage.
type CohereEmbeddingService struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewCohereEmbeddingService(apiKey, model string) *CohereEmbeddingService {
	if model == "" {
		model = "embed-multilingual-v3.0"
	}
	return &CohereEmbeddingService{
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type cohereEmbedReq struct {
	Texts          []string `json:"texts"`
	Model          string   `json:"model"`
	InputType      string   `json:"input_type"`
	EmbeddingTypes []string `json:"embedding_types"`
}

type cohereEmbedResp struct {
	Embeddings struct {
		Float [][]float32 `json:"float"`
	} `json:"embeddings"`
	Message string `json:"message"` // set on API errors
}

func (s *CohereEmbeddingService) embed(ctx context.Context, texts []string, inputType string) ([][]float32, error) {
	const batchSize = 96 // Cohere max is 96 texts per request
	var all [][]float32

	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}

		body, err := json.Marshal(cohereEmbedReq{
			Texts:          texts[i:end],
			Model:          s.model,
			InputType:      inputType,
			EmbeddingTypes: []string{"float"},
		})
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			"https://api.cohere.com/v2/embed", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
		req.Header.Set("X-Client-Name", "inbetwin-rag")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("cohere embed: %w", err)
		}
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()

		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("cohere HTTP %d: %s", resp.StatusCode, string(respBody))
		}

		var result cohereEmbedResp
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("cohere decode: %w", err)
		}
		if result.Message != "" {
			return nil, fmt.Errorf("cohere error: %s", result.Message)
		}
		all = append(all, result.Embeddings.Float...)
	}
	return all, nil
}

// EmbedBatch uses input_type=search_document (optimised for indexing).
func (s *CohereEmbeddingService) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return s.embed(ctx, texts, "search_document")
}

// EmbedQuery uses input_type=search_query (optimised for retrieval).
func (s *CohereEmbeddingService) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vecs, err := s.embed(ctx, []string{text}, "search_query")
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

// ── Factory ───────────────────────────────────────────────────────────────────

// NewEmbedder returns the appropriate Embedder based on EMBEDDING_PROVIDER.
// "cohere" → CohereEmbeddingService; anything else → EmbeddingService (OpenAI).
func NewEmbedder(provider, apiKey, model string) Embedder {
	if provider == "cohere" {
		return NewCohereEmbeddingService(apiKey, model)
	}
	if model == "" {
		model = "text-embedding-3-small"
	}
	return NewEmbeddingService(apiKey, model)
}
