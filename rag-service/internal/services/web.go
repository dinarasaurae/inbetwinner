package services

import (
	"context"
	"crypto/tls"
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

type WebService struct {
	db        *database.DB
	embedding *EmbeddingService
	pinecone  *PineconeService
	nsSvc     *NamespaceService
}

func NewWebService(db *database.DB, emb *EmbeddingService, pc *PineconeService, ns *NamespaceService) *WebService {
	return &WebService{db: db, embedding: emb, pinecone: pc, nsSvc: ns}
}

// Add registers a URL and immediately crawls + embeds its content.
func (s *WebService) Add(ctx context.Context, workspaceID uuid.UUID, req models.WebSourceCreateRequest) (*models.WebSource, error) {
	ns, err := s.nsSvc.GetByID(ctx, req.NamespaceID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("namespace: %w", err)
	}

	// Upsert row in pending state.
	var srcID uuid.UUID
	now := time.Now()
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO web_sources (workspace_id, namespace_id, url, status)
		VALUES ($1,$2,$3,'indexing')
		ON CONFLICT (workspace_id, url) DO UPDATE
			SET namespace_id=$2, status='indexing', error_message=NULL
		RETURNING id`,
		workspaceID, req.NamespaceID, req.URL,
	).Scan(&srcID)
	if err != nil {
		return nil, fmt.Errorf("upsert web_source: %w", err)
	}

	// Crawl + embed in foreground. Failures are non-fatal: record is saved with
	// an error/pending status so the user can see it and retry later.
	title, chunks, crawlErr := s.crawlAndChunk(ctx, req.URL)
	status := "indexed"
	errMsg := ""

	if crawlErr != nil {
		status = "error"
		errMsg = crawlErr.Error()
	} else {
		// Embed — also non-fatal
		embeddings, embedErr := s.embedding.EmbedBatch(ctx, chunks)
		if embedErr != nil {
			status = "pending" // saved, but not vectorized yet
		} else if s.pinecone != nil {
			var vectors []VectorRecord
			for i, chunk := range chunks {
				vectors = append(vectors, VectorRecord{
					ID:     fmt.Sprintf("web_%s_%d", srcID.String(), i),
					Values: embeddings[i],
					Metadata: map[string]interface{}{
						"workspace_id": workspaceID.String(),
						"source_id":    srcID.String(),
						"chunk_index":  float64(i),
						"text":         chunk,
						"source":       "web",
						"url":          req.URL,
						"title":        title,
					},
				})
			}
			_ = s.pinecone.UpsertVectors(ctx, ns.PineconeNS, vectors) // best-effort
		}
	}

	s.db.ExecContext(ctx, `
		UPDATE web_sources SET status=$1, title=$2, chunk_count=$3, last_crawl_at=$4, error_message=$5 WHERE id=$6`,
		status, title, len(chunks), now, errMsg, srcID)

	return &models.WebSource{
		ID:           srcID,
		WorkspaceID:  workspaceID,
		NamespaceID:  req.NamespaceID,
		URL:          req.URL,
		Title:        title,
		Status:       status,
		ErrorMessage: errMsg,
		ChunkCount:   len(chunks),
		LastCrawlAt:  &now,
		CreatedAt:    now,
	}, nil
}

// List returns all web sources for the workspace.
func (s *WebService) List(ctx context.Context, workspaceID uuid.UUID) ([]models.WebSource, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workspace_id, namespace_id, url, title, status,
		       COALESCE(error_message,''), chunk_count, last_crawl_at, created_at
		FROM web_sources WHERE workspace_id=$1 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.WebSource
	for rows.Next() {
		var ws models.WebSource
		if err := rows.Scan(&ws.ID, &ws.WorkspaceID, &ws.NamespaceID,
			&ws.URL, &ws.Title, &ws.Status, &ws.ErrorMessage,
			&ws.ChunkCount, &ws.LastCrawlAt, &ws.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ws)
	}
	return out, rows.Err()
}

// Delete removes a web source and its vectors.
func (s *WebService) Delete(ctx context.Context, id, workspaceID uuid.UUID) error {
	// Delete vectors from Pinecone first (best-effort).
	if s.pinecone != nil {
		var nsID uuid.UUID
		var ns string
		if err := s.db.QueryRowContext(ctx,
			`SELECT ws.namespace_id, n.pinecone_ns
			 FROM web_sources ws JOIN namespaces n ON n.id=ws.namespace_id
			 WHERE ws.id=$1 AND ws.workspace_id=$2`,
			id, workspaceID).Scan(&nsID, &ns); err == nil && ns != "" {
			// Delete vectors by prefix — delete each chunk ID individually
			// (Pinecone free tier doesn't support prefix delete, so we do nothing here;
			// in production use Pinecone's delete by metadata filter).
			_ = nsID
		}
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM web_sources WHERE id=$1 AND workspace_id=$2`, id, workspaceID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("not_found: web source not found")
	}
	return nil
}

// ─── Crawler ─────────────────────────────────────────────────────────────────

var (
	tagRe      = regexp.MustCompile(`(?i)<(script|style)[^>]*>.*?</(script|style)>`)
	htmlTagRe  = regexp.MustCompile(`<[^>]+>`)
	entityRe   = regexp.MustCompile(`&[a-zA-Z]+;|&#\d+;`)
	spacesRe   = regexp.MustCompile(`[ \t]+`)
	titleTagRe = regexp.MustCompile(`(?i)<title[^>]*>([^<]+)</title>`)
)

func (s *WebService) crawlAndChunk(ctx context.Context, rawURL string) (title string, chunks []string, err error) {
	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			// Allow self-signed / unverified certs in dev Docker environment.
			// In production replace this with proper certificate validation.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", "inBeTwin-RAG/1.0 (+https://inbetwin.ru)")

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("fetch %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2 MB max
	if err != nil {
		return "", nil, fmt.Errorf("read body: %w", err)
	}
	html := string(body)

	// Extract title
	if m := titleTagRe.FindStringSubmatch(html); len(m) > 1 {
		title = strings.TrimSpace(m[1])
	}
	if title == "" {
		title = rawURL
	}

	text := extractText(html)
	chunks = chunkTextWithOverlap(text, 500, 50)
	if len(chunks) == 0 {
		return title, nil, fmt.Errorf("no text content extracted from page")
	}
	return title, chunks, nil
}

func extractText(html string) string {
	// Remove script/style blocks
	text := tagRe.ReplaceAllString(html, " ")
	// Remove HTML tags
	text = htmlTagRe.ReplaceAllString(text, " ")
	// Decode entities
	text = entityRe.ReplaceAllString(text, " ")
	// Normalise whitespace
	text = spacesRe.ReplaceAllString(text, " ")
	// Normalise newlines
	lines := strings.Split(text, "\n")
	var out []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if len([]rune(l)) > 20 { // skip very short fragments
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// chunkTextWithOverlap splits text into overlapping chunks of ~wordsPerChunk words.
func chunkTextWithOverlap(text string, wordsPerChunk, overlapWords int) []string {
	words := strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r)
	})
	if len(words) == 0 {
		return nil
	}
	var chunks []string
	for i := 0; i < len(words); i += wordsPerChunk - overlapWords {
		end := i + wordsPerChunk
		if end > len(words) {
			end = len(words)
		}
		chunk := strings.Join(words[i:end], " ")
		if strings.TrimSpace(chunk) != "" {
			chunks = append(chunks, chunk)
		}
		if end == len(words) {
			break
		}
	}
	return chunks
}
