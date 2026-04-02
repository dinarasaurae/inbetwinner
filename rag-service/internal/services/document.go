package services

import (
	"context"
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

	embeddings, err := s.embedding.EmbedBatch(ctx, texts)
	if err != nil {
		_, _ = s.db.ExecContext(ctx, `UPDATE knowledge_documents SET status='failed' WHERE id=$1`, docID)
		return nil, fmt.Errorf("embed: %w", err)
	}

	var vectors []VectorRecord
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

	if s.pinecone != nil {
		if err := s.pinecone.UpsertVectors(ctx, ns.PineconeNS, vectors); err != nil {
			return nil, fmt.Errorf("pinecone upsert: %w", err)
		}
	}

	_, _ = s.db.ExecContext(ctx, `UPDATE knowledge_documents SET status='indexed', chunk_count=$1 WHERE id=$2`, len(chunks), docID)
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

func (s *DocumentService) List(ctx context.Context, workspaceID, namespaceID uuid.UUID) ([]models.KnowledgeDocument, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT id, workspace_id, namespace_id, filename, content_type,
               chunk_size, chunk_count, status, embed_model,
               minio_key, file_size, created_at, updated_at
        FROM knowledge_documents WHERE workspace_id=$1 AND namespace_id=$2 ORDER BY created_at DESC`,
		workspaceID, namespaceID)
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

func chunkText(text string, chunkSize int) []string {
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return nil
	}
	paragraphs := strings.Split(text, "\n\n")
	var chunks []string
	current := ""
	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		if len(current)+len(para)+2 <= chunkSize {
			if current != "" {
				current += "\n\n"
			}
			current += para
		} else {
			if current != "" {
				chunks = append(chunks, current)
			}
			if len(para) > chunkSize {
				sentences := strings.Split(para, ". ")
				current = ""
				for _, sent := range sentences {
					sent = strings.TrimSpace(sent)
					if sent == "" {
						continue
					}
					if len(current)+len(sent)+2 <= chunkSize {
						if current != "" {
							current += ". "
						}
						current += sent
					} else {
						if current != "" {
							chunks = append(chunks, current)
						}
						current = sent
					}
				}
			} else {
				current = para
			}
		}
	}
	if current != "" {
		chunks = append(chunks, current)
	}
	return chunks
}
