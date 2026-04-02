package services

import (
	"context"
	"fmt"
	"log"

	"github.com/pinecone-io/go-pinecone/pinecone"
	"google.golang.org/protobuf/types/known/structpb"
)

type VectorRecord struct {
	ID       string
	Values   []float32
	Metadata map[string]interface{}
}

type QueryResult struct {
	ID       string
	Score    float32
	Metadata map[string]interface{}
}

type PineconeService struct {
	client    *pinecone.Client
	indexHost string
	indexName string
}

func NewPineconeService(apiKey, indexHost, indexName string) (*PineconeService, error) {
	client, err := pinecone.NewClient(pinecone.NewClientParams{ApiKey: apiKey})
	if err != nil {
		return nil, fmt.Errorf("pinecone client: %w", err)
	}
	return &PineconeService{client: client, indexHost: indexHost, indexName: indexName}, nil
}

func (s *PineconeService) getIndex(ctx context.Context, namespace string) (*pinecone.IndexConnection, error) {
	return s.client.Index(pinecone.NewIndexConnParams{Host: s.indexHost, Namespace: namespace})
}

func (s *PineconeService) UpsertVectors(ctx context.Context, namespace string, records []VectorRecord) error {
	idx, err := s.getIndex(ctx, namespace)
	if err != nil {
		return err
	}
	defer idx.Close()
	const batchSize = 100
	for i := 0; i < len(records); i += batchSize {
		end := i + batchSize
		if end > len(records) {
			end = len(records)
		}
		batch := records[i:end]
		var vecs []*pinecone.Vector
		for _, r := range batch {
			vals := r.Values
			meta, _ := structpb.NewStruct(r.Metadata)
			vecs = append(vecs, &pinecone.Vector{
				Id:       r.ID,
				Values:   vals,
				Metadata: meta,
			})
		}
		if _, err := idx.UpsertVectors(ctx, vecs); err != nil {
			return fmt.Errorf("upsert batch: %w", err)
		}
	}
	return nil
}

func (s *PineconeService) QueryVectors(ctx context.Context, namespace string, vector []float32, topK uint32, filter map[string]interface{}) ([]QueryResult, error) {
	idx, err := s.getIndex(ctx, namespace)
	if err != nil {
		return nil, err
	}
	defer idx.Close()
	var metaFilter *structpb.Struct
	if len(filter) > 0 {
		metaFilter, _ = structpb.NewStruct(filter)
	}
	res, err := idx.QueryByVectorValues(ctx, &pinecone.QueryByVectorValuesRequest{
		Vector:          vector,
		TopK:            topK,
		MetadataFilter:  metaFilter,
		IncludeValues:   false,
		IncludeMetadata: true,
	})
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	var out []QueryResult
	for _, m := range res.Matches {
		meta := make(map[string]interface{})
		if m.Vector.Metadata != nil {
			meta = m.Vector.Metadata.AsMap()
		}
		out = append(out, QueryResult{
			ID:       m.Vector.Id,
			Score:    m.Score,
			Metadata: meta,
		})
	}
	return out, nil
}

func (s *PineconeService) DeleteVectors(ctx context.Context, namespace string, ids []string) error {
	idx, err := s.getIndex(ctx, namespace)
	if err != nil {
		return err
	}
	defer idx.Close()
	const batchSize = 1000
	for i := 0; i < len(ids); i += batchSize {
		end := i + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		if err := idx.DeleteVectorsById(ctx, ids[i:end]); err != nil {
			log.Printf("warn delete batch: %v", err)
		}
	}
	return nil
}

func (s *PineconeService) DeleteNamespace(ctx context.Context, namespace string) error {
	idx, err := s.getIndex(ctx, namespace)
	if err != nil {
		return err
	}
	defer idx.Close()
	return idx.DeleteAllVectorsInNamespace(ctx)
}
