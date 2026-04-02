package services

import (
	"context"
	"fmt"

	"github.com/dinarasaurae/inbetwin-rag-service/internal/database"
	"github.com/dinarasaurae/inbetwin-rag-service/internal/models"
	"github.com/google/uuid"
)

type NamespaceService struct {
	db       *database.DB
	pinecone *PineconeService
}

func NewNamespaceService(db *database.DB, pinecone *PineconeService) *NamespaceService {
	return &NamespaceService{db: db, pinecone: pinecone}
}

func (s *NamespaceService) Create(ctx context.Context, workspaceID uuid.UUID, req models.CreateNamespaceRequest) (*models.Namespace, error) {
	if req.Type == "" {
		req.Type = "doc"
	}
	if req.Scope == "" {
		req.Scope = "workspace"
	}
	shortID := workspaceID.String()[:8]
	pineconeNS := fmt.Sprintf("%s_%s_%s", req.Type, req.Name, shortID)

	ns := &models.Namespace{}
	err := s.db.QueryRowContext(ctx, `
        INSERT INTO namespaces (workspace_id, name, type, scope, pinecone_ns, description)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING id, workspace_id, name, type, scope, pinecone_ns, description, is_active, created_at, updated_at`,
		workspaceID, req.Name, req.Type, req.Scope, pineconeNS, req.Description,
	).Scan(&ns.ID, &ns.WorkspaceID, &ns.Name, &ns.Type, &ns.Scope, &ns.PineconeNS, &ns.Description, &ns.IsActive, &ns.CreatedAt, &ns.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert namespace: %w", err)
	}
	return ns, nil
}

func (s *NamespaceService) List(ctx context.Context, workspaceID uuid.UUID) ([]models.Namespace, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT id, workspace_id, name, type, scope, pinecone_ns, description, is_active, created_at, updated_at
        FROM namespaces WHERE workspace_id=$1 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Namespace
	for rows.Next() {
		var ns models.Namespace
		if err := rows.Scan(&ns.ID, &ns.WorkspaceID, &ns.Name, &ns.Type, &ns.Scope, &ns.PineconeNS, &ns.Description, &ns.IsActive, &ns.CreatedAt, &ns.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, ns)
	}
	return out, nil
}

func (s *NamespaceService) GetByID(ctx context.Context, id, workspaceID uuid.UUID) (*models.Namespace, error) {
	ns := &models.Namespace{}
	err := s.db.QueryRowContext(ctx, `
        SELECT id, workspace_id, name, type, scope, pinecone_ns, description, is_active, created_at, updated_at
        FROM namespaces WHERE id=$1 AND workspace_id=$2`, id, workspaceID,
	).Scan(&ns.ID, &ns.WorkspaceID, &ns.Name, &ns.Type, &ns.Scope, &ns.PineconeNS, &ns.Description, &ns.IsActive, &ns.CreatedAt, &ns.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return ns, nil
}

func (s *NamespaceService) Delete(ctx context.Context, id, workspaceID uuid.UUID) error {
	ns, err := s.GetByID(ctx, id, workspaceID)
	if err != nil {
		return err
	}
	if s.pinecone != nil {
		_ = s.pinecone.DeleteNamespace(ctx, ns.PineconeNS)
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM namespaces WHERE id=$1 AND workspace_id=$2`, id, workspaceID)
	return err
}
