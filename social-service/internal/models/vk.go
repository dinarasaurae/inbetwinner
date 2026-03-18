package models

import (
	"time"

	"github.com/google/uuid"
)

// VKIntegration represents a connected VK community (group).
type VKIntegration struct {
	ID              uuid.UUID `json:"id"`
	UserID          uuid.UUID `json:"user_id"`
	GroupID         int64     `json:"group_id"`
	GroupTokenEnc   []byte    `json:"-"`
	GroupTokenIV    []byte    `json:"-"`
	GroupName       string    `json:"group_name"`
	GroupScreenName string    `json:"group_screen_name"`
	GroupPhoto      string    `json:"group_photo"`
	LongPollTs      string    `json:"-"`
	IsActive        bool      `json:"is_active"`
	ConnectedAt     time.Time `json:"connected_at"`
}

// VKPost is a post from the connected VK group's wall.
type VKPost struct {
	ID            uuid.UUID `json:"id"`
	IntegrationID uuid.UUID `json:"integration_id"`
	UserID        uuid.UUID `json:"user_id"`
	VKPostID      int64     `json:"vk_post_id"`
	OwnerID       int64     `json:"owner_id"`
	Text          *string   `json:"text,omitempty"`
	MediaType     *string   `json:"media_type,omitempty"`
	HasMedia      bool      `json:"has_media"`
	LikesCount    *int      `json:"likes_count,omitempty"`
	RepostsCount  *int      `json:"reposts_count,omitempty"`
	ViewsCount    *int      `json:"views_count,omitempty"`
	CommentsCount *int      `json:"comments_count,omitempty"`
	ExtractedURLs []string  `json:"extracted_urls"`
	PostedAt      time.Time `json:"posted_at"`
}

// VKMessage is an inbound DM or outbound reply for a VK group.
type VKMessage struct {
	ID                    uuid.UUID `json:"id"`
	IntegrationID         uuid.UUID `json:"integration_id"`
	FromVKUserID          int64     `json:"from_vk_user_id"`
	MessageID             int64     `json:"message_id"`
	ConversationMessageID *int64    `json:"conversation_message_id,omitempty"`
	Text                  *string   `json:"text,omitempty"`
	IsIncoming            bool      `json:"is_incoming"`
	IsProcessed           bool      `json:"is_processed"`
	ReceivedAt            time.Time `json:"received_at"`
}

// VKLeadProfile is the digital twin of a lead built from their public VK data.
type VKLeadProfile struct {
	ID             uuid.UUID  `json:"id"`
	IntegrationID  uuid.UUID  `json:"integration_id"`
	VKUserID       int64      `json:"vk_user_id"`
	FirstName      string     `json:"first_name"`
	LastName       string     `json:"last_name"`
	Sex            int        `json:"sex"`
	BDate          string     `json:"bdate,omitempty"`
	City           string     `json:"city,omitempty"`
	Country        string     `json:"country,omitempty"`
	About          string     `json:"about,omitempty"`
	Status         string     `json:"status,omitempty"`
	Domain         string     `json:"domain,omitempty"`
	PhotoURL       string     `json:"photo_url,omitempty"`
	FollowersCount int        `json:"followers_count"`
	OccupationType string     `json:"occupation_type,omitempty"`
	OccupationName string     `json:"occupation_name,omitempty"`
	LastEnrichedAt *time.Time `json:"last_enriched_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// ConnectVKRequest is sent by the mobile app to complete OAuth.
type ConnectVKRequest struct {
	Code    string `json:"code"`
	State   string `json:"state"`
	GroupID int64  `json:"group_id"`
}

// OAuthStartResponse is returned by GET /vk/oauth/start.
type OAuthStartResponse struct {
	AuthURL string `json:"auth_url"`
	State   string `json:"state"`
}

// SyncVKRequest requests a wall post import.
type SyncVKRequest struct {
	IntegrationID string `json:"integration_id"`
	Count         int    `json:"count"` // max 100, default 50
}

// VKSendMessageRequest sends a DM to a lead from the group.
type VKSendMessageRequest struct {
	IntegrationID string `json:"integration_id"`
	ToVKUserID    int64  `json:"to_vk_user_id"`
	Text          string `json:"text"`
}
