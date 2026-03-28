package models

import (
	"time"

	"github.com/google/uuid"
)

// ─── Integrations ─────────────────────────────────────────────────────────────

// VKUserConnection stores the user-level OAuth token obtained when the user
// authorises inBeTwin in VK.  Used to list admin groups and scrape profile data.
type VKUserConnection struct {
	ID             uuid.UUID `json:"id"`
	UserID         uuid.UUID `json:"user_id"`
	VKUserID       int64     `json:"vk_user_id"`
	Platform       string    `json:"platform"` // web | android | ios
	AccessTokenEnc []byte    `json:"-"`
	AccessTokenIV  []byte    `json:"-"`
	Scope          string    `json:"scope"`
	ConnectedAt    time.Time `json:"connected_at"`
}

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

// ─── Data objects ─────────────────────────────────────────────────────────────

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

// VKAdminGroup is returned when listing groups where the user is an admin,
// before the group OAuth is completed.
type VKAdminGroup struct {
	GroupID      int64  `json:"group_id"`
	Name         string `json:"name"`
	ScreenName   string `json:"screen_name"`
	Photo        string `json:"photo,omitempty"`
	MembersCount int    `json:"members_count"`
	// IsConnected is true when the group is already connected in inBeTwin.
	IsConnected bool `json:"is_connected"`
}

// VKUserProfile is the authenticated user's own VK profile.
type VKUserProfile struct {
	VKUserID       int64  `json:"vk_user_id"`
	FirstName      string `json:"first_name"`
	LastName       string `json:"last_name"`
	Sex            int    `json:"sex,omitempty"`
	BDate          string `json:"bdate,omitempty"`
	City           string `json:"city,omitempty"`
	Country        string `json:"country,omitempty"`
	About          string `json:"about,omitempty"`
	Status         string `json:"status,omitempty"`
	Domain         string `json:"domain,omitempty"`
	PhotoURL       string `json:"photo_url,omitempty"`
	FollowersCount int    `json:"followers_count"`
	OccupationType string `json:"occupation_type,omitempty"`
	OccupationName string `json:"occupation_name,omitempty"`
}

// ─── Request / Response DTOs ──────────────────────────────────────────────────

// VKUserOAuthStartResponse is returned by GET /vk/oauth/user/start.
type VKUserOAuthStartResponse struct {
	AuthURL      string `json:"auth_url"`
	State        string `json:"state"`
	Platform     string `json:"platform"`
	ImplicitFlow bool   `json:"implicit_flow"` // true → response_type=token, no secret needed
}

// VKUserOAuthExchangeRequest is sent by the mobile app to complete user OAuth.
//
// VK ID OAuth 2.1 (PKCE): code + state + device_id + platform
// device_id is returned by VK in the redirect URI alongside the code.
type VKUserOAuthExchangeRequest struct {
	Code     string `json:"code"`
	State    string `json:"state"`
	DeviceID string `json:"device_id"`          // VK ID PKCE — returned in callback
	Platform string `json:"platform"`            // android | ios
}

// ConnectVKRequest is sent by the mobile app to complete group OAuth.
type ConnectVKRequest struct {
	Code    string `json:"code"`
	State   string `json:"state"`
	GroupID int64  `json:"group_id"`
}

// OAuthStartResponse is returned by GET /vk/oauth/start (group OAuth).
type OAuthStartResponse struct {
	AuthURL  string `json:"auth_url"`
	State    string `json:"state"`
	Platform string `json:"platform"`
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
