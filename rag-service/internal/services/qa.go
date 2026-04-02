package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/dinarasaurae/inbetwin-rag-service/internal/database"
	"github.com/dinarasaurae/inbetwin-rag-service/internal/models"
	"github.com/google/uuid"
)

type QAService struct {
	db        *database.DB
	embedding *EmbeddingService
	pinecone  *PineconeService
	nsSvc     *NamespaceService
}

func NewQAService(db *database.DB, emb *EmbeddingService, pc *PineconeService, ns *NamespaceService) *QAService {
	return &QAService{db: db, embedding: emb, pinecone: pc, nsSvc: ns}
}

func (s *QAService) Create(ctx context.Context, workspaceID uuid.UUID, req models.CreateQARequest) (*models.QAPair, error) {
	ns, err := s.nsSvc.GetByID(ctx, req.NamespaceID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("namespace: %w", err)
	}
	tagsJSON, _ := json.Marshal(req.Tags)
	vec, err := s.embedding.EmbedText(ctx, req.Question)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	qa := &models.QAPair{}
	err = s.db.QueryRowContext(ctx, `
        INSERT INTO qa_pairs (workspace_id, namespace_id, question, answer, tags, is_strict)
        VALUES ($1,$2,$3,$4,$5,$6)
        RETURNING id, workspace_id, namespace_id, question, answer, is_strict, created_at, updated_at`,
		workspaceID, req.NamespaceID, req.Question, req.Answer, tagsJSON, req.IsStrict,
	).Scan(&qa.ID, &qa.WorkspaceID, &qa.NamespaceID, &qa.Question, &qa.Answer, &qa.IsStrict, &qa.CreatedAt, &qa.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert qa: %w", err)
	}
	qa.Tags = req.Tags
	pid := fmt.Sprintf("qa_%s", qa.ID.String())
	qa.PineconeID = pid
	_, _ = s.db.ExecContext(ctx, `UPDATE qa_pairs SET pinecone_id=$1 WHERE id=$2`, pid, qa.ID)
	if s.pinecone != nil {
		_ = s.pinecone.UpsertVectors(ctx, ns.PineconeNS, []VectorRecord{{
			ID:     pid,
			Values: vec,
			Metadata: map[string]interface{}{
				"workspace_id": workspaceID.String(),
				"qa_id":        qa.ID.String(),
				"text":         req.Question + "\n" + req.Answer,
				"source":       "qa",
				"question":     req.Question,
				"answer":       req.Answer,
			},
		}})
	}
	return qa, nil
}

func (s *QAService) List(ctx context.Context, workspaceID, namespaceID uuid.UUID) ([]models.QAPair, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT id, workspace_id, namespace_id, question, answer, tags, is_strict, pinecone_id, created_at, updated_at
        FROM qa_pairs WHERE workspace_id=$1 AND namespace_id=$2 ORDER BY created_at DESC`,
		workspaceID, namespaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.QAPair
	for rows.Next() {
		var qa models.QAPair
		var tagsJSON []byte
		var pid sql.NullString
		if err := rows.Scan(&qa.ID, &qa.WorkspaceID, &qa.NamespaceID, &qa.Question, &qa.Answer, &tagsJSON, &qa.IsStrict, &pid, &qa.CreatedAt, &qa.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(tagsJSON, &qa.Tags)
		qa.PineconeID = pid.String
		out = append(out, qa)
	}
	return out, nil
}

func (s *QAService) Delete(ctx context.Context, id, workspaceID uuid.UUID) error {
	var nsID uuid.UUID
	var pid sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT namespace_id, pinecone_id FROM qa_pairs WHERE id=$1 AND workspace_id=$2`, id, workspaceID).Scan(&nsID, &pid)
	if err != nil {
		return err
	}
	if s.pinecone != nil && pid.Valid {
		ns, _ := s.nsSvc.GetByID(ctx, nsID, workspaceID)
		if ns != nil {
			_ = s.pinecone.DeleteVectors(ctx, ns.PineconeNS, []string{pid.String})
		}
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM qa_pairs WHERE id=$1 AND workspace_id=$2`, id, workspaceID)
	return err
}
