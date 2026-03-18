package services

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/dinarasaurae/inbetwin-social-service/internal/database"
	"github.com/dinarasaurae/inbetwin-social-service/internal/models"
)

// WebhookService processes incoming Telegram Bot API updates.
type WebhookService struct {
	db *database.DB
}

func NewWebhookService(db *database.DB) *WebhookService {
	return &WebhookService{db: db}
}

// ProcessUpdate routes an incoming update to the appropriate handler.
// Only channel_post events are processed; edited_channel_post is ignored in Phase 1.
func (s *WebhookService) ProcessUpdate(ctx context.Context, update *models.TelegramUpdate) {
	if update.ChannelPost == nil {
		return
	}
	if err := s.processChannelPost(ctx, update.ChannelPost); err != nil {
		log.Printf("webhook: processChannelPost error for message %d: %v",
			update.ChannelPost.MessageID, err)
	}
}

func (s *WebhookService) processChannelPost(ctx context.Context, msg *models.TelegramMessage) error {
	// Find the active integration for this channel.
	integration, err := s.findIntegrationByChannel(ctx, msg.Chat.ID)
	if err != nil {
		return nil // not a registered channel; ignore silently
	}

	post := buildPost(integration, msg)

	// Upsert: do nothing on duplicate (channel_id, message_id).
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO telegram_posts
			(integration_id, user_id, message_id, channel_id,
			 text, media_type, has_media, link_count, extracted_urls,
			 posted_at, forward_count, view_count)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT (channel_id, message_id) DO NOTHING`,
		post.IntegrationID, post.UserID, post.MessageID, post.ChannelID,
		post.Text, post.MediaType, post.HasMedia, post.LinkCount, pq.Array(post.ExtractedURLs),
		post.PostedAt, post.ForwardCount, post.ViewCount,
	)
	return err
}

// findIntegrationByChannel returns the active integration for a given Telegram channel ID.
func (s *WebhookService) findIntegrationByChannel(ctx context.Context, channelID int64) (*models.TelegramIntegration, error) {
	var i models.TelegramIntegration
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id FROM telegram_integrations
		 WHERE channel_id = $1 AND status = 'active'
		 LIMIT 1`,
		channelID,
	).Scan(&i.ID, &i.UserID)
	if err != nil {
		return nil, fmt.Errorf("integration not found for channel %d", channelID)
	}
	return &i, nil
}

// buildPost extracts and normalises data from a Telegram message.
// Only collects: text, media type, link count, timestamps, engagement counts.
// Does NOT collect: user IDs, commenter data, personal info.
func buildPost(integration *models.TelegramIntegration, msg *models.TelegramMessage) *models.TelegramPost {
	post := &models.TelegramPost{
		IntegrationID: integration.ID,
		UserID:        integration.UserID,
		MessageID:     msg.MessageID,
		ChannelID:     msg.Chat.ID,
		PostedAt:      time.Unix(msg.Date, 0).UTC(),
	}

	// Text content: prefer Text, fall back to Caption (for media posts).
	if msg.Text != "" {
		text := msg.Text
		post.Text = &text
	} else if msg.Caption != "" {
		caption := msg.Caption
		post.Text = &caption
	}

	// Detect media type.
	mediaType, hasMedia := detectMediaType(msg)
	post.MediaType = mediaType
	post.HasMedia = hasMedia

	// Extract URLs from entities.
	urls := extractURLs(msg)
	post.ExtractedURLs = urls
	post.LinkCount = len(urls)

	// Engagement (may be zero on fresh posts).
	if msg.ForwardCount > 0 {
		fc := msg.ForwardCount
		post.ForwardCount = &fc
	}
	if msg.Views > 0 {
		v := msg.Views
		post.ViewCount = &v
	}

	return post
}

func detectMediaType(msg *models.TelegramMessage) (*string, bool) {
	var t string
	switch {
	case len(msg.Photo) > 0:
		t = "photo"
	case msg.Video != nil:
		t = "video"
	case msg.Document != nil:
		t = "document"
	case msg.Audio != nil:
		t = "audio"
	case msg.Voice != nil:
		t = "voice"
	case msg.Poll != nil:
		t = "poll"
	default:
		return nil, false
	}
	return &t, true
}

func extractURLs(msg *models.TelegramMessage) []string {
	seen := make(map[string]struct{})
	var urls []string

	addURL := func(u string) {
		u = strings.TrimSpace(u)
		if u == "" {
			return
		}
		if _, dup := seen[u]; dup {
			return
		}
		seen[u] = struct{}{}
		urls = append(urls, u)
	}

	text := msg.Text
	if text == "" {
		text = msg.Caption
	}

	// Collect from message entities.
	allEntities := append(msg.Entities, msg.CaptionEntities...)
	for _, e := range allEntities {
		switch e.Type {
		case "url":
			// Slice the URL from the text using byte offsets.
			runes := []rune(text)
			if e.Offset >= 0 && e.Offset+e.Length <= len(runes) {
				addURL(string(runes[e.Offset : e.Offset+e.Length]))
			}
		case "text_link":
			addURL(e.URL)
		}
	}

	return urls
}

// ensure uuid package is used (it's imported via models)
var _ = uuid.UUID{}
