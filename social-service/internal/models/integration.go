package models

import (
	"time"

	"github.com/google/uuid"
)

// TelegramIntegration represents a connected Telegram channel.
type TelegramIntegration struct {
	ID              uuid.UUID  `db:"id"`
	UserID          uuid.UUID  `db:"user_id"`
	ConsentID       uuid.UUID  `db:"consent_id"`
	ChannelID       int64      `db:"channel_id"`
	ChannelUsername *string    `db:"channel_username"`
	ChannelTitle    string     `db:"channel_title"`
	BotTokenEnc     []byte     `db:"bot_token_enc"` // never serialised to JSON
	BotTokenIV      []byte     `db:"bot_token_iv"`  // never serialised to JSON
	Status          string     `db:"status"`
	LastSyncedAt    *time.Time `db:"last_synced_at"`
	LastMessageID   *int64     `db:"last_message_id"`
	SyncError       *string    `db:"sync_error"`
	ConnectedAt     time.Time  `db:"connected_at"`
	DisconnectedAt  *time.Time `db:"disconnected_at"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at"`
}

// ToResponse converts an integration to its safe JSON representation.
func (i *TelegramIntegration) ToResponse() IntegrationResponse {
	return IntegrationResponse{
		ID:              i.ID,
		ChannelID:       i.ChannelID,
		ChannelUsername: i.ChannelUsername,
		ChannelTitle:    i.ChannelTitle,
		Status:          i.Status,
		LastSyncedAt:    i.LastSyncedAt,
		ConnectedAt:     i.ConnectedAt,
	}
}

// IntegrationResponse is the public JSON shape (no secrets).
type IntegrationResponse struct {
	ID              uuid.UUID  `json:"id"`
	ChannelID       int64      `json:"channel_id"`
	ChannelUsername *string    `json:"channel_username,omitempty"`
	ChannelTitle    string     `json:"channel_title"`
	Status          string     `json:"status"`
	LastSyncedAt    *time.Time `json:"last_synced_at,omitempty"`
	ConnectedAt     time.Time  `json:"connected_at"`
}

// UserConsent records an explicit data-collection consent.
type UserConsent struct {
	ID          uuid.UUID  `db:"id"`
	UserID      uuid.UUID  `db:"user_id"`
	Platform    string     `db:"platform"`
	Scope       []string   `db:"scope"`
	GrantedAt   time.Time  `db:"granted_at"`
	RevokedAt   *time.Time `db:"revoked_at"`
	ConsentText string     `db:"consent_text"`
	IPAddress   *string    `db:"ip_address"`
	UserAgent   *string    `db:"user_agent"`
}

// ConnectTelegramRequest is the payload for POST /telegram/connect.
// The hash and auth_date fields come from the Telegram Login Widget.
type ConnectTelegramRequest struct {
	// Telegram Login Widget fields
	ID        int64  `json:"id"         validate:"required"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
	PhotoURL  string `json:"photo_url"`
	AuthDate  int64  `json:"auth_date"  validate:"required"`
	Hash      string `json:"hash"       validate:"required"`

	// Channel to connect (must be administered by the user)
	ChannelUsername string `json:"channel_username" validate:"required"`

	// Must be true; frontend shows consent text before the user submits.
	ConsentAcknowledged bool `json:"consent_acknowledged" validate:"required"`
}

// StatusResponse is returned by GET /telegram/status.
type StatusResponse struct {
	Connected        bool       `json:"connected"`
	ChannelID        *int64     `json:"channel_id,omitempty"`
	ChannelUsername  *string    `json:"channel_username,omitempty"`
	ChannelTitle     *string    `json:"channel_title,omitempty"`
	IntegrationStatus *string   `json:"integration_status,omitempty"`
	LastSyncedAt     *time.Time `json:"last_synced_at,omitempty"`
	ConsentGrantedAt *time.Time `json:"consent_granted_at,omitempty"`
}

// TelegramBotUser is the result of getMe Bot API call.
type TelegramBotUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

// TelegramChat is the minimal chat/channel info from getChat.
type TelegramChat struct {
	ID       int64   `json:"id"`
	Type     string  `json:"type"`
	Title    string  `json:"title"`
	Username *string `json:"username"`
}

// TelegramChatMember is the result of getChatMember.
type TelegramChatMember struct {
	Status string          `json:"status"`
	User   TelegramBotUser `json:"user"`
}

// BotAPIResponse is the generic envelope for Telegram Bot API responses.
type BotAPIResponse struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description,omitempty"`
	Result      interface{}     `json:"result,omitempty"`
}
