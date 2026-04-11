package services

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/dinarasaurae/inbetwin-rag-service/internal/database"
	"github.com/dinarasaurae/inbetwin-rag-service/internal/models"
	"github.com/google/uuid"
)

type SheetsService struct {
	db          *database.DB
	embedding   *EmbeddingService
	pinecone    *PineconeService
	nsSvc       *NamespaceService
	googleOAuth *GoogleOAuthService // kept for future use / OAuth sheets
}

func NewSheetsService(db *database.DB, emb *EmbeddingService, pc *PineconeService, ns *NamespaceService, googleOAuth *GoogleOAuthService) *SheetsService {
	return &SheetsService{db: db, embedding: emb, pinecone: pc, nsSvc: ns, googleOAuth: googleOAuth}
}

// parseGoogleSheetsURL extracts spreadsheetID and gid from a Google Sheets URL or bare ID.
// Accepts:
//   - Full URL: https://docs.google.com/spreadsheets/d/{id}/edit?gid={gid}
//   - Bare ID:  1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms
func parseGoogleSheetsURL(input string) (spreadsheetID, gid string) {
	re := regexp.MustCompile(`/spreadsheets/d/([a-zA-Z0-9-_]+)`)
	if m := re.FindStringSubmatch(input); len(m) > 1 {
		spreadsheetID = m[1]
	} else {
		// Assume bare ID
		spreadsheetID = strings.TrimSpace(input)
	}

	// Extract gid from ?gid=... or #gid=...
	gidRe := regexp.MustCompile(`[?&#]gid=(\d+)`)
	if m := gidRe.FindStringSubmatch(input); len(m) > 1 {
		gid = m[1]
	}
	return
}

// buildCSVExportURL converts spreadsheet ID + optional gid to a CSV export URL.
// Public Google Sheets support this without auth.
func buildCSVExportURL(spreadsheetID, gid string) string {
	base := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/export?format=csv", spreadsheetID)
	if gid != "" {
		return base + "&gid=" + gid
	}
	return base + "&gid=0"
}

// fetchCSV downloads the CSV export and parses rows. Max 1000 rows.
func fetchCSV(ctx context.Context, csvURL string) ([][]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, csvURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch csv: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google sheets returned status %d — make sure the sheet is public (File → Share → Anyone with link)", resp.StatusCode)
	}

	r := csv.NewReader(resp.Body)
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	var rows [][]string
	for i := 0; i < 1000; i++ {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // skip malformed rows
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (s *SheetsService) SyncSheet(ctx context.Context, workspaceID uuid.UUID, req models.SheetsSyncRequest) (*models.KnowledgeTable, error) {
	ns, err := s.nsSvc.GetByID(ctx, req.NamespaceID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("namespace: %w", err)
	}

	// Parse URL or bare ID
	spreadsheetID, gid := parseGoogleSheetsURL(req.SpreadsheetID)
	if spreadsheetID == "" {
		return nil, fmt.Errorf("invalid spreadsheet_id: provide a Google Sheets URL or bare ID")
	}

	csvURL := buildCSVExportURL(spreadsheetID, gid)

	rows, err := fetchCSV(ctx, csvURL)
	if err != nil {
		return nil, fmt.Errorf("get sheet: %w", err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("sheet has no data rows (found %d row(s))", len(rows))
	}

	headers := rows[0]

	var rowTexts []string
	for _, row := range rows[1:] {
		var parts []string
		for i, h := range headers {
			val := ""
			if i < len(row) {
				val = strings.TrimSpace(row[i])
			}
			if val != "" && h != "" {
				parts = append(parts, h+": "+val)
			}
		}
		if len(parts) > 0 {
			rowTexts = append(rowTexts, strings.Join(parts, ", "))
		}
	}

	if len(rowTexts) == 0 {
		return nil, fmt.Errorf("sheet has no non-empty data rows")
	}

	// Derive sheet_name for display (gid or "Sheet1")
	sheetName := req.SheetName
	if sheetName == "" {
		if gid != "" && gid != "0" {
			sheetName = "gid:" + gid
		} else {
			sheetName = "Sheet1"
		}
	}

	// Save the table record — non-fatal: works even without embeddings.
	status := "pending"
	var tableID uuid.UUID
	now := time.Now()
	err = s.db.QueryRowContext(ctx, `
        INSERT INTO knowledge_tables (workspace_id, namespace_id, name, spreadsheet_id, sheet_name, row_count, status, last_sync_at)
        VALUES ($1,$2,$3,$4,$5,$6,'pending',$7)
        ON CONFLICT (workspace_id, spreadsheet_id, sheet_name) DO UPDATE
            SET row_count=$6, status='pending', last_sync_at=$7
        RETURNING id`,
		workspaceID, req.NamespaceID, req.TableName, spreadsheetID, sheetName, len(rowTexts), now,
	).Scan(&tableID)
	if err != nil {
		return nil, fmt.Errorf("upsert table: %w", err)
	}

	// Embed and upsert to Pinecone — non-fatal.
	embeddings, embedErr := s.embedding.EmbedBatch(ctx, rowTexts)
	if embedErr == nil && s.pinecone != nil {
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
		if upsertErr := s.pinecone.UpsertVectors(ctx, ns.PineconeNS, vectors); upsertErr == nil {
			status = "indexed"
			_, _ = s.db.ExecContext(ctx,
				`UPDATE knowledge_tables SET status='indexed' WHERE id=$1`, tableID)
		}
	}

	return &models.KnowledgeTable{
		ID:            tableID,
		WorkspaceID:   workspaceID,
		NamespaceID:   req.NamespaceID,
		Name:          req.TableName,
		SpreadsheetID: spreadsheetID,
		SheetName:     sheetName,
		RowCount:      len(rowTexts),
		Status:        status,
		LastSyncAt:    &now,
	}, nil
}

// List returns all knowledge tables for the workspace.
func (s *SheetsService) List(ctx context.Context, workspaceID uuid.UUID) ([]models.KnowledgeTable, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workspace_id, namespace_id, name, spreadsheet_id, sheet_name,
		       row_count, status, last_sync_at, created_at
		FROM knowledge_tables WHERE workspace_id=$1 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.KnowledgeTable
	for rows.Next() {
		var t models.KnowledgeTable
		if err := rows.Scan(&t.ID, &t.WorkspaceID, &t.NamespaceID, &t.Name,
			&t.SpreadsheetID, &t.SheetName, &t.RowCount, &t.Status,
			&t.LastSyncAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Delete removes a knowledge table.
func (s *SheetsService) Delete(ctx context.Context, id, workspaceID uuid.UUID) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM knowledge_tables WHERE id=$1 AND workspace_id=$2`, id, workspaceID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("not_found: table not found")
	}
	return nil
}
