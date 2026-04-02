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
	embedModel string
}

func NewDocumentService(db *database.DB, emb *EmbeddingService, pc *PineconeService, ns *NamespaceService, model string) *DocumentService {
	return &DocumentService{db: db, embedding: emb, pinecone: pc, nsSvc: ns, embedModel: model}
}

func (s *DocumentService) Create(ctx context.Context, workspaceID uuid.UUID, req models.CreateDocumentRequest) (*models.KnowledgeDocument, error) {
	if req.ChunkSize <= 0 {
		req.ChunkSize = 512
	}
	ns, err := s.nsSvc.GetByID(ctx, req.NamespaceID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("namespace not found: %w", err)
	}

	var docID uuid.UUID
	err = s.db.QueryRowContext(ctx, `
        INSERT INTO knowledge_documents (workspace_id, namespace_id, filename, content_type, content, chunk_size, status, embed_model)
        VALUES ($1,$2,$3,$4,$5,$6,'indexing',$7) RETURNING id`,
		workspaceID, req.NamespaceID, req.Filename, req.ContentType, req.Content, req.ChunkSize, s.embedModel,
	).Scan(&docID)
	if err != nil {
		return nil, fmt.Errorf("insert doc: %w", err)
	}

	chunks := chunkText(req.Content, req.ChunkSize)
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
		_, err = s.db.ExecContext(ctx, `
            INSERT INTO knowledge_chunks (document_id, workspace_id, chunk_index, text, token_count, pinecone_id)
            VALUES ($1,$2,$3,$4,$5,$6)`,
			docID, workspaceID, i, chunk, tokenCount, pid,
		)
		if err != nil {
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
				"filename":     req.Filename,
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
        SELECT id, workspace_id, namespace_id, filename, content_type, chunk_size, chunk_count, status, embed_model, created_at, updated_at
        FROM knowledge_documents WHERE id=$1 AND workspace_id=$2`, id, workspaceID,
	).Scan(&doc.ID, &doc.WorkspaceID, &doc.NamespaceID, &doc.Filename, &doc.ContentType, &doc.ChunkSize, &doc.ChunkCount, &doc.Status, &doc.EmbedModel, &doc.CreatedAt, &doc.UpdatedAt)
	return doc, err
}

func (s *DocumentService) List(ctx context.Context, workspaceID, namespaceID uuid.UUID) ([]models.KnowledgeDocument, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT id, workspace_id, namespace_id, filename, content_type, chunk_size, chunk_count, status, embed_model, created_at, updated_at
        FROM knowledge_documents WHERE workspace_id=$1 AND namespace_id=$2 ORDER BY created_at DESC`,
		workspaceID, namespaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.KnowledgeDocument
	for rows.Next() {
		var d models.KnowledgeDocument
		if err := rows.Scan(&d.ID, &d.WorkspaceID, &d.NamespaceID, &d.Filename, &d.ContentType, &d.ChunkSize, &d.ChunkCount, &d.Status, &d.EmbedModel, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func (s *DocumentService) Delete(ctx context.Context, id, workspaceID uuid.UUID) error {
	var nsID uuid.UUID
	err := s.db.QueryRowContext(ctx, `SELECT namespace_id FROM knowledge_documents WHERE id=$1 AND workspace_id=$2`, id, workspaceID).Scan(&nsID)
	if err != nil {
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
	_, err = s.db.ExecContext(ctx, `DELETE FROM knowledge_documents WHERE id=$1 AND workspace_id=$2`, id, workspaceID)
	return err
}

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
