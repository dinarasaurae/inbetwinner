package amocrm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/dinarasaurae/inbetwin-llm-service/internal/database"
	"github.com/dinarasaurae/inbetwin-llm-service/internal/models"
	"github.com/google/uuid"
)

const (
	oauthBaseURL    = "https://www.kommo.com/oauth"
	cacheTTL        = 5 * time.Minute
	maxRetryAttempts = 3
)

// ── Service ───────────────────────────────────────────────────────────────────

type Service struct {
	db           *database.DB
	clientID     string
	clientSecret string
	redirectURL  string
	httpClient   *http.Client

	// in-memory token cache — avoids a DB round-trip on every tool call
	tokenMu    sync.RWMutex
	tokenCache map[uuid.UUID]cachedToken
}

type cachedToken struct {
	subdomain   string
	accessToken string
	expiresAt   time.Time
}

// ── OAuth token types ─────────────────────────────────────────────────────────

type tokenResponse struct {
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// ── AmoCRM API request/response types ────────────────────────────────────────

type CreateLeadParams struct {
	Name         string `json:"name"`
	Price        int    `json:"price,omitempty"`
	PipelineID   int64  `json:"pipeline_id,omitempty"`
	ContactName  string `json:"contact_name,omitempty"`
	ContactPhone string `json:"contact_phone,omitempty"`
	ContactEmail string `json:"contact_email,omitempty"`
	Notes        string `json:"notes,omitempty"`
}

type CreateTaskParams struct {
	Text       string `json:"text"`
	LeadID     int64  `json:"lead_id,omitempty"`
	DueDate    string `json:"due_date,omitempty"` // ISO 8601
	TaskTypeID int    `json:"task_type_id,omitempty"`
}

type AddNoteParams struct {
	LeadID int64  `json:"lead_id"`
	Text   string `json:"text"`
}

type LeadResult struct {
	ID   int64  `json:"id"`
	URL  string `json:"url"`
	Name string `json:"name"`
}

type TaskResult struct {
	ID   int64  `json:"id"`
	Text string `json:"text"`
}

type NoteResult struct {
	ID     int64  `json:"id"`
	LeadID int64  `json:"lead_id"`
}

type Pipeline struct {
	ID       int64            `json:"id"`
	Name     string           `json:"name"`
	IsMain   bool             `json:"is_main"`
	Statuses []PipelineStatus `json:"statuses,omitempty"`
}

type PipelineStatus struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ── Constructor ───────────────────────────────────────────────────────────────

func NewService(db *database.DB, clientID, clientSecret, redirectURL string) *Service {
	return &Service{
		db:           db,
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURL:  redirectURL,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
		tokenCache:   make(map[uuid.UUID]cachedToken),
	}
}

// ── OAuth helpers ─────────────────────────────────────────────────────────────

// GetAuthURL returns the Kommo OAuth consent URL.
// state encodes the workspaceID so we can store the token after callback.
func (s *Service) GetAuthURL(workspaceID uuid.UUID) string {
	return fmt.Sprintf(
		"%s?client_id=%s&state=%s&redirect_uri=%s&response_type=code&mode=post_message",
		oauthBaseURL, s.clientID, workspaceID.String(), s.redirectURL,
	)
}

// HandleCallback exchanges the code for tokens and stores the integration.
// subdomain comes from the "referer" query param that Kommo appends to the callback URL.
func (s *Service) HandleCallback(ctx context.Context, code, state, subdomain string) (*models.AmoCRMIntegration, error) {
	if subdomain == "" {
		return nil, fmt.Errorf("missing subdomain (referer param)")
	}
	workspaceID, err := uuid.Parse(state)
	if err != nil {
		return nil, fmt.Errorf("invalid state: %w", err)
	}

	tok, err := s.exchangeCode(ctx, subdomain, code)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}

	accountID, email := s.fetchAccountInfo(ctx, subdomain, tok.AccessToken)
	expiry := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)

	integration := &models.AmoCRMIntegration{}
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO amocrm_integrations
			(workspace_id, subdomain, access_token, refresh_token, token_expiry, account_id, email)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (workspace_id) DO UPDATE SET
			subdomain=EXCLUDED.subdomain,
			access_token=EXCLUDED.access_token,
			refresh_token=EXCLUDED.refresh_token,
			token_expiry=EXCLUDED.token_expiry,
			account_id=EXCLUDED.account_id,
			email=EXCLUDED.email,
			is_active=true,
			updated_at=NOW()
		RETURNING id, workspace_id, subdomain, account_id, email, is_active, token_expiry, created_at, updated_at`,
		workspaceID, subdomain, tok.AccessToken, tok.RefreshToken, expiry, accountID, email,
	).Scan(&integration.ID, &integration.WorkspaceID, &integration.Subdomain,
		&integration.AccountID, &integration.Email, &integration.IsActive,
		&integration.TokenExpiry, &integration.CreatedAt, &integration.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("save integration: %w", err)
	}

	// Warm the cache with the freshly-issued token
	s.setCachedToken(workspaceID, subdomain, tok.AccessToken, expiry)
	return integration, nil
}

func (s *Service) GetIntegration(ctx context.Context, workspaceID uuid.UUID) (*models.AmoCRMIntegration, error) {
	i := &models.AmoCRMIntegration{}
	err := s.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, subdomain, account_id, email, is_active, token_expiry, created_at, updated_at
		FROM amocrm_integrations WHERE workspace_id=$1`, workspaceID,
	).Scan(&i.ID, &i.WorkspaceID, &i.Subdomain, &i.AccountID, &i.Email,
		&i.IsActive, &i.TokenExpiry, &i.CreatedAt, &i.UpdatedAt)
	return i, err
}

// ── Token cache helpers ───────────────────────────────────────────────────────

func (s *Service) getCachedToken(workspaceID uuid.UUID) (subdomain, accessToken string, ok bool) {
	s.tokenMu.RLock()
	defer s.tokenMu.RUnlock()
	c, found := s.tokenCache[workspaceID]
	if !found || time.Now().After(c.expiresAt) {
		return "", "", false
	}
	return c.subdomain, c.accessToken, true
}

func (s *Service) setCachedToken(workspaceID uuid.UUID, subdomain, accessToken string, tokenExpiry time.Time) {
	// Cache until min(tokenExpiry - 5min, now + cacheTTL)
	cacheExp := time.Now().Add(cacheTTL)
	naturalExp := tokenExpiry.Add(-5 * time.Minute)
	if naturalExp.Before(cacheExp) {
		cacheExp = naturalExp
	}
	if cacheExp.Before(time.Now()) {
		// Token is effectively expired — don't cache
		return
	}
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	s.tokenCache[workspaceID] = cachedToken{
		subdomain:   subdomain,
		accessToken: accessToken,
		expiresAt:   cacheExp,
	}
}

func (s *Service) invalidateCachedToken(workspaceID uuid.UUID) {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	delete(s.tokenCache, workspaceID)
}

// ── Token management ──────────────────────────────────────────────────────────

func (s *Service) getAccessToken(ctx context.Context, workspaceID uuid.UUID) (subdomain, accessToken string, err error) {
	// Fast path: serve from cache
	if sd, tok, ok := s.getCachedToken(workspaceID); ok {
		return sd, tok, nil
	}

	// Slow path: query DB
	var refreshToken string
	var expiry time.Time
	err = s.db.QueryRowContext(ctx,
		`SELECT subdomain, access_token, refresh_token, token_expiry
		 FROM amocrm_integrations WHERE workspace_id=$1 AND is_active=true`,
		workspaceID).Scan(&subdomain, &accessToken, &refreshToken, &expiry)
	if err != nil {
		return "", "", fmt.Errorf("no amocrm integration: %w", err)
	}

	// Refresh if token expires within 5 minutes
	if time.Until(expiry) < 5*time.Minute {
		tok, rerr := s.refreshToken(ctx, subdomain, refreshToken)
		if rerr == nil {
			newExpiry := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
			_, _ = s.db.ExecContext(ctx,
				`UPDATE amocrm_integrations SET access_token=$1, refresh_token=$2, token_expiry=$3, updated_at=NOW()
				 WHERE workspace_id=$4`,
				tok.AccessToken, tok.RefreshToken, newExpiry, workspaceID)
			accessToken = tok.AccessToken
			expiry = newExpiry
		}
		// If refresh fails, continue with the old token — it may still be valid.
	}

	s.setCachedToken(workspaceID, subdomain, accessToken, expiry)
	return subdomain, accessToken, nil
}

func (s *Service) exchangeCode(ctx context.Context, subdomain, code string) (*tokenResponse, error) {
	body, _ := json.Marshal(map[string]string{
		"client_id":     s.clientID,
		"client_secret": s.clientSecret,
		"grant_type":    "authorization_code",
		"code":          code,
		"redirect_uri":  s.redirectURL,
	})
	return s.doTokenRequest(ctx, subdomain, body)
}

func (s *Service) refreshToken(ctx context.Context, subdomain, refreshToken string) (*tokenResponse, error) {
	body, _ := json.Marshal(map[string]string{
		"client_id":     s.clientID,
		"client_secret": s.clientSecret,
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"redirect_uri":  s.redirectURL,
	})
	return s.doTokenRequest(ctx, subdomain, body)
}

func (s *Service) doTokenRequest(ctx context.Context, subdomain string, body []byte) (*tokenResponse, error) {
	url := fmt.Sprintf("https://%s.kommo.com/oauth2/access_token", subdomain)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token request %d: %s", resp.StatusCode, b)
	}
	var tok tokenResponse
	return &tok, json.NewDecoder(resp.Body).Decode(&tok)
}

func (s *Service) fetchAccountInfo(ctx context.Context, subdomain, accessToken string) (accountID int64, email string) {
	url := fmt.Sprintf("https://%s.kommo.com/api/v4/account", subdomain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := s.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return 0, ""
	}
	defer resp.Body.Close()
	var result struct {
		ID int64 `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	return result.ID, ""
}

// ── API calls ─────────────────────────────────────────────────────────────────

// GetPipelines returns all sales pipelines with their statuses (stages).
// The agent should call this before CreateLead to pick the right pipeline_id.
func (s *Service) GetPipelines(ctx context.Context, workspaceID uuid.UUID) ([]Pipeline, error) {
	subdomain, token, err := s.getAccessToken(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://%s.kommo.com/api/v4/leads/pipelines?with=statuses", subdomain)
	var respBody []byte
	if err := withRetry(ctx, maxRetryAttempts, func() error {
		var e error
		respBody, e = s.doAPIRequest(ctx, http.MethodGet, url, token, nil)
		return e
	}); err != nil {
		return nil, err
	}

	var raw struct {
		Embedded struct {
			Pipelines []struct {
				ID       int64  `json:"id"`
				Name     string `json:"name"`
				IsMain   bool   `json:"is_main"`
				Embedded struct {
					Statuses []struct {
						ID   int64  `json:"id"`
						Name string `json:"name"`
					} `json:"statuses"`
				} `json:"_embedded"`
			} `json:"pipelines"`
		} `json:"_embedded"`
	}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("parse pipelines response: %w", err)
	}

	pipelines := make([]Pipeline, 0, len(raw.Embedded.Pipelines))
	for _, p := range raw.Embedded.Pipelines {
		pl := Pipeline{ID: p.ID, Name: p.Name, IsMain: p.IsMain}
		for _, st := range p.Embedded.Statuses {
			pl.Statuses = append(pl.Statuses, PipelineStatus{ID: st.ID, Name: st.Name})
		}
		pipelines = append(pipelines, pl)
	}
	return pipelines, nil
}

// CreateLead creates a lead (and optionally a linked contact) in AmoCRM.
func (s *Service) CreateLead(ctx context.Context, workspaceID uuid.UUID, params CreateLeadParams) (*LeadResult, error) {
	subdomain, token, err := s.getAccessToken(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	type complexLead struct {
		Name       string           `json:"name"`
		Price      int              `json:"price,omitempty"`
		PipelineID int64            `json:"pipeline_id,omitempty"`
		Embedded   map[string][]any `json:"_embedded,omitempty"`
	}

	lead := complexLead{
		Name:       params.Name,
		Price:      params.Price,
		PipelineID: params.PipelineID,
	}

	if params.ContactName != "" || params.ContactPhone != "" || params.ContactEmail != "" {
		contact := map[string]any{"name": params.ContactName}
		var cfv []map[string]any
		if params.ContactPhone != "" {
			cfv = append(cfv, map[string]any{
				"field_code": "PHONE",
				"values":     []map[string]string{{"value": params.ContactPhone}},
			})
		}
		if params.ContactEmail != "" {
			cfv = append(cfv, map[string]any{
				"field_code": "EMAIL",
				"values":     []map[string]string{{"value": params.ContactEmail}},
			})
		}
		if len(cfv) > 0 {
			contact["custom_fields_values"] = cfv
		}
		lead.Embedded = map[string][]any{"contacts": {contact}}
	}

	body, _ := json.Marshal([]complexLead{lead})
	url := fmt.Sprintf("https://%s.kommo.com/api/v4/leads/complex", subdomain)

	var respBody []byte
	if err := withRetry(ctx, maxRetryAttempts, func() error {
		var e error
		respBody, e = s.doAPIRequest(ctx, http.MethodPost, url, token, body)
		return e
	}); err != nil {
		return nil, err
	}

	var resp []struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil || len(resp) == 0 {
		return nil, fmt.Errorf("unexpected response: %s", respBody)
	}

	leadResult := &LeadResult{
		ID:   resp[0].ID,
		Name: params.Name,
		URL:  fmt.Sprintf("https://%s.kommo.com/leads/detail/%d", subdomain, resp[0].ID),
	}

	// If caller provided notes, attach them immediately
	if params.Notes != "" {
		_, _ = s.AddNote(ctx, workspaceID, AddNoteParams{LeadID: resp[0].ID, Text: params.Notes})
	}

	return leadResult, nil
}

// CreateTask creates a task in AmoCRM, optionally linked to a lead.
func (s *Service) CreateTask(ctx context.Context, workspaceID uuid.UUID, params CreateTaskParams) (*TaskResult, error) {
	subdomain, token, err := s.getAccessToken(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	task := map[string]any{
		"text":         params.Text,
		"task_type_id": 1, // 1 = follow-up call (default)
	}
	if params.TaskTypeID > 0 {
		task["task_type_id"] = params.TaskTypeID
	}
	if params.LeadID > 0 {
		task["entity_id"] = params.LeadID
		task["entity_type"] = "leads"
	}
	if params.DueDate != "" {
		t, err := time.Parse(time.RFC3339, params.DueDate)
		if err == nil {
			task["complete_till"] = t.Unix()
		}
	} else {
		task["complete_till"] = time.Now().Add(24 * time.Hour).Unix()
	}

	body, _ := json.Marshal([]any{task})
	url := fmt.Sprintf("https://%s.kommo.com/api/v4/tasks", subdomain)

	var respBody []byte
	if err := withRetry(ctx, maxRetryAttempts, func() error {
		var e error
		respBody, e = s.doAPIRequest(ctx, http.MethodPost, url, token, body)
		return e
	}); err != nil {
		return nil, err
	}

	var resp struct {
		Embedded struct {
			Tasks []struct {
				ID int64 `json:"id"`
			} `json:"tasks"`
		} `json:"_embedded"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil || len(resp.Embedded.Tasks) == 0 {
		return nil, fmt.Errorf("unexpected response: %s", respBody)
	}
	return &TaskResult{ID: resp.Embedded.Tasks[0].ID, Text: params.Text}, nil
}

// AddNote adds a text note to an existing lead in AmoCRM.
func (s *Service) AddNote(ctx context.Context, workspaceID uuid.UUID, params AddNoteParams) (*NoteResult, error) {
	subdomain, token, err := s.getAccessToken(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	note := map[string]any{
		"entity_id":   params.LeadID,
		"entity_type": "leads",
		"note_type":   "common",
		"params": map[string]string{
			"text": params.Text,
		},
	}

	body, _ := json.Marshal([]any{note})
	url := fmt.Sprintf("https://%s.kommo.com/api/v4/notes", subdomain)

	var respBody []byte
	if err := withRetry(ctx, maxRetryAttempts, func() error {
		var e error
		respBody, e = s.doAPIRequest(ctx, http.MethodPost, url, token, body)
		return e
	}); err != nil {
		return nil, err
	}

	var resp struct {
		Embedded struct {
			Notes []struct {
				ID int64 `json:"id"`
			} `json:"notes"`
		} `json:"_embedded"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil || len(resp.Embedded.Notes) == 0 {
		return nil, fmt.Errorf("unexpected response: %s", respBody)
	}
	return &NoteResult{ID: resp.Embedded.Notes[0].ID, LeadID: params.LeadID}, nil
}

// ── HTTP helper ───────────────────────────────────────────────────────────────

func (s *Service) doAPIRequest(ctx context.Context, method, url, token string, body []byte) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode >= 400 {
		return nil, &APIError{StatusCode: resp.StatusCode, Body: respBody}
	}
	return respBody, nil
}
