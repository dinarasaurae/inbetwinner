package services

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/dinarasaurae/inbetwin-rag-service/internal/database"
	"github.com/dinarasaurae/inbetwin-rag-service/internal/models"
	"github.com/google/uuid"
)

type DocumentService struct {
	db         *database.DB
	embedding  *EmbeddingService
	pinecone   *PineconeService
	nsSvc      *NamespaceService
	storage    *StorageService
	embedModel string
}

func NewDocumentService(
	db *database.DB,
	emb *EmbeddingService,
	pc *PineconeService,
	ns *NamespaceService,
	storage *StorageService,
	model string,
) *DocumentService {
	return &DocumentService{
		db:         db,
		embedding:  emb,
		pinecone:   pc,
		nsSvc:      ns,
		storage:    storage,
		embedModel: model,
	}
}

// Upload accepts a raw file (from multipart), stores it in MinIO, extracts
// text and delegates to the standard Create pipeline.
func (s *DocumentService) Upload(ctx context.Context, workspaceID uuid.UUID, filename, contentType string, data []byte, namespaceID uuid.UUID, chunkSize int) (*models.KnowledgeDocument, error) {
	if chunkSize <= 0 {
		chunkSize = 512
	}

	// 1. Extract text from binary
	text, err := ExtractText(filename, contentType, data)
	if err != nil {
		return nil, fmt.Errorf("extract text from %q: %w", filename, err)
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("no text could be extracted from %q", filename)
	}

	// 2. Reserve a document ID so we can use it as part of the MinIO key
	docID := uuid.New()

	// 3. Upload original binary to MinIO (non-fatal if storage not configured)
	minioKey := ""
	if s.storage != nil {
		key := ObjectKey(workspaceID.String(), docID.String(), filename)
		minioKey, _ = s.storage.Upload(ctx, key, data, contentType)
	}

	// 4. Chunk, embed, index
	return s.createWithID(ctx, docID, workspaceID, namespaceID, filename, contentType, text, minioKey, int64(len(data)), chunkSize)
}

// Create handles the plain-text JSON ingestion path (no file upload).
func (s *DocumentService) Create(ctx context.Context, workspaceID uuid.UUID, req models.CreateDocumentRequest) (*models.KnowledgeDocument, error) {
	if req.ChunkSize <= 0 {
		req.ChunkSize = 512
	}
	return s.createWithID(ctx, uuid.New(), workspaceID, req.NamespaceID, req.Filename, req.ContentType, req.Content, "", 0, req.ChunkSize)
}

func (s *DocumentService) createWithID(
	ctx context.Context,
	docID, workspaceID, namespaceID uuid.UUID,
	filename, contentType, content, minioKey string,
	fileSize int64,
	chunkSize int,
) (*models.KnowledgeDocument, error) {
	ns, err := s.nsSvc.GetByID(ctx, namespaceID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("namespace not found: %w", err)
	}

	err = s.db.QueryRowContext(ctx, `
        INSERT INTO knowledge_documents
            (id, workspace_id, namespace_id, filename, content_type, content,
             chunk_size, status, embed_model, minio_key, file_size)
        VALUES ($1,$2,$3,$4,$5,$6,$7,'indexing',$8,$9,$10)
        RETURNING id`,
		docID, workspaceID, namespaceID, filename, contentType, content,
		chunkSize, s.embedModel, minioKey, fileSize,
	).Scan(&docID)
	if err != nil {
		return nil, fmt.Errorf("insert doc: %w", err)
	}

	chunks := chunkText(content, chunkSize)
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c
	}

	embeddings, embedErr := s.embedding.EmbedBatch(ctx, texts)

	// Save chunks to postgres for BM25 search regardless of embedding result
	for i, chunk := range chunks {
		pid := fmt.Sprintf("%s_%d", docID.String(), i)
		tokenCount := len(strings.Fields(chunk))
		if _, err = s.db.ExecContext(ctx, `
            INSERT INTO knowledge_chunks (document_id, workspace_id, chunk_index, text, token_count, pinecone_id)
            VALUES ($1,$2,$3,$4,$5,$6)`,
			docID, workspaceID, i, chunk, tokenCount, pid,
		); err != nil {
			return nil, fmt.Errorf("insert chunk: %w", err)
		}
	}

	// Upsert to Pinecone only if embedding succeeded
	if embedErr == nil && s.pinecone != nil {
		var vectors []VectorRecord
		for i, chunk := range chunks {
			pid := fmt.Sprintf("%s_%d", docID.String(), i)
			vectors = append(vectors, VectorRecord{
				ID:     pid,
				Values: embeddings[i],
				Metadata: map[string]interface{}{
					"workspace_id": workspaceID.String(),
					"document_id":  docID.String(),
					"chunk_index":  float64(i),
					"text":         chunk,
					"source":       "doc",
					"filename":     filename,
				},
			})
		}
		_ = s.pinecone.UpsertVectors(ctx, ns.PineconeNS, vectors) // best-effort
	}

	status := "indexed"
	if embedErr != nil {
		status = "pending" // saved, but not yet vectorized
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE knowledge_documents SET status=$1, chunk_count=$2 WHERE id=$3`, status, len(chunks), docID)
	return s.GetByID(ctx, docID, workspaceID)
}

func (s *DocumentService) GetByID(ctx context.Context, id, workspaceID uuid.UUID) (*models.KnowledgeDocument, error) {
	doc := &models.KnowledgeDocument{}
	err := s.db.QueryRowContext(ctx, `
        SELECT id, workspace_id, namespace_id, filename, content_type,
               chunk_size, chunk_count, status, embed_model,
               minio_key, file_size, created_at, updated_at
        FROM knowledge_documents WHERE id=$1 AND workspace_id=$2`,
		id, workspaceID,
	).Scan(
		&doc.ID, &doc.WorkspaceID, &doc.NamespaceID,
		&doc.Filename, &doc.ContentType,
		&doc.ChunkSize, &doc.ChunkCount,
		&doc.Status, &doc.EmbedModel,
		&doc.MinioKey, &doc.FileSize,
		&doc.CreatedAt, &doc.UpdatedAt,
	)
	return doc, err
}

func (s *DocumentService) List(ctx context.Context, workspaceID uuid.UUID, namespaceID *uuid.UUID) ([]models.KnowledgeDocument, error) {
	var rows *sql.Rows
	var err error
	if namespaceID != nil {
		rows, err = s.db.QueryContext(ctx, `
            SELECT id, workspace_id, namespace_id, filename, content_type,
                   chunk_size, chunk_count, status, embed_model,
                   minio_key, file_size, created_at, updated_at
            FROM knowledge_documents WHERE workspace_id=$1 AND namespace_id=$2 ORDER BY created_at DESC`,
			workspaceID, *namespaceID)
	} else {
		rows, err = s.db.QueryContext(ctx, `
            SELECT id, workspace_id, namespace_id, filename, content_type,
                   chunk_size, chunk_count, status, embed_model,
                   minio_key, file_size, created_at, updated_at
            FROM knowledge_documents WHERE workspace_id=$1 ORDER BY created_at DESC`,
			workspaceID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.KnowledgeDocument
	for rows.Next() {
		var d models.KnowledgeDocument
		if err := rows.Scan(
			&d.ID, &d.WorkspaceID, &d.NamespaceID,
			&d.Filename, &d.ContentType,
			&d.ChunkSize, &d.ChunkCount,
			&d.Status, &d.EmbedModel,
			&d.MinioKey, &d.FileSize,
			&d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func (s *DocumentService) Delete(ctx context.Context, id, workspaceID uuid.UUID) error {
	var nsID uuid.UUID
	var minioKey string
	if err := s.db.QueryRowContext(ctx,
		`SELECT namespace_id, minio_key FROM knowledge_documents WHERE id=$1 AND workspace_id=$2`,
		id, workspaceID,
	).Scan(&nsID, &minioKey); err != nil {
		return err
	}
	ns, err := s.nsSvc.GetByID(ctx, nsID, workspaceID)
	if err != nil {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT pinecone_id FROM knowledge_chunks WHERE document_id=$1`, id)
	if err == nil {
		defer rows.Close()
		var ids []string
		for rows.Next() {
			var pid string
			_ = rows.Scan(&pid)
			ids = append(ids, pid)
		}
		if s.pinecone != nil && len(ids) > 0 {
			_ = s.pinecone.DeleteVectors(ctx, ns.PineconeNS, ids)
		}
	}
	if minioKey != "" && s.storage != nil {
		s.storage.Delete(ctx, minioKey)
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM knowledge_documents WHERE id=$1 AND workspace_id=$2`, id, workspaceID)
	return err
}

// ── text chunking ─────────────────────────────────────────────────────────────

// chunkText splits text into overlapping chunks using a sliding-window sentence
// strategy. Overlap defaults to 20 % of chunkSize (min 50 chars) so that
// context is never lost at chunk boundaries.
//
// Algorithm:
//  1. Split text into sentences on ". ", "! ", "? ", "\n\n", "\n".
//  2. Accumulate sentences into a window until the window reaches chunkSize.
//  3. Save the window as a chunk, then slide forward by dropping sentences from
//     the front until what remains fits within the overlap budget.
//  4. The kept tail becomes the seed of the next chunk.
func chunkText(text string, chunkSize int) []string {
	overlap := chunkSize / 5 // 20 %
	if overlap < 50 {
		overlap = 0
	}
	return chunkDocText(text, chunkSize, overlap)
}

// chunkDocText is the document-specific chunker (character-based, sentence-aware).
// web.go uses a separate word-based chunkTextWithOverlap for HTML pages.
func chunkDocText(text string, chunkSize, overlap int) []string {
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return nil
	}

	sentences := splitSentences(text)

	// windowLen returns the total character length of a sentence slice including
	// single-space separators between items.
	windowLen := func(w []string) int {
		if len(w) == 0 {
			return 0
		}
		n := 0
		for _, s := range w {
			n += len(s)
		}
		n += len(w) - 1 // separators
		return n
	}

	joinWindow := func(w []string) string {
		return strings.Join(w, " ")
	}

	var chunks []string
	window := make([]string, 0, 16)

	for _, sent := range sentences {
		sent = strings.TrimSpace(sent)
		if sent == "" {
			continue
		}

		// If a single sentence is already larger than chunkSize, hard-split it.
		if len(sent) > chunkSize {
			// Flush current window first.
			if len(window) > 0 {
				chunks = append(chunks, joinWindow(window))
				window = window[:0]
			}
			// Emit the long sentence in chunkSize-sized slices.
			for len(sent) > chunkSize {
				chunks = append(chunks, sent[:chunkSize])
				sent = sent[chunkSize:]
			}
			if sent != "" {
				window = append(window, sent)
			}
			continue
		}

		// Would adding this sentence overflow the window?
		sep := 0
		if len(window) > 0 {
			sep = 1
		}
		if windowLen(window)+sep+len(sent) > chunkSize {
			// Save the current window.
			if len(window) > 0 {
				chunks = append(chunks, joinWindow(window))
			}
			// Slide: drop sentences from the front until the remaining tail
			// fits within the overlap budget (and keep at least one sentence).
			for len(window) > 1 && windowLen(window) > overlap {
				window = window[1:]
			}
			// If overlap is 0, clear entirely.
			if overlap == 0 {
				window = window[:0]
			}
			// Safety: if the overlap tail + new sentence still won't fit
			// (e.g. a single very long sentence dominates the tail), abandon
			// the tail so we never exceed chunkSize.
			sep = 0
			if len(window) > 0 {
				sep = 1
			}
			if windowLen(window)+sep+len(sent) > chunkSize {
				window = window[:0]
			}
		}

		window = append(window, sent)
	}

	// Flush the last window.
	if len(window) > 0 {
		last := joinWindow(window)
		// Avoid emitting a chunk identical to the previous one (can happen when
		// the last sentence fits exactly into an overlap tail).
		if len(chunks) == 0 || chunks[len(chunks)-1] != last {
			chunks = append(chunks, last)
		}
	}

	return chunks
}

// splitSentences breaks text into sentence-level units. It splits on common
// end-of-sentence punctuation followed by whitespace, and also on newlines, so
// that paragraph structure is preserved as natural boundaries.
func splitSentences(text string) []string {
	// Normalise CRLF and multiple blank lines.
	text = strings.ReplaceAll(text, "\r\n", "\n")

	var sentences []string
	current := strings.Builder{}

	runes := []rune(text)
	for i, r := range runes {
		current.WriteRune(r)

		switch r {
		case '\n':
			if s := strings.TrimSpace(current.String()); s != "" {
				sentences = append(sentences, s)
			}
			current.Reset()
		case '.', '!', '?':
			// Only split if followed by a space or end-of-text.
			next := i + 1
			if next >= len(runes) || runes[next] == ' ' || runes[next] == '\n' {
				if s := strings.TrimSpace(current.String()); s != "" {
					sentences = append(sentences, s)
				}
				current.Reset()
			}
		}
	}
	if s := strings.TrimSpace(current.String()); s != "" {
		sentences = append(sentences, s)
	}
	return sentences
}
