package services

import (
	"context"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
)

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

func (s *EmbeddingService) EmbedText(ctx context.Context, text string) ([]float32, error) {
	vecs, err := s.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

func (s *EmbeddingService) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	const batchSize = 100
	var all [][]float32
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]
		resp, err := s.client.CreateEmbeddings(ctx, openai.EmbeddingRequestStrings{
			Input: batch,
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
