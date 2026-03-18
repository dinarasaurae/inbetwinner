package models

import (
	"time"

	"github.com/google/uuid"
)

// TelegramPost represents a single channel post collected for analysis.
type TelegramPost struct {
	ID            uuid.UUID  `db:"id"             json:"id"`
	IntegrationID uuid.UUID  `db:"integration_id" json:"integration_id"`
	UserID        uuid.UUID  `db:"user_id"         json:"user_id"`
	MessageID     int64      `db:"message_id"      json:"message_id"`
	ChannelID     int64      `db:"channel_id"      json:"channel_id"`
	Text          *string    `db:"text"            json:"text,omitempty"`
	MediaType     *string    `db:"media_type"      json:"media_type,omitempty"`
	HasMedia      bool       `db:"has_media"       json:"has_media"`
	LinkCount     int        `db:"link_count"      json:"link_count"`
	ExtractedURLs []string   `db:"extracted_urls"  json:"extracted_urls,omitempty"`
	PostedAt      time.Time  `db:"posted_at"       json:"posted_at"`
	FetchedAt     time.Time  `db:"fetched_at"      json:"fetched_at"`
	ForwardCount  *int       `db:"forward_count"   json:"forward_count,omitempty"`
	ViewCount     *int       `db:"view_count"      json:"view_count,omitempty"`
}

// PostsResponse is returned by GET /telegram/posts.
type PostsResponse struct {
	Items  []*TelegramPost `json:"items"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

// TelegramUpdate is the Bot API Update object (minimal subset we handle).
type TelegramUpdate struct {
	UpdateID          int64            `json:"update_id"`
	ChannelPost       *TelegramMessage `json:"channel_post,omitempty"`
	EditedChannelPost *TelegramMessage `json:"edited_channel_post,omitempty"`
}

// TelegramMessage is a channel post from the Bot API.
type TelegramMessage struct {
	MessageID    int64            `json:"message_id"`
	Chat         TelegramChat     `json:"chat"`
	Date         int64            `json:"date"`   // Unix timestamp
	Text         string           `json:"text"`
	Caption      string           `json:"caption"`
	Photo        []interface{}    `json:"photo"`
	Video        interface{}      `json:"video"`
	Document     interface{}      `json:"document"`
	Audio        interface{}      `json:"audio"`
	Voice        interface{}      `json:"voice"`
	Poll         interface{}      `json:"poll"`
	Entities     []MessageEntity  `json:"entities"`
	CaptionEntities []MessageEntity `json:"caption_entities"`
	ForwardCount int              `json:"forward_count"`
	Views        int              `json:"views"`
}

// MessageEntity represents a Bot API entity (URL, text_link, etc.).
type MessageEntity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	URL    string `json:"url,omitempty"`
}
