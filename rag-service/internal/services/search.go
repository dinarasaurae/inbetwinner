package services

import (
	"context"
	"fmt"
	"sort"

	"github.com/dinarasaurae/inbetwin-rag-service/internal/database"
	"github.com/google/uuid"
)

type SearchService struct {
	db        *database.DB
	embedding *EmbeddingService
	pinecone  *PineconeService
	nsSvc     *NamespaceService
}

type SearchRequest struct {
	Query       string    `json:"query"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	Namespaces  []string  `json:"namespaces"`
	TopK        int       `json:"top_k"`
	MinScore    float32   `json:"min_score"`
}

type SearchResult struct {
	Text       string                 `json:"text"`
	Score      float32                `json:"score"`
	Source     string                 `json:"source"`
	DocumentID string                 `json:"document_id,omitempty"`
	ChunkIndex int                    `json:"chunk_index,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

func NewSearchService(db *database.DB, emb *EmbeddingService, pc *PineconeService, ns *NamespaceService) *SearchService {
	return &SearchService{db: db, embedding: emb, pinecone: pc, nsSvc: ns}
}

func (s *SearchService) HybridSearch(ctx context.Context, req SearchRequest) ([]SearchResult, error) {
	if req.TopK <= 0 {
		req.TopK = 5
	}
	if req.MinScore <= 0 {
		req.MinScore = 0.70
	}

	queryVec, err := s.embedding.EmbedText(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	// Resolve namespace names -> Pinecone NS strings
	var pineconeNSList []string
	if len(req.Namespaces) > 0 {
		for _, nsName := range req.Namespaces {
			rows, err := s.db.QueryContext(ctx, `SELECT pinecone_ns FROM namespaces WHERE workspace_id=$1 AND name=$2 AND is_active=true`, req.WorkspaceID, nsName)
			if err != nil {
				continue
			}
			for rows.Next() {
				var pns string
				_ = rows.Scan(&pns)
				pineconeNSList = append(pineconeNSList, pns)
			}
			rows.Close()
		}
	} else {
		// All active namespaces for workspace
		rows, _ := s.db.QueryContext(ctx, `SELECT pinecone_ns FROM namespaces WHERE workspace_id=$1 AND is_active=true`, req.WorkspaceID)
		if rows != nil {
			defer rows.Close()
			for rows.Next() {
				var pns string
				_ = rows.Scan(&pns)
				pineconeNSList = append(pineconeNSList, pns)
			}
		}
	}

	// --- Vector search (Pinecone) ---
	type ranked struct {
		result SearchResult
		rank   int // 1-indexed position
	}
	vectorRanked := make(map[string]ranked)
	vectorOrder := []string{}
	rank := 1
	for _, ns := range pineconeNSList {
		results, err := s.pinecone.QueryVectors(ctx, ns, queryVec, uint32(req.TopK*2), nil)
		if err != nil {
			continue
		}
		for _, r := range results {
			if r.Score < req.MinScore {
				continue
			}
			text, _ := r.Metadata["text"].(string)
			source, _ := r.Metadata["source"].(string)
			docID, _ := r.Metadata["document_id"].(string)
			chunkIdx := 0
			if ci, ok := r.Metadata["chunk_index"].(float64); ok {
				chunkIdx = int(ci)
			}
			vectorRanked[r.ID] = ranked{
				result: SearchResult{
					Text:       text,
					Score:      r.Score,
					Source:     source,
					DocumentID: docID,
					ChunkIndex: chunkIdx,
					Metadata:   r.Metadata,
				},
				rank: rank,
			}
			vectorOrder = append(vectorOrder, r.ID)
			rank++
		}
	}

	// --- BM25 via PostgreSQL full-text search ---
	bm25Ranked := make(map[string]ranked)
	bm25Order := []string{}
	ftsRows, err := s.db.QueryContext(ctx, `
        SELECT pinecone_id, text, ts_rank_cd(search_vector, plainto_tsquery('simple', $1)) as rank
        FROM knowledge_chunks
        WHERE workspace_id=$2 AND search_vector @@ plainto_tsquery('simple', $1)
        ORDER BY rank DESC LIMIT $3`,
		req.Query, req.WorkspaceID, req.TopK*2,
	)
	if err == nil {
		defer ftsRows.Close()
		bmsRank := 1
		for ftsRows.Next() {
			var pid, text string
			var score float32
			if err := ftsRows.Scan(&pid, &text, &score); err != nil {
				continue
			}
			bm25Ranked[pid] = ranked{result: SearchResult{Text: text, Score: score, Source: "doc"}, rank: bmsRank}
			bm25Order = append(bm25Order, pid)
			bmsRank++
		}
	}

	// --- Reciprocal Rank Fusion (k=60) ---
	const k = 60.0
	rrfScores := make(map[string]float64)
	rrfResults := make(map[string]SearchResult)

	for i, id := range vectorOrder {
		rrfScores[id] += 1.0 / (k + float64(i+1))
		rrfResults[id] = vectorRanked[id].result
	}
	for i, id := range bm25Order {
		rrfScores[id] += 1.0 / (k + float64(i+1))
		if _, exists := rrfResults[id]; !exists {
			rrfResults[id] = bm25Ranked[id].result
		}
	}

	type scored struct {
		id    string
		score float64
	}
	var all []scored
	for id, sc := range rrfScores {
		all = append(all, scored{id, sc})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].score > all[j].score })

	var out []SearchResult
	for _, item := range all {
		r := rrfResults[item.id]
		r.Score = float32(item.score)
		out = append(out, r)
		if len(out) >= req.TopK {
			break
		}
	}
	return out, nil
}
