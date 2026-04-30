package zoho

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/dinarasaurae/inbetwin-llm-service/internal/database"
	"github.com/dinarasaurae/inbetwin-llm-service/internal/models"
	"github.com/google/uuid"
)

const (
	// Zoho Accounts API (global, datacenter-independent)
	zohoAccountsURL  = "https://accounts.zoho.com"
	zohoTokenURL     = "https://accounts.zoho.com/oauth/v2/token"
	zohoAuthURL      = "https://accounts.zoho.com/oauth/v2/auth"
	defaultAPIDomain = "https://www.zohoapis.com"

	zohoScopes = "ZohoCRM.modules.ALL,ZohoCRM.settings.fields.READ,ZohoCRM.users.READ,ZohoCRM.org.READ,offline_access"
	cacheTTL         = 5 * time.Minute
	maxRetryAttempts = 3
)

// ── Service ───────────────────────────────────────────────────────────────────

type Service struct {
	db           *database.DB
	clientID     string
	clientSecret string
	redirectURL  string
	httpClient   *http.Client

	tokenMu    sync.RWMutex
	tokenCache map[uuid.UUID]cachedToken
}

type cachedToken struct {
	apiDomain   string
	accessToken string
	expiresAt   time.Time
}

// ── OAuth token types ─────────────────────────────────────────────────────────

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // seconds
	APIDomain    string `json:"api_domain"` // e.g. https://www.zohoapis.com
	TokenType    string `json:"token_type"`
	Error        string `json:"error"`
}

// ── Params / results for API calls ───────────────────────────────────────────

type CreateLeadParams struct {
	LastName     string `json:"last_name"`
	FirstName    string `json:"first_name,omitempty"`
	Company      string `json:"company,omitempty"`
	Phone        string `json:"phone,omitempty"`
	Email        string `json:"email,omitempty"`
	Description  string `json:"description,omitempty"`
}

type CreateTaskParams struct {
	Subject string `json:"subject"`
	LeadID  string `json:"lead_id,omitempty"` // Zoho record ID string
	DueDate string `json:"due_date,omitempty"` // YYYY-MM-DD
}

type AddNoteParams struct {
	LeadID  string `json:"lead_id"`
	Content string `json:"content"`
}

type LeadResult struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	Name string `json:"name"`
}

type TaskResult struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
}

type NoteResult struct {
	ID     string `json:"id"`
	LeadID string `json:"lead_id"`
}

// DealStage represents one stage in the Zoho Deals pipeline.
type DealStage struct {
	ID           string `json:"id"`
	DisplayLabel string `json:"display_label"`
	Sequence     int    `json:"sequence_number"`
}

// Constructor ─────────────────────────────────────────────────────────────────

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

// GetAuthURL returns the Zoho OAuth consent URL.
// state = workspaceID so we can match the callback to the right tenant.
func (s *Service) GetAuthURL(workspaceID uuid.UUID) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", s.clientID)
	q.Set("scope", zohoScopes)
	q.Set("redirect_uri", s.redirectURL)
	q.Set("access_type", "offline") // ensures refresh_token is returned
	q.Set("state", workspaceID.String())
	q.Set("prompt", "consent") // always show consent to guarantee fresh refresh_token
	return zohoAuthURL + "?" + q.Encode()
}

// HandleCallback exchanges the authorisation code and persists the integration.
// Zoho returns `accounts-server` in the callback query — we use it for the
// token request so multi-datacenter accounts work correctly.
func (s *Service) HandleCallback(ctx context.Context, code, state, accountsServer string) (*models.ZohoIntegration, error) {
	workspaceID, err := uuid.Parse(state)
	if err != nil {
		return nil, fmt.Errorf("invalid state: %w", err)
	}

	tokenEndpoint := zohoTokenURL
	if accountsServer != "" {
		tokenEndpoint = accountsServer + "/oauth/v2/token"
	}

	tok, err := s.exchangeCode(ctx, tokenEndpoint, code)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}

	apiDomain := tok.APIDomain
	if apiDomain == "" {
		apiDomain = defaultAPIDomain
	}

	orgID, orgName := s.fetchOrgInfo(ctx, apiDomain, tok.AccessToken)
	expiry := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)

	log.Printf("[zoho] saving integration: workspace_id=%s api_domain=%s org_id=%q org_name=%q",
		workspaceID, apiDomain, orgID, orgName)

	// Use sql.NullString for nullable org columns to avoid scan errors on NULL.
	var orgIDNull, orgNameNull sql.NullString
	if orgID != "" {
		orgIDNull = sql.NullString{String: orgID, Valid: true}
	}
	if orgName != "" {
		orgNameNull = sql.NullString{String: orgName, Valid: true}
	}

	integration := &models.ZohoIntegration{}
	var scanOrgID, scanOrgName sql.NullString
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO zoho_integrations
			(workspace_id, api_domain, org_id, org_name, access_token, refresh_token, token_expiry)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (workspace_id) DO UPDATE SET
			api_domain    = EXCLUDED.api_domain,
			org_id        = EXCLUDED.org_id,
			org_name      = EXCLUDED.org_name,
			access_token  = EXCLUDED.access_token,
			refresh_token = EXCLUDED.refresh_token,
			token_expiry  = EXCLUDED.token_expiry,
			is_active     = true,
			updated_at    = NOW()
		RETURNING id, workspace_id, api_domain, org_id, org_name, is_active, token_expiry, created_at, updated_at`,
		workspaceID, apiDomain, orgIDNull, orgNameNull, tok.AccessToken, tok.RefreshToken, expiry,
	).Scan(&integration.ID, &integration.WorkspaceID, &integration.APIDomain,
		&scanOrgID, &scanOrgName, &integration.IsActive,
		&integration.TokenExpiry, &integration.CreatedAt, &integration.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("save integration: %w", err)
	}
	integration.OrgID = scanOrgID.String
	integration.OrgName = scanOrgName.String

	log.Printf("[zoho] integration saved: id=%s workspace_id=%s", integration.ID, integration.WorkspaceID)

	s.setCachedToken(workspaceID, apiDomain, tok.AccessToken, expiry)
	return integration, nil
}

// GetIntegration returns the stored integration row for a workspace (no token).
func (s *Service) GetIntegration(ctx context.Context, workspaceID uuid.UUID) (*models.ZohoIntegration, error) {
	i := &models.ZohoIntegration{}
	var orgID, orgName sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, workspace_id, api_domain, org_id, org_name, is_active, token_expiry, created_at, updated_at
		FROM zoho_integrations WHERE workspace_id=$1 AND is_active=true`,
		workspaceID,
	).Scan(&i.ID, &i.WorkspaceID, &i.APIDomain,
		&orgID, &orgName, &i.IsActive, &i.TokenExpiry, &i.CreatedAt, &i.UpdatedAt)
	if err != nil {
		return nil, err
	}
	i.OrgID = orgID.String
	i.OrgName = orgName.String
	return i, nil
}

// ── Token cache ───────────────────────────────────────────────────────────────

func (s *Service) getCachedToken(workspaceID uuid.UUID) (apiDomain, accessToken string, ok bool) {
	s.tokenMu.RLock()
	defer s.tokenMu.RUnlock()
	c, found := s.tokenCache[workspaceID]
	if !found || time.Now().After(c.expiresAt) {
		return "", "", false
	}
	return c.apiDomain, c.accessToken, true
}

func (s *Service) setCachedToken(workspaceID uuid.UUID, apiDomain, accessToken string, tokenExpiry time.Time) {
	cacheExp := time.Now().Add(cacheTTL)
	if natural := tokenExpiry.Add(-5 * time.Minute); natural.Before(cacheExp) {
		cacheExp = natural
	}
	if cacheExp.Before(time.Now()) {
		return
	}
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	s.tokenCache[workspaceID] = cachedToken{
		apiDomain:   apiDomain,
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

func (s *Service) getAccessToken(ctx context.Context, workspaceID uuid.UUID) (apiDomain, accessToken string, err error) {
	if d, t, ok := s.getCachedToken(workspaceID); ok {
		return d, t, nil
	}

	var refreshToken string
	var expiry time.Time
	err = s.db.QueryRowContext(ctx,
		`SELECT api_domain, access_token, refresh_token, token_expiry
		 FROM zoho_integrations WHERE workspace_id=$1 AND is_active=true`,
		workspaceID,
	).Scan(&apiDomain, &accessToken, &refreshToken, &expiry)
	if err != nil {
		return "", "", fmt.Errorf("no zoho integration: %w", err)
	}

	// Auto-refresh if expiring within 5 minutes.
	if time.Until(expiry) < 5*time.Minute {
		if tok, rerr := s.doRefreshToken(ctx, refreshToken); rerr == nil {
			newExpiry := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
			_, _ = s.db.ExecContext(ctx,
				`UPDATE zoho_integrations SET access_token=$1, refresh_token=$2, token_expiry=$3, updated_at=NOW()
				 WHERE workspace_id=$4`,
				tok.AccessToken, tok.RefreshToken, newExpiry, workspaceID)
			accessToken = tok.AccessToken
			expiry = newExpiry
		}
	}

	s.setCachedToken(workspaceID, apiDomain, accessToken, expiry)
	return apiDomain, accessToken, nil
}

func (s *Service) exchangeCode(ctx context.Context, tokenEndpoint, code string) (*tokenResponse, error) {
	log.Printf("[zoho] exchangeCode: endpoint=%s redirect_uri=%s code_prefix=%s",
		tokenEndpoint, s.redirectURL, safePrefix(code, 8))
	params := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {s.clientID},
		"client_secret": {s.clientSecret},
		"redirect_uri":  {s.redirectURL},
		"code":          {code},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint,
		bytes.NewBufferString(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return s.doTokenHTTP(req)
}

func safePrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (s *Service) doRefreshToken(ctx context.Context, refreshToken string) (*tokenResponse, error) {
	params := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {s.clientID},
		"client_secret": {s.clientSecret},
		"refresh_token": {refreshToken},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, zohoTokenURL,
		bytes.NewBufferString(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return s.doTokenHTTP(req)
}

func (s *Service) doTokenHTTP(req *http.Request) (*tokenResponse, error) {
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("parse token response: %w (body: %s)", err, body)
	}
	if tok.Error != "" {
		return nil, fmt.Errorf("zoho oauth error: %s", tok.Error)
	}
	return &tok, nil
}

// fetchOrgInfo retrieves the organisation name and ID from Zoho CRM.
// Silently returns empty strings on any error.
func (s *Service) fetchOrgInfo(ctx context.Context, apiDomain, accessToken string) (orgID, orgName string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		apiDomain+"/crm/v3/org", nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Zoho-oauthtoken "+accessToken)
	resp, err := s.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return
	}
	defer resp.Body.Close()
	var result struct {
		Org []struct {
			ID   string `json:"id"`
			Name string `json:"company_name"`
		} `json:"org"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && len(result.Org) > 0 {
		orgID = result.Org[0].ID
		orgName = result.Org[0].Name
	}
	return
}

// ── API calls ─────────────────────────────────────────────────────────────────

// GetDealStages returns the stage options for the Deals module.
// Uses /crm/v3/settings/fields?module=Deals which is available on all Zoho CRM plans.
// The Enterprise-only /crm/v3/settings/stages endpoint requires ZohoCRM.settings.pipeline.READ
// which is not granted on Free/Standard plans (OAUTH_SCOPE_MISMATCH).
func (s *Service) GetDealStages(ctx context.Context, workspaceID uuid.UUID) ([]DealStage, error) {
	apiDomain, token, err := s.getAccessToken(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	// Fields endpoint works on all plans; requires ZohoCRM.settings.fields.READ
	endpoint := apiDomain + "/crm/v3/settings/fields?module=Deals"
	var respBody []byte
	if err := withRetry(ctx, maxRetryAttempts, func() error {
		var e error
		respBody, e = s.doAPIRequest(ctx, http.MethodGet, endpoint, token, nil)
		return e
	}); err != nil {
		return nil, err
	}

	// Find the "Stage" field and extract its picklist values
	var raw struct {
		Fields []struct {
			APIName   string `json:"api_name"`
			PickList  []struct {
				ID           string `json:"id"`
				DisplayValue string `json:"display_value"`
				SequenceNo   int    `json:"sequence_number"`
			} `json:"pick_list_values"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("parse fields: %w", err)
	}

	for _, f := range raw.Fields {
		if f.APIName == "Stage" {
			stages := make([]DealStage, 0, len(f.PickList))
			for i, v := range f.PickList {
				id := v.ID
				if id == "" {
					id = v.DisplayValue // fallback: use display value as ID
				}
				seq := v.SequenceNo
				if seq == 0 {
					seq = i + 1
				}
				stages = append(stages, DealStage{
					ID:           id,
					DisplayLabel: v.DisplayValue,
					Sequence:     seq,
				})
			}
			return stages, nil
		}
	}
	// Stage field not found — return empty, not an error
	return []DealStage{}, nil
}

// CreateLead creates a record in the Zoho CRM Leads module.
func (s *Service) CreateLead(ctx context.Context, workspaceID uuid.UUID, params CreateLeadParams) (*LeadResult, error) {
	apiDomain, token, err := s.getAccessToken(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	record := map[string]any{
		"Last_Name": params.LastName,
	}
	if params.FirstName != "" {
		record["First_Name"] = params.FirstName
	}
	if params.Company != "" {
		record["Company"] = params.Company
	}
	if params.Phone != "" {
		record["Phone"] = params.Phone
	}
	if params.Email != "" {
		record["Email"] = params.Email
	}
	if params.Description != "" {
		record["Description"] = params.Description
	}

	payload, _ := json.Marshal(map[string]any{"data": []any{record}})
	endpoint := apiDomain + "/crm/v3/Leads"

	var respBody []byte
	if err := withRetry(ctx, maxRetryAttempts, func() error {
		var e error
		respBody, e = s.doAPIRequest(ctx, http.MethodPost, endpoint, token, payload)
		return e
	}); err != nil {
		return nil, err
	}

	var resp struct {
		Data []struct {
			Details struct {
				ID string `json:"id"`
			} `json:"details"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil || len(resp.Data) == 0 {
		return nil, fmt.Errorf("unexpected response: %s", respBody)
	}
	if resp.Data[0].Code != "SUCCESS" {
		return nil, fmt.Errorf("zoho create lead: %s — %s", resp.Data[0].Code, resp.Data[0].Message)
	}

	recordID := resp.Data[0].Details.ID
	name := params.FirstName + " " + params.LastName
	return &LeadResult{
		ID:   recordID,
		Name: name,
		URL:  apiDomain + "/crm/v3/Leads/" + recordID,
	}, nil
}

// CreateTask creates a task in Zoho CRM, optionally linked to a lead.
func (s *Service) CreateTask(ctx context.Context, workspaceID uuid.UUID, params CreateTaskParams) (*TaskResult, error) {
	apiDomain, token, err := s.getAccessToken(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	record := map[string]any{
		"Subject": params.Subject,
		"Status":  "Not Started",
	}
	if params.LeadID != "" {
		record["What_Id"] = map[string]string{"id": params.LeadID}
	}
	if params.DueDate != "" {
		record["Due_Date"] = params.DueDate
	} else {
		record["Due_Date"] = time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	}

	payload, _ := json.Marshal(map[string]any{"data": []any{record}})
	endpoint := apiDomain + "/crm/v3/Tasks"

	var respBody []byte
	if err := withRetry(ctx, maxRetryAttempts, func() error {
		var e error
		respBody, e = s.doAPIRequest(ctx, http.MethodPost, endpoint, token, payload)
		return e
	}); err != nil {
		return nil, err
	}

	var resp struct {
		Data []struct {
			Details struct {
				ID string `json:"id"`
			} `json:"details"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil || len(resp.Data) == 0 {
		return nil, fmt.Errorf("unexpected response: %s", respBody)
	}
	if resp.Data[0].Code != "SUCCESS" {
		return nil, fmt.Errorf("zoho create task: %s — %s", resp.Data[0].Code, resp.Data[0].Message)
	}
	return &TaskResult{ID: resp.Data[0].Details.ID, Subject: params.Subject}, nil
}

// AddNote attaches a note to a Zoho CRM Lead record.
func (s *Service) AddNote(ctx context.Context, workspaceID uuid.UUID, params AddNoteParams) (*NoteResult, error) {
	apiDomain, token, err := s.getAccessToken(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	record := map[string]any{
		"Note_Content": params.Content,
		"Parent_Id":    params.LeadID,
		"$se_module":   "Leads",
	}

	payload, _ := json.Marshal(map[string]any{"data": []any{record}})
	endpoint := apiDomain + "/crm/v3/Notes"

	var respBody []byte
	if err := withRetry(ctx, maxRetryAttempts, func() error {
		var e error
		respBody, e = s.doAPIRequest(ctx, http.MethodPost, endpoint, token, payload)
		return e
	}); err != nil {
		return nil, err
	}

	var resp struct {
		Data []struct {
			Details struct {
				ID string `json:"id"`
			} `json:"details"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil || len(resp.Data) == 0 {
		return nil, fmt.Errorf("unexpected response: %s", respBody)
	}
	if resp.Data[0].Code != "SUCCESS" {
		return nil, fmt.Errorf("zoho add note: %s — %s", resp.Data[0].Code, resp.Data[0].Message)
	}
	return &NoteResult{ID: resp.Data[0].Details.ID, LeadID: params.LeadID}, nil
}

// ── HTTP helper ───────────────────────────────────────────────────────────────

func (s *Service) doAPIRequest(ctx context.Context, method, endpoint, token string, body []byte) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return nil, err
	}
	// Zoho uses "Zoho-oauthtoken" scheme, not "Bearer"
	req.Header.Set("Authorization", "Zoho-oauthtoken "+token)
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
