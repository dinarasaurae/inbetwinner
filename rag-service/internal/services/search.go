package services

import (
	"context"
	"log"
	"sort"
	"strings"

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
	// AccessMode filters namespaces when no explicit list is given.
	// "AUTO_QUERY" returns only automatically-queried namespaces (default for LLM pipeline).
	// "" or "ALL" returns all active namespaces.
	AccessMode string `json:"access_mode"`
}

type SearchResult struct {
	Text       string                 `json:"text"`
	Score      float32                `json:"score"`
	Source     string                 `json:"source"`
	DocumentID string                 `json:"document_id,omitempty"`
	ChunkIndex int                    `json:"chunk_index,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// buildORQuery converts a natural-language query into a PostgreSQL tsquery
// that matches rows containing ANY significant word.
//
// Rules:
//   - Tokenises on non-alphanumeric characters
//   - Keeps tokens ≥ 2 runes
//   - Drops pure-digit tokens (e.g. "3", "10") — they are ubiquitous in
//     dosage text ("по 3 капсулы", "3 раза в день") and cause false boosts
//   - Alphanumeric tokens like "д3", "q10", "b12" are kept as product identifiers
//
// Example: "расскажи про омега 3"  → "расскажи | про | омега"
// Example: "витамин д3 дозировка"  → "витамин | д3 | дозировка"
func buildORQuery(query string) string {
	var words []string
	current := strings.Builder{}

	isDigitRune := func(r rune) bool { return r >= '0' && r <= '9' }
	isAlpha := func(r rune) bool {
		return (r >= 'а' && r <= 'я') || r == 'ё' || (r >= 'a' && r <= 'z')
	}
	isAlphaNum := func(r rune) bool { return isAlpha(r) || isDigitRune(r) }

	flush := func() {
		w := current.String()
		current.Reset()
		if len([]rune(w)) < 2 {
			return
		}
		// Skip pure-digit tokens — too common in dosage instructions
		onlyDigits := true
		for _, r := range w {
			if !isDigitRune(r) {
				onlyDigits = false
				break
			}
		}
		if onlyDigits {
			return
		}
		words = append(words, w)
	}

	for _, r := range strings.ToLower(query) {
		if isAlphaNum(r) {
			current.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()

	if len(words) == 0 {
		return "''"
	}

	// Deduplicate preserving order
	seen := map[string]bool{}
	var unique []string
	for _, w := range words {
		if !seen[w] {
			seen[w] = true
			unique = append(unique, w)
		}
	}
	// Build the OR tsquery using russian config so stems match correctly.
	// e.g. "дозировке" → stem "дозировк" matches stored "дозировка"
	quotedTerms := make([]string, len(unique))
	for i, w := range unique {
		quotedTerms[i] = w
	}
	return strings.Join(quotedTerms, " | ")
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

	type ranked struct {
		result SearchResult
		rank   int
	}

	vectorRanked := make(map[string]ranked)
	vectorOrder := []string{}

	// --- Vector search (Pinecone) — non-fatal ---
	queryVec, embedErr := s.embedding.EmbedText(ctx, req.Query)
	if embedErr != nil {
		log.Printf("[search] embed query failed (skipping vector search): %v", embedErr)
	} else if s.pinecone != nil {
		// Resolve namespace names → Pinecone NS strings
		var pineconeNSList []string
		if len(req.Namespaces) > 0 {
			for _, nsName := range req.Namespaces {
				rows, err := s.db.QueryContext(ctx,
					`SELECT pinecone_ns FROM namespaces WHERE workspace_id=$1 AND name=$2 AND is_active=true`,
					req.WorkspaceID, nsName)
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
			// No explicit namespace list — apply access_mode filter if requested.
			query := `SELECT pinecone_ns FROM namespaces WHERE workspace_id=$1 AND is_active=true`
			if req.AccessMode == "AUTO_QUERY" {
				query += ` AND access_mode='AUTO_QUERY'`
			}
			rows, _ := s.db.QueryContext(ctx, query, req.WorkspaceID)
			if rows != nil {
				defer rows.Close()
				for rows.Next() {
					var pns string
					_ = rows.Scan(&pns)
					pineconeNSList = append(pineconeNSList, pns)
				}
			}
		}

		rank := 1
		for _, ns := range pineconeNSList {
			results, err := s.pinecone.QueryVectors(ctx, ns, queryVec, uint32(req.TopK*2), nil)
			if err != nil {
				log.Printf("[search] pinecone query ns=%s: %v", ns, err)
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
	}

	// --- BM25: document chunks (PostgreSQL FTS) ---
	bm25Ranked := make(map[string]ranked)
	bm25Order := []string{}

	// When access_mode filter is set and no explicit namespaces, restrict BM25
	// to chunks whose namespace has matching access_mode.
	// knowledge_chunks → knowledge_documents → namespaces
	chunkNSJoin := ""
	chunkNSWhere := ""
	if req.AccessMode == "AUTO_QUERY" && len(req.Namespaces) == 0 {
		chunkNSJoin = `JOIN knowledge_documents kd ON kd.id = knowledge_chunks.document_id
                       JOIN namespaces ns ON ns.id = kd.namespace_id`
		chunkNSWhere = `AND ns.access_mode = 'AUTO_QUERY' AND ns.is_active = true`
	}
	chunkSQL := `
        SELECT knowledge_chunks.pinecone_id, knowledge_chunks.text,
               ts_rank_cd(knowledge_chunks.search_vector, plainto_tsquery('russian', $1)) as rank
        FROM knowledge_chunks ` + chunkNSJoin + `
        WHERE knowledge_chunks.workspace_id=$2
          AND knowledge_chunks.search_vector @@ plainto_tsquery('russian', $1)
          ` + chunkNSWhere + `
        ORDER BY rank DESC LIMIT $3`
	ftsRows, err := s.db.QueryContext(ctx, chunkSQL,
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

	// --- BM25: table rows (PostgreSQL FTS on knowledge_table_rows) ---
	// Use OR-based tsquery so conversational queries ("расскажи про омега")
	// still match as long as at least one keyword is present in the row.
	tableRanked := make(map[string]ranked)
	tableOrder := []string{}
	orQuery := buildORQuery(req.Query)

	// Apply access_mode filter via knowledge_tables → namespaces join if needed.
	tableNSJoin := ""
	tableNSWhere := ""
	if req.AccessMode == "AUTO_QUERY" && len(req.Namespaces) == 0 {
		tableNSJoin = `JOIN namespaces ns ON ns.id = kt.namespace_id`
		tableNSWhere = `AND ns.access_mode = 'AUTO_QUERY' AND ns.is_active = true`
	}
	tableSQL := `
        SELECT ktr.id::text, ktr.text,
               ts_rank_cd(ktr.search_vector, to_tsquery('russian', $1)) as rank,
               kt.name as table_name
        FROM knowledge_table_rows ktr
        JOIN knowledge_tables kt ON kt.id = ktr.table_id ` + tableNSJoin + `
        WHERE ktr.workspace_id=$2
          AND ktr.search_vector @@ to_tsquery('russian', $1)
          ` + tableNSWhere + `
        ORDER BY rank DESC LIMIT $3`
	tableRows, err := s.db.QueryContext(ctx, tableSQL,
		orQuery, req.WorkspaceID, req.TopK*2,
	)
	if err == nil {
		defer tableRows.Close()
		tRank := 1
		for tableRows.Next() {
			var id, text, tableName string
			var score float32
			if err := tableRows.Scan(&id, &text, &score, &tableName); err != nil {
				continue
			}
			tableRanked[id] = ranked{
				result: SearchResult{
					Text:   text,
					Score:  score,
					Source: "table",
					Metadata: map[string]interface{}{
						"table_name": tableName,
					},
				},
				rank: tRank,
			}
			tableOrder = append(tableOrder, id)
			tRank++
		}
	} else {
		log.Printf("[search] table rows FTS query failed: %v", err)
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
	for i, id := range tableOrder {
		rrfScores[id] += 1.0 / (k + float64(i+1))
		if _, exists := rrfResults[id]; !exists {
			rrfResults[id] = tableRanked[id].result
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
