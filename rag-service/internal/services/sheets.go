package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dinarasaurae/inbetwin-rag-service/internal/database"
	"github.com/dinarasaurae/inbetwin-rag-service/internal/models"
	"github.com/google/uuid"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

type SheetsService struct {
	db        *database.DB
	embedding *EmbeddingService
	pinecone  *PineconeService
	nsSvc     *NamespaceService
}

func NewSheetsService(db *database.DB, emb *EmbeddingService, pc *PineconeService, ns *NamespaceService) *SheetsService {
	return &SheetsService{db: db, embedding: emb, pinecone: pc, nsSvc: ns}
}

func (s *SheetsService) SyncSheet(ctx context.Context, workspaceID uuid.UUID, req models.SheetsSyncRequest) (*models.KnowledgeTable, error) {
	ns, err := s.nsSvc.GetByID(ctx, req.NamespaceID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("namespace: %w", err)
	}
	svc, err := sheets.NewService(ctx, option.WithAPIKey(req.APIKey))
	if err != nil {
		return nil, fmt.Errorf("sheets client: %w", err)
	}
	sheetRange := req.SheetName + "!A:Z"
	if req.SheetName == "" {
		sheetRange = "Sheet1!A:Z"
	}
	resp, err := svc.Spreadsheets.Values.Get(req.SpreadsheetID, sheetRange).Do()
	if err != nil {
		return nil, fmt.Errorf("get sheet: %w", err)
	}
	if len(resp.Values) < 2 {
		return nil, fmt.Errorf("sheet has no data rows")
	}

	headers := make([]string, len(resp.Values[0]))
	for i, h := range resp.Values[0] {
		headers[i] = fmt.Sprintf("%v", h)
	}

	var rowTexts []string
	for _, row := range resp.Values[1:] {
		parts := make([]string, 0, len(headers))
		for i, h := range headers {
			val := ""
			if i < len(row) {
				val = fmt.Sprintf("%v", row[i])
			}
			if val != "" {
				parts = append(parts, h+": "+val)
			}
		}
		if len(parts) > 0 {
			rowTexts = append(rowTexts, strings.Join(parts, ", "))
		}
	}

	embeddings, err := s.embedding.EmbedBatch(ctx, rowTexts)
	if err != nil {
		return nil, fmt.Errorf("embed rows: %w", err)
	}

	var tableID uuid.UUID
	now := time.Now()
	err = s.db.QueryRowContext(ctx, `
        INSERT INTO knowledge_tables (workspace_id, namespace_id, name, spreadsheet_id, sheet_name, row_count, status, last_sync_at)
        VALUES ($1,$2,$3,$4,$5,$6,'indexed',$7)
        ON CONFLICT (workspace_id, spreadsheet_id, sheet_name) DO UPDATE
            SET row_count=$6, status='indexed', last_sync_at=$7
        RETURNING id`,
		workspaceID, req.NamespaceID, req.TableName, req.SpreadsheetID, req.SheetName, len(rowTexts), now,
	).Scan(&tableID)
	if err != nil {
		return nil, fmt.Errorf("upsert table: %w", err)
	}

	var vectors []VectorRecord
	for i, text := range rowTexts {
		pid := fmt.Sprintf("table_%s_%d", tableID.String(), i)
		vectors = append(vectors, VectorRecord{
			ID:     pid,
			Values: embeddings[i],
			Metadata: map[string]interface{}{
				"workspace_id": workspaceID.String(),
				"table_id":     tableID.String(),
				"row_index":    float64(i),
				"text":         text,
				"source":       "table",
			},
		})
	}
	if s.pinecone != nil {
		if err := s.pinecone.UpsertVectors(ctx, ns.PineconeNS, vectors); err != nil {
			return nil, fmt.Errorf("upsert vectors: %w", err)
		}
	}

	return &models.KnowledgeTable{
		ID:            tableID,
		WorkspaceID:   workspaceID,
		NamespaceID:   req.NamespaceID,
		Name:          req.TableName,
		SpreadsheetID: req.SpreadsheetID,
		SheetName:     req.SheetName,
		RowCount:      len(rowTexts),
		Status:        "indexed",
		LastSyncAt:    &now,
	}, nil
}
