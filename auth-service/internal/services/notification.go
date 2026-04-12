package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/google/uuid"

	"github.com/dinarasaurae/inbetwin-auth-service/internal/database"
	"github.com/dinarasaurae/inbetwin-auth-service/internal/models"
)

type NotificationService struct {
	db     *database.DB
	fcmKey string // FCM Server Key from Firebase Console → Project Settings → Cloud Messaging
}

func NewNotificationService(db *database.DB) *NotificationService {
	return &NotificationService{
		db:     db,
		fcmKey: os.Getenv("FCM_SERVER_KEY"),
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
			SET platform = EXCLUDED.platform,
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

// GetTokensForUser returns all active FCM tokens for a user.
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
	Type     string            `json:"type"`     // e.g. "HOT_LEAD"
	Title    string            `json:"title"`
	Body     string            `json:"body"`
	Data     map[string]string `json:"data,omitempty"`
	DeepLink string            `json:"deep_link,omitempty"`
}

// SendPushToUser sends a FCM notification to all devices of a user.
func (s *NotificationService) SendPushToUser(ctx context.Context, payload PushPayload) error {
	tokens, err := s.GetTokensForUser(ctx, payload.UserID)
	if err != nil {
		return fmt.Errorf("get tokens: %w", err)
	}
	if len(tokens) == 0 {
		log.Printf("[notifications] no device tokens for user %s, skipping push", payload.UserID)
		return nil
	}

	for _, dt := range tokens {
		if err := s.sendFCM(ctx, dt.Token, payload); err != nil {
			// Non-fatal: log and continue with remaining tokens
			log.Printf("[notifications] FCM send error for token %s: %v", dt.Token[:8]+"…", err)
		}
	}
	return nil
}

// sendFCM sends a single FCM message using the legacy HTTP API.
// For production, replace with firebase-admin-go SDK.
func (s *NotificationService) sendFCM(ctx context.Context, token string, payload PushPayload) error {
	if s.fcmKey == "" {
		log.Println("[notifications] FCM_SERVER_KEY not set, skipping push")
		return nil
	}

	data := map[string]string{
		"type": payload.Type,
	}
	for k, v := range payload.Data {
		data[k] = v
	}
	if payload.DeepLink != "" {
		data["deep_link"] = payload.DeepLink
	}

	body := map[string]interface{}{
		"to": token,
		"notification": map[string]string{
			"title": payload.Title,
			"body":  payload.Body,
		},
		"data": data,
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://fcm.googleapis.com/fcm/send", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "key="+s.fcmKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("FCM returned %d", resp.StatusCode)
	}
	return nil
}
