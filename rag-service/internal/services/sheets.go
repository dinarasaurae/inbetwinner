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
	"unicode"

	"github.com/dinarasaurae/inbetwin-rag-service/internal/database"
	"github.com/dinarasaurae/inbetwin-rag-service/internal/models"
	"github.com/google/uuid"
)

// ─── Column type classification ───────────────────────────────────────────────

type columnType string

const (
	colID          columnType = "id"
	colName        columnType = "name"
	colProperty    columnType = "property"
	colDescription columnType = "description"
	colCategory    columnType = "category"
	colDate        columnType = "date"
	colNumeric     columnType = "numeric"
	colEmpty       columnType = "empty"
)

// ─── Service ──────────────────────────────────────────────────────────────────

type SheetsService struct {
	db          *database.DB
	embedding   *EmbeddingService
	pinecone    *PineconeService
	nsSvc       *NamespaceService
	googleOAuth *GoogleOAuthService
}

func NewSheetsService(db *database.DB, emb *EmbeddingService, pc *PineconeService, ns *NamespaceService, googleOAuth *GoogleOAuthService) *SheetsService {
	return &SheetsService{db: db, embedding: emb, pinecone: pc, nsSvc: ns, googleOAuth: googleOAuth}
}

// ─── URL parsing ──────────────────────────────────────────────────────────────

func parseGoogleSheetsURL(input string) (spreadsheetID, gid string) {
	re := regexp.MustCompile(`/spreadsheets/d/([a-zA-Z0-9-_]+)`)
	if m := re.FindStringSubmatch(input); len(m) > 1 {
		spreadsheetID = m[1]
	} else {
		spreadsheetID = strings.TrimSpace(input)
	}
	gidRe := regexp.MustCompile(`[?&#]gid=(\d+)`)
	if m := gidRe.FindStringSubmatch(input); len(m) > 1 {
		gid = m[1]
	}
	return
}

func buildCSVExportURL(spreadsheetID, gid string) string {
	base := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/export?format=csv", spreadsheetID)
	if gid != "" {
		return base + "&gid=" + gid
	}
	return base + "&gid=0"
}

// ─── CSV fetch ────────────────────────────────────────────────────────────────

func fetchCSVMatrix(ctx context.Context, csvURL string) ([][]string, error) {
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
		return nil, fmt.Errorf("google sheets returned %d — make sure the sheet is public (File → Share → Anyone with link)", resp.StatusCode)
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
			continue
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// ─── Smart table parsing (ported from reference TypeScript impl) ───────────────

// cleanTableData removes completely empty rows.
func cleanTableData(data [][]string) [][]string {
	var out [][]string
	for _, row := range data {
		for _, cell := range row {
			if strings.TrimSpace(cell) != "" {
				out = append(out, row)
				break
			}
		}
	}
	return out
}

// transposeMatrix swaps rows and columns.
func transposeMatrix(m [][]string) [][]string {
	if len(m) == 0 {
		return nil
	}
	cols := len(m[0])
	for _, row := range m {
		if len(row) > cols {
			cols = len(row)
		}
	}
	out := make([][]string, cols)
	for c := 0; c < cols; c++ {
		out[c] = make([]string, len(m))
		for r, row := range m {
			if c < len(row) {
				out[c][r] = row[c]
			}
		}
	}
	return out
}

// detectDataOrientation decides rows vs columns using the reference heuristic.
func detectDataOrientation(data [][]string) string {
	if len(data) == 0 {
		return "rows"
	}
	rowCount := len(data)
	colCount := len(data[0])

	if float64(rowCount) > float64(colCount)*1.5 {
		return "rows"
	}
	if float64(colCount) > float64(rowCount)*2 {
		return "columns"
	}
	// borderline: compare uniqueness of first row vs first column
	firstRow := make([]string, len(data[0]))
	copy(firstRow, data[0])
	firstCol := make([]string, len(data))
	for i, row := range data {
		if len(row) > 0 {
			firstCol[i] = row[0]
		}
	}
	if analyzeDataVariety(firstRow) > analyzeDataVariety(firstCol) {
		return "rows"
	}
	return "columns"
}

func analyzeDataVariety(cells []string) float64 {
	unique := map[string]struct{}{}
	total := 0
	for _, c := range cells {
		total++
		if t := strings.TrimSpace(c); t != "" {
			unique[strings.ToLower(t)] = struct{}{}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(len(unique)) / float64(total) * 100
}

// findHeaderRow returns the index of the row that looks most like a header
// (short text values, not pure numbers) within the first 8 rows.
func findHeaderRow(data [][]string) int {
	maxRows := 8
	if len(data) < maxRows {
		maxRows = len(data)
	}
	best, bestCount := 0, 0
	hasLetter := regexp.MustCompile(`[а-яА-Яa-zA-Z]`)
	isNumber := regexp.MustCompile(`^\d+$`)
	for i := 0; i < maxRows; i++ {
		count := 0
		for _, cell := range data[i] {
			s := strings.TrimSpace(cell)
			if len(s) >= 3 && len(s) <= 50 && !isNumber.MatchString(s) && hasLetter.MatchString(s) {
				count++
			}
		}
		if count > bestCount {
			bestCount = count
			best = i
		}
	}
	return best
}

// detectValueType classifies a single cell value.
func detectValueType(v string) string {
	s := strings.TrimSpace(v)
	if s == "" {
		return "empty"
	}
	if regexp.MustCompile(`^\d+$`).MatchString(s) {
		return "integer"
	}
	if regexp.MustCompile(`^\d+[.,]\d+$`).MatchString(s) {
		return "decimal"
	}
	if regexp.MustCompile(`^\d{1,2}[-./]\d{1,2}[-./]\d{2,4}$`).MatchString(s) {
		return "date"
	}
	if len([]rune(s)) > 100 {
		return "long_text"
	}
	if len([]rune(s)) < 10 {
		return "short_text"
	}
	return "medium_text"
}

// detectColumnType determines the semantic type of a column from its data.
func detectColumnType(columnData []string) columnType {
	sampleSize := 20
	if len(columnData) < sampleSize {
		sampleSize = len(columnData)
	}
	sample := columnData[:sampleSize]

	typeCounts := map[string]int{}
	for _, cell := range sample {
		typeCounts[detectValueType(cell)]++
	}

	dominant := ""
	maxCount := 0
	for t, c := range typeCounts {
		if c > maxCount {
			maxCount = c
			dominant = t
		}
	}

	switch dominant {
	case "integer", "decimal":
		return colNumeric
	case "date":
		return colDate
	case "long_text":
		return colDescription
	case "empty":
		return colEmpty
	}

	// Text analysis
	totalLen := 0
	unique := map[string]struct{}{}
	for _, cell := range sample {
		totalLen += len([]rune(cell))
		unique[cell] = struct{}{}
	}
	avgLen := float64(totalLen) / float64(len(sample))
	uniqueness := float64(len(unique)) / float64(len(sample))

	if uniqueness < 0.7 && avgLen < 50 {
		return colCategory
	}
	if uniqueness > 0.9 && avgLen < 20 {
		return colID
	}
	if uniqueness > 0.8 && avgLen < 50 {
		return colName
	}
	return colProperty
}

// analyzeColumnTypes returns the type of each column.
func analyzeColumnTypes(data [][]string, headerRowIndex int) []columnType {
	if len(data) <= headerRowIndex {
		return nil
	}
	colCount := len(data[headerRowIndex])
	dataRows := data[headerRowIndex+1:]
	types := make([]columnType, colCount)

	for c := 0; c < colCount; c++ {
		var colData []string
		for _, row := range dataRows {
			if c < len(row) {
				if v := strings.TrimSpace(row[c]); v != "" {
					colData = append(colData, v)
				}
			}
		}
		if len(colData) == 0 {
			types[c] = colEmpty
		} else {
			types[c] = detectColumnType(colData)
		}
	}
	return types
}

// generateSmartKeys normalises header names into snake_case keys.
func generateSmartKeys(headers []string, types []columnType) []string {
	keys := make([]string, len(headers))
	allowedRune := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' '
	}
	for i, h := range headers {
		clean := strings.TrimSpace(h)
		if clean == "" {
			keys[i] = fmt.Sprintf("column_%d", i+1)
			continue
		}
		if len([]rune(clean)) > 50 {
			words := strings.Fields(clean)
			if len(words) > 3 {
				words = words[:3]
			}
			clean = strings.Join(words, " ")
		}
		var b strings.Builder
		for _, r := range strings.ToLower(clean) {
			if allowedRune(r) {
				b.WriteRune(r)
			} else {
				b.WriteRune(' ')
			}
		}
		key := strings.Join(strings.Fields(b.String()), "_")
		key = strings.Trim(key, "_")
		if len([]rune(key)) > 40 {
			key = string([]rune(key)[:40])
		}
		if key == "" {
			key = fmt.Sprintf("column_%d", i+1)
		}
		keys[i] = key
		_ = types // prefix disabled (matches reference: getTypePrefix returns "")
	}
	return keys
}

// parseTableWithSmartAnalysis is the full port of the reference algorithm.
// Returns rows as "key: value, key: value" strings ready for embedding.
func parseTableWithSmartAnalysis(rawData [][]string) ([]string, error) {
	if len(rawData) == 0 {
		return nil, fmt.Errorf("empty table")
	}

	// 1. Clean empty rows
	data := cleanTableData(rawData)
	if len(data) == 0 {
		return nil, fmt.Errorf("table has only empty rows")
	}

	// 2. Detect orientation; transpose if column-oriented
	orientation := detectDataOrientation(data)
	if orientation == "columns" {
		data = transposeMatrix(data)
	}

	// 3. Find header row
	headerRowIndex := findHeaderRow(data)

	// 4. Analyse column types
	colTypes := analyzeColumnTypes(data, headerRowIndex)

	// 5. Generate smart keys from headers
	headers := data[headerRowIndex]
	smartKeys := generateSmartKeys(headers, colTypes)

	// 6. Build row text strings
	dataRows := data[headerRowIndex+1:]
	var rowTexts []string
	for _, row := range dataRows {
		var parts []string
		for i, key := range smartKeys {
			val := ""
			if i < len(row) {
				val = strings.TrimSpace(row[i])
			}
			if val != "" && key != "" {
				parts = append(parts, key+": "+val)
			}
		}
		if len(parts) > 0 {
			rowTexts = append(rowTexts, strings.Join(parts, ", "))
		}
	}
	return rowTexts, nil
}

// ─── Main sync logic ──────────────────────────────────────────────────────────

func (s *SheetsService) SyncSheet(ctx context.Context, workspaceID uuid.UUID, req models.SheetsSyncRequest) (*models.KnowledgeTable, error) {
	ns, err := s.nsSvc.GetByID(ctx, req.NamespaceID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("namespace: %w", err)
	}

	spreadsheetID, gid := parseGoogleSheetsURL(req.SpreadsheetID)
	if spreadsheetID == "" {
		return nil, fmt.Errorf("invalid spreadsheet_id: provide a Google Sheets URL or bare ID")
	}

	csvURL := buildCSVExportURL(spreadsheetID, gid)
	rawData, err := fetchCSVMatrix(ctx, csvURL)
	if err != nil {
		return nil, fmt.Errorf("get sheet: %w", err)
	}
	if len(rawData) < 2 {
		return nil, fmt.Errorf("sheet has no data rows (got %d rows total)", len(rawData))
	}

	rowTexts, err := parseTableWithSmartAnalysis(rawData)
	if err != nil {
		return nil, fmt.Errorf("parse table: %w", err)
	}
	if len(rowTexts) == 0 {
		return nil, fmt.Errorf("sheet has no non-empty data rows after parsing")
	}

	// Derive sheet_name for display
	sheetName := req.SheetName
	if sheetName == "" {
		if gid != "" && gid != "0" {
			sheetName = "gid:" + gid
		} else {
			sheetName = "Sheet1"
		}
	}

	// Save to DB — non-fatal: works without embeddings
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

	// Persist row texts for BM25 search (works without Pinecone).
	_, _ = s.db.ExecContext(ctx, `DELETE FROM knowledge_table_rows WHERE table_id=$1`, tableID)
	for i, text := range rowTexts {
		_, _ = s.db.ExecContext(ctx,
			`INSERT INTO knowledge_table_rows (table_id, workspace_id, row_index, text, pinecone_id)
             VALUES ($1,$2,$3,$4,'')`,
			tableID, workspaceID, i, text,
		)
	}

	// Embed + upsert to Pinecone — non-fatal
	embeddings, embedErr := s.embedding.EmbedBatch(ctx, rowTexts)
	if embedErr == nil && s.pinecone != nil {
		var vectors []VectorRecord
		for i, text := range rowTexts {
			vectors = append(vectors, VectorRecord{
				ID:     fmt.Sprintf("table_%s_%d", tableID.String(), i),
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
			_, _ = s.db.ExecContext(ctx, `UPDATE knowledge_tables SET status='indexed' WHERE id=$1`, tableID)
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

// ─── List / Delete ────────────────────────────────────────────────────────────

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
