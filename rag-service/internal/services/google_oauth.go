package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dinarasaurae/inbetwin-rag-service/internal/database"
	"github.com/google/uuid"
)

const (
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"
	// Full Sheets scope is required for values:batchGetByDataFilter.
	sheetsScope = "https://www.googleapis.com/auth/spreadsheets"
)

type GoogleOAuthService struct {
	db           *database.DB
	clientID     string
	clientSecret string
	redirectURI  string
}

type GoogleToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
	Scope        string    `json:"scope"`
}

func NewGoogleOAuthService(db *database.DB, clientID, clientSecret, redirectURI string) *GoogleOAuthService {
	return &GoogleOAuthService{
		db:           db,
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
	}
}

// IsConfigured returns true when Google OAuth credentials are present.
func (s *GoogleOAuthService) IsConfigured() bool {
	return s.clientID != "" && s.clientSecret != ""
}

// StartOAuth generates an authorization URL for the workspace.
// State is persisted in PostgreSQL so it survives service restarts.
func (s *GoogleOAuthService) StartOAuth(ctx context.Context, workspaceID uuid.UUID) (authURL, state string, err error) {
	if !s.IsConfigured() {
		return "", "", fmt.Errorf("google_oauth_not_configured: set GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET")
	}

	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", "", fmt.Errorf("generate state: %w", err)
	}
	state = hex.EncodeToString(stateBytes)

	// Persist state in DB (TTL 15 min), clean up expired rows first
	_, _ = s.db.ExecContext(ctx, `DELETE FROM google_oauth_states WHERE expires_at < NOW()`)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO google_oauth_states (state, workspace_id, expires_at) VALUES ($1, $2, $3)`,
		state, workspaceID, time.Now().Add(15*time.Minute),
	)
	if err != nil {
		return "", "", fmt.Errorf("save oauth state: %w", err)
	}

	params := url.Values{
		"client_id":              {s.clientID},
		"redirect_uri":           {s.redirectURI},
		"response_type":          {"code"},
		"scope":                  {sheetsScope},
		"access_type":            {"offline"},
		"prompt":                 {"consent"},
		"include_granted_scopes": {"true"},
		"state":                  {state},
	}
	return googleAuthURL + "?" + params.Encode(), state, nil
}

// ExchangeCode exchanges the authorization code from the callback.
func (s *GoogleOAuthService) ExchangeCode(ctx context.Context, code, state string) error {
	// Look up and delete the state from DB
	var workspaceID uuid.UUID
	var expiresAt time.Time
	err := s.db.QueryRowContext(ctx,
		`DELETE FROM google_oauth_states WHERE state=$1 RETURNING workspace_id, expires_at`,
		state,
	).Scan(&workspaceID, &expiresAt)
	if err != nil {
		return fmt.Errorf("invalid_state: unknown or expired OAuth state")
	}
	if expiresAt.Before(time.Now()) {
		return fmt.Errorf("invalid_state: OAuth state expired")
	}

	token, err := s.exchangeCode(ctx, code)
	if err != nil {
		return err
	}

	return s.saveToken(ctx, workspaceID, token)
}

func (s *GoogleOAuthService) exchangeCode(ctx context.Context, code string) (*GoogleToken, error) {
	body := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {s.clientID},
		"client_secret": {s.clientSecret},
		"redirect_uri":  {s.redirectURI},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL,
		strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var data struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
		Error        string `json:"error"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	if data.Error != "" {
		return nil, fmt.Errorf("google oauth error: %s", data.Error)
	}
	return &GoogleToken{
		AccessToken:  data.AccessToken,
		RefreshToken: data.RefreshToken,
		Expiry:       time.Now().Add(time.Duration(data.ExpiresIn) * time.Second),
		Scope:        data.Scope,
	}, nil
}

func (s *GoogleOAuthService) refreshToken(ctx context.Context, refreshToken string) (*GoogleToken, error) {
	body := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {s.clientID},
		"client_secret": {s.clientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL,
		strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var data struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parse refresh response: %w", err)
	}
	if data.Error != "" {
		return nil, fmt.Errorf("google refresh error: %s", data.Error)
	}
	return &GoogleToken{
		AccessToken:  data.AccessToken,
		RefreshToken: refreshToken, // Google keeps the same refresh token
		Expiry:       time.Now().Add(time.Duration(data.ExpiresIn) * time.Second),
		Scope:        data.Scope,
	}, nil
}

// GetValidToken returns a valid access token for the workspace, refreshing if necessary.
func (s *GoogleOAuthService) GetValidToken(ctx context.Context, workspaceID uuid.UUID) (string, error) {
	var accessToken, refreshToken string
	var expiry time.Time

	err := s.db.QueryRowContext(ctx,
		`SELECT access_token, refresh_token, expiry FROM google_oauth_tokens WHERE workspace_id=$1`,
		workspaceID,
	).Scan(&accessToken, &refreshToken, &expiry)
	if err != nil {
		return "", fmt.Errorf("google_not_connected: no Google OAuth token for this workspace")
	}

	// Refresh if token expires within 60 seconds
	if time.Now().Add(60*time.Second).After(expiry) && refreshToken != "" {
		token, err := s.refreshToken(ctx, refreshToken)
		if err != nil {
			return "", fmt.Errorf("refresh_failed: %w", err)
		}
		if err := s.saveToken(ctx, workspaceID, token); err != nil {
			return "", err
		}
		return token.AccessToken, nil
	}
	return accessToken, nil
}

// IsConnected returns true if the workspace has stored Google OAuth tokens.
func (s *GoogleOAuthService) IsConnected(ctx context.Context, workspaceID uuid.UUID) bool {
	var exists bool
	s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM google_oauth_tokens WHERE workspace_id=$1)`,
		workspaceID,
	).Scan(&exists)
	return exists
}

// Disconnect removes stored tokens for the workspace.
func (s *GoogleOAuthService) Disconnect(ctx context.Context, workspaceID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM google_oauth_tokens WHERE workspace_id=$1`, workspaceID)
	return err
}

func (s *GoogleOAuthService) saveToken(ctx context.Context, workspaceID uuid.UUID, token *GoogleToken) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO google_oauth_tokens (workspace_id, access_token, refresh_token, expiry, scope)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (workspace_id) DO UPDATE SET
			access_token  = EXCLUDED.access_token,
			refresh_token = CASE WHEN EXCLUDED.refresh_token != '' THEN EXCLUDED.refresh_token
			                     ELSE google_oauth_tokens.refresh_token END,
			expiry        = EXCLUDED.expiry,
			scope         = EXCLUDED.scope,
			updated_at    = NOW()`,
		workspaceID, token.AccessToken, token.RefreshToken, token.Expiry, token.Scope,
	)
	return err
}
