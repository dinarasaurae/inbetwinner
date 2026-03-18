package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/dinarasaurae/inbetwin-social-service/internal/config"
	"github.com/dinarasaurae/inbetwin-social-service/internal/crypto"
	"github.com/dinarasaurae/inbetwin-social-service/internal/database"
	"github.com/dinarasaurae/inbetwin-social-service/internal/models"
)

var (
	ErrInvalidTelegramHash  = errors.New("invalid_telegram_hash")
	ErrAuthDataExpired      = errors.New("auth_data_expired")
	ErrConsentRequired      = errors.New("consent_required")
	ErrBotNotAdmin          = errors.New("bot_not_channel_admin")
	ErrChannelNotFound      = errors.New("channel_not_found")
	ErrAlreadyConnected     = errors.New("channel_already_connected")
	ErrIntegrationNotFound  = errors.New("integration_not_found")
)

// consentText is the text shown to the user and stored verbatim in user_consents.
const consentText = "I authorize inBeTwin to collect and analyze posts from my Telegram channel " +
	"for digital footprint analysis. Only my own channel content will be processed. " +
	"I can revoke this consent at any time."

// TelegramService handles channel connect/disconnect and data collection.
type TelegramService struct {
	db         *database.DB
	enc        *crypto.Encryptor
	cfg        *config.Config
	httpClient *http.Client
	botID      int64 // populated by Init()
}

// NewTelegramService creates the service. Call Init() before use.
func NewTelegramService(db *database.DB, enc *crypto.Encryptor, cfg *config.Config) *TelegramService {
	return &TelegramService{
		db:  db,
		enc: enc,
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Init fetches the bot's own user ID via getMe and registers the webhook URL if configured.
func (s *TelegramService) Init(ctx context.Context) error {
	if s.cfg.TelegramBotToken == "" {
		return errors.New("TELEGRAM_BOT_TOKEN is not set")
	}

	me, err := s.getMe(ctx)
	if err != nil {
		return fmt.Errorf("getMe failed: %w", err)
	}
	s.botID = me.ID

	if s.cfg.TelegramWebhookURL != "" {
		if err := s.setWebhook(ctx); err != nil {
			return fmt.Errorf("setWebhook failed: %w", err)
		}
	}

	return nil
}

// VerifyLoginWidget validates the hash from the Telegram Login Widget.
// Algorithm: https://core.telegram.org/widgets/login#checking-authorization
func (s *TelegramService) VerifyLoginWidget(req models.ConnectTelegramRequest) error {
	// Build sorted key=value pairs, excluding "hash" and empty fields.
	fields := map[string]string{
		"auth_date":  strconv.FormatInt(req.AuthDate, 10),
		"first_name": req.FirstName,
		"id":         strconv.FormatInt(req.ID, 10),
		"last_name":  req.LastName,
		"photo_url":  req.PhotoURL,
		"username":   req.Username,
	}

	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		if fields[k] != "" {
			parts = append(parts, k+"="+fields[k])
		}
	}
	checkString := strings.Join(parts, "\n")

	// secret_key = SHA256(bot_token) — raw hash, NOT HMAC
	h := sha256.Sum256([]byte(s.cfg.TelegramBotToken))
	mac := hmac.New(sha256.New, h[:])
	mac.Write([]byte(checkString))
	expected := hex.EncodeToString(mac.Sum(nil))

	// Constant-time comparison prevents timing attacks.
	if !hmac.Equal([]byte(expected), []byte(req.Hash)) {
		return ErrInvalidTelegramHash
	}

	// Freshness: auth_date must be within 5 minutes.
	if time.Now().Unix()-req.AuthDate > 300 {
		return ErrAuthDataExpired
	}

	return nil
}

// ConnectChannel verifies the request, checks bot admin status, records consent,
// and stores the integration — all in a single DB transaction.
func (s *TelegramService) ConnectChannel(
	ctx context.Context,
	userID uuid.UUID,
	req models.ConnectTelegramRequest,
	ipAddr, userAgent string,
) (*models.TelegramIntegration, error) {
	// 1. Verify Login Widget signature.
	if err := s.VerifyLoginWidget(req); err != nil {
		return nil, err
	}

	// 2. Require explicit consent acknowledgement.
	if !req.ConsentAcknowledged {
		return nil, ErrConsentRequired
	}

	// 3. Resolve channel info via Bot API.
	chatID := "@" + strings.TrimPrefix(req.ChannelUsername, "@")
	chat, err := s.getChat(ctx, chatID)
	if err != nil {
		return nil, ErrChannelNotFound
	}
	if chat.Type != "channel" {
		return nil, ErrChannelNotFound
	}

	// 4. Verify the platform bot is an administrator on the channel.
	if err := s.verifyBotIsAdmin(ctx, chat.ID); err != nil {
		return nil, ErrBotNotAdmin
	}

	// 5. Encrypt the bot token before storage.
	var tokenEnc, tokenIV []byte
	if s.enc != nil {
		tokenEnc, tokenIV, err = s.enc.Encrypt([]byte(s.cfg.TelegramBotToken))
		if err != nil {
			return nil, fmt.Errorf("token encryption failed: %w", err)
		}
	} else {
		// Encryption key not set (dev/test): store placeholder bytes.
		tokenEnc = []byte("DEV_NO_ENCRYPTION")
		tokenIV = []byte("DEV_NO_IV_TWELVE!")
	}

	// 6. Atomic: insert consent + integration in one transaction.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Insert consent.
	var consentID uuid.UUID
	err = tx.QueryRowContext(ctx,
		`INSERT INTO user_consents (user_id, platform, scope, consent_text, ip_address, user_agent)
		 VALUES ($1, 'telegram', $2, $3, $4::inet, $5)
		 RETURNING id`,
		userID,
		pq.Array([]string{"read_posts", "read_media"}),
		consentText,
		nullableString(ipAddr),
		nullableString(userAgent),
	).Scan(&consentID)
	if err != nil {
		return nil, fmt.Errorf("insert consent: %w", err)
	}

	// Insert integration.
	var integration models.TelegramIntegration
	err = tx.QueryRowContext(ctx,
		`INSERT INTO telegram_integrations
			(user_id, consent_id, channel_id, channel_username, channel_title, bot_token_enc, bot_token_iv)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, user_id, consent_id, channel_id, channel_username, channel_title,
		           status, connected_at, created_at, updated_at`,
		userID, consentID, chat.ID, chat.Username, chat.Title, tokenEnc, tokenIV,
	).Scan(
		&integration.ID, &integration.UserID, &integration.ConsentID,
		&integration.ChannelID, &integration.ChannelUsername, &integration.ChannelTitle,
		&integration.Status, &integration.ConnectedAt, &integration.CreatedAt, &integration.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "uq_user_channel") {
			return nil, ErrAlreadyConnected
		}
		return nil, fmt.Errorf("insert integration: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return &integration, nil
}

// DisconnectChannel soft-deletes consent and integration, then purges posts.
func (s *TelegramService) DisconnectChannel(ctx context.Context, userID uuid.UUID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Soft-delete integration.
	var integrationID uuid.UUID
	err = tx.QueryRowContext(ctx,
		`UPDATE telegram_integrations
		 SET status = 'revoked', disconnected_at = NOW()
		 WHERE user_id = $1 AND status = 'active'
		 RETURNING id`,
		userID,
	).Scan(&integrationID)
	if err != nil {
		return ErrIntegrationNotFound
	}

	// Revoke consent.
	_, err = tx.ExecContext(ctx,
		`UPDATE user_consents
		 SET revoked_at = NOW()
		 WHERE user_id = $1 AND platform = 'telegram' AND revoked_at IS NULL`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("revoke consent: %w", err)
	}

	// Delete posts (Phase 1: inline; Phase 2: enqueue via RabbitMQ).
	_, err = tx.ExecContext(ctx,
		`DELETE FROM telegram_posts WHERE integration_id = $1`,
		integrationID,
	)
	if err != nil {
		return fmt.Errorf("delete posts: %w", err)
	}

	return tx.Commit()
}

// GetStatus returns the current integration status for a user.
func (s *TelegramService) GetStatus(ctx context.Context, userID uuid.UUID) (*models.StatusResponse, error) {
	var resp models.StatusResponse

	row := s.db.QueryRowContext(ctx,
		`SELECT i.channel_id, i.channel_username, i.channel_title, i.status, i.last_synced_at,
		        c.granted_at
		 FROM telegram_integrations i
		 JOIN user_consents c ON c.id = i.consent_id
		 WHERE i.user_id = $1 AND i.status = 'active'
		 LIMIT 1`,
		userID,
	)

	var channelID int64
	var channelUsername *string
	var channelTitle, status string
	var lastSyncedAt, consentGrantedAt *time.Time

	err := row.Scan(&channelID, &channelUsername, &channelTitle, &status, &lastSyncedAt, &consentGrantedAt)
	if err != nil {
		// No active integration.
		resp.Connected = false
		return &resp, nil
	}

	resp.Connected = true
	resp.ChannelID = &channelID
	resp.ChannelUsername = channelUsername
	resp.ChannelTitle = &channelTitle
	resp.IntegrationStatus = &status
	resp.LastSyncedAt = lastSyncedAt
	resp.ConsentGrantedAt = consentGrantedAt
	return &resp, nil
}

// GetPosts returns paginated posts for the user's active integration.
func (s *TelegramService) GetPosts(
	ctx context.Context,
	userID uuid.UUID,
	limit, offset int,
) ([]*models.TelegramPost, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Get total count.
	var total int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM telegram_posts WHERE user_id = $1`,
		userID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, integration_id, user_id, message_id, channel_id,
		        text, media_type, has_media, link_count, extracted_urls,
		        posted_at, fetched_at, forward_count, view_count
		 FROM telegram_posts
		 WHERE user_id = $1
		 ORDER BY posted_at DESC
		 LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var posts []*models.TelegramPost
	for rows.Next() {
		var p models.TelegramPost
		var urlsJSON []byte
		err := rows.Scan(
			&p.ID, &p.IntegrationID, &p.UserID, &p.MessageID, &p.ChannelID,
			&p.Text, &p.MediaType, &p.HasMedia, &p.LinkCount, &urlsJSON,
			&p.PostedAt, &p.FetchedAt, &p.ForwardCount, &p.ViewCount,
		)
		if err != nil {
			return nil, 0, err
		}
		posts = append(posts, &p)
	}

	return posts, total, rows.Err()
}

// --- Bot API helpers ---

func (s *TelegramService) getMe(ctx context.Context) (*models.TelegramBotUser, error) {
	var result models.TelegramBotUser
	if err := s.callBotAPI(ctx, "getMe", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *TelegramService) getChat(ctx context.Context, chatID string) (*models.TelegramChat, error) {
	var result models.TelegramChat
	err := s.callBotAPI(ctx, "getChat", map[string]interface{}{
		"chat_id": chatID,
	}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *TelegramService) verifyBotIsAdmin(ctx context.Context, channelID int64) error {
	var result models.TelegramChatMember
	err := s.callBotAPI(ctx, "getChatMember", map[string]interface{}{
		"chat_id": channelID,
		"user_id": s.botID,
	}, &result)
	if err != nil {
		return ErrBotNotAdmin
	}
	if result.Status != "administrator" && result.Status != "creator" {
		return ErrBotNotAdmin
	}
	return nil
}

func (s *TelegramService) setWebhook(ctx context.Context) error {
	return s.callBotAPI(ctx, "setWebhook", map[string]interface{}{
		"url":          s.cfg.TelegramWebhookURL,
		"secret_token": s.cfg.TelegramWebhookSecret,
		"allowed_updates": []string{"channel_post", "edited_channel_post"},
	}, nil)
}

// callBotAPI performs a POST request to the Telegram Bot API and decodes the result.
func (s *TelegramService) callBotAPI(ctx context.Context, method string, params map[string]interface{}, result interface{}) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/%s", s.cfg.TelegramBotToken, method)

	var body io.Reader
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return err
	}
	if params != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("telegram API request failed: %w", err)
	}
	defer resp.Body.Close()

	var envelope struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("decode telegram response: %w", err)
	}
	if !envelope.OK {
		return fmt.Errorf("telegram API error: %s", envelope.Description)
	}

	if result != nil && envelope.Result != nil {
		return json.Unmarshal(envelope.Result, result)
	}
	return nil
}

// nullableString returns nil if s is empty, otherwise a pointer to s.
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
