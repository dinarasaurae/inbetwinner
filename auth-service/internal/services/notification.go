package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2/google"

	"github.com/dinarasaurae/inbetwin-auth-service/internal/database"
	"github.com/dinarasaurae/inbetwin-auth-service/internal/models"
)

// FCM v1 API — requires Service Account JSON, not legacy Server Key.
// Env vars:
//
//	GOOGLE_SERVICE_ACCOUNT_JSON — full JSON content of the service account key file
//	FIREBASE_PROJECT_ID         — Firebase project ID (e.g. "inbetwin-12345")
const fcmScope = "https://www.googleapis.com/auth/firebase.messaging"

type NotificationService struct {
	db        *database.DB
	projectID string

	// OAuth2 token cache (service account tokens are valid for 1 hour)
	tokenMu      sync.Mutex
	cachedToken  string
	tokenExpires time.Time
}

func NewNotificationService(db *database.DB) *NotificationService {
	return &NotificationService{
		db:        db,
		projectID: os.Getenv("FIREBASE_PROJECT_ID"),
	}
}

// ── Token management ─────────────────────────────────────────────────────────

// RegisterToken saves (or updates) a device token for the given user.
func (s *NotificationService) RegisterToken(ctx context.Context, userID uuid.UUID, req models.RegisterDeviceTokenRequest) error {
	if req.Token == "" {
		return fmt.Errorf("token is required")
	}
	platform := req.Platform
	if platform == "" {
		platform = "android"
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO device_tokens (user_id, token, platform)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, token) DO UPDATE
			SET platform   = EXCLUDED.platform,
			    updated_at = NOW()
	`, userID, req.Token, platform)
	return err
}

// UnregisterToken removes a device token (e.g. on logout).
func (s *NotificationService) UnregisterToken(ctx context.Context, userID uuid.UUID, token string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM device_tokens WHERE user_id=$1 AND token=$2`,
		userID, token,
	)
	return err
}

// GetTokensForUser returns all FCM tokens registered for the user.
func (s *NotificationService) GetTokensForUser(ctx context.Context, userID uuid.UUID) ([]models.DeviceToken, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, token, platform, created_at, updated_at
		 FROM device_tokens WHERE user_id=$1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []models.DeviceToken
	for rows.Next() {
		var t models.DeviceToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.Token, &t.Platform, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// ── Push sending ─────────────────────────────────────────────────────────────

type PushPayload struct {
	UserID   uuid.UUID         `json:"-"`
	Type     string            `json:"type"`
	Title    string            `json:"title"`
	Body     string            `json:"body"`
	Data     map[string]string `json:"data,omitempty"`
	DeepLink string            `json:"deep_link,omitempty"`
}

// SendPushToUser sends FCM notifications to all devices of a user.
func (s *NotificationService) SendPushToUser(ctx context.Context, payload PushPayload) error {
	if s.projectID == "" {
		log.Println("[notifications] FIREBASE_PROJECT_ID not set, skipping push")
		return nil
	}

	tokens, err := s.GetTokensForUser(ctx, payload.UserID)
	if err != nil {
		return fmt.Errorf("get tokens: %w", err)
	}
	if len(tokens) == 0 {
		log.Printf("[notifications] no device tokens for user %s, skipping", payload.UserID)
		return nil
	}

	accessToken, err := s.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("get FCM access token: %w", err)
	}

	for _, dt := range tokens {
		if err := s.sendFCMv1(ctx, accessToken, dt.Token, payload); err != nil {
			// Non-fatal: log and continue with remaining tokens
			log.Printf("[notifications] FCM error for token %s…: %v", dt.Token[:8], err)
		}
	}
	return nil
}

// getAccessToken returns a cached OAuth2 access token.
// Uses Application Default Credentials — reads GOOGLE_APPLICATION_CREDENTIALS
// env var which points to the service account JSON file path.
func (s *NotificationService) getAccessToken(ctx context.Context) (string, error) {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()

	// Return cached token if still valid (with 30-second buffer)
	if s.cachedToken != "" && time.Now().Before(s.tokenExpires.Add(-30*time.Second)) {
		return s.cachedToken, nil
	}

	// google.FindDefaultCredentials automatically reads GOOGLE_APPLICATION_CREDENTIALS
	creds, err := google.FindDefaultCredentials(ctx, fcmScope)
	if err != nil {
		return "", fmt.Errorf("firebase credentials not found (set GOOGLE_APPLICATION_CREDENTIALS): %w", err)
	}

	token, err := creds.TokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("get token: %w", err)
	}

	s.cachedToken = token.AccessToken
	s.tokenExpires = token.Expiry
	return s.cachedToken, nil
}

// sendFCMv1 sends one FCM message via the HTTP v1 API using a Bearer token.
func (s *NotificationService) sendFCMv1(ctx context.Context, accessToken, deviceToken string, payload PushPayload) error {
	data := map[string]string{"type": payload.Type}
	for k, v := range payload.Data {
		data[k] = v
	}
	if payload.DeepLink != "" {
		data["deep_link"] = payload.DeepLink
	}

	// FCM v1 message format
	body := map[string]interface{}{
		"message": map[string]interface{}{
			"token": deviceToken,
			"notification": map[string]string{
				"title": payload.Title,
				"body":  payload.Body,
			},
			"data": data,
			// Android-specific: high priority for urgent notifications
			"android": map[string]interface{}{
				"priority": "high",
			},
		},
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", s.projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("FCM v1 returned HTTP %d", resp.StatusCode)
	}
	return nil
}
