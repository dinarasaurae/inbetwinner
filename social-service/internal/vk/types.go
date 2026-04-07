package vk

import (
	"encoding/json"
	"fmt"
	"strings"
)

// APIVersion is the VK API version used across all requests.
const APIVersion = "5.199"

// APIBase is the VK API base URL.
// Declared as var (not const) so integration tests can point it at an httptest
// server without modifying production code. Production value never changes.
var APIBase = "https://api.vk.com/method"

// VKIDAuthURL is the browser-facing VK ID authorization endpoint (opens in browser/WebView).
const VKIDAuthURL = "https://id.vk.com/authorize"

// VKIDTokenURL is the server-side token exchange endpoint (POST, PKCE).
const VKIDTokenURL = "https://id.vk.ru/oauth2/auth"

// VKIDBase kept for backwards compat references.
const VKIDBase = "https://id.vk.ru"

// OAuthBase is kept for group/community token flow (oauth.vk.com/access_token).
const OAuthBase = "https://oauth.vk.com"

// LongPollWait is used for Long Poll event polling (differs from main API).
const LongPollWait = 25 // seconds per LP request

// ─── API Response envelope ────────────────────────────────────────────────────

// Response is the top-level envelope returned by VK API methods.
type Response[T any] struct {
	Response T         `json:"response"`
	Error    *APIError `json:"error,omitempty"`
}

// APIErrorParam is a single key-value pair from the request_params VK debug field.
type APIErrorParam struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// APIError represents a VK API error.
type APIError struct {
	Code          int             `json:"error_code"`
	Message       string          `json:"error_msg"`
	RequestParams []APIErrorParam `json:"request_params,omitempty"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("vk error %d: %s", e.Code, e.Message)
}

// DebugString returns the full error including request_params for diagnostics.
func (e *APIError) DebugString() string {
	if len(e.RequestParams) == 0 {
		return e.Error()
	}
	params := make([]string, 0, len(e.RequestParams))
	for _, p := range e.RequestParams {
		if p.Key != "access_token" { // never log tokens
			params = append(params, p.Key+"="+p.Value)
		}
	}
	return fmt.Sprintf("vk error %d: %s | request_params: %s", e.Code, e.Message, strings.Join(params, " "))
}

// ─── Users ────────────────────────────────────────────────────────────────────

// UserFields is the comma-separated list of extra fields to request.
const UserFields = "bdate,sex,city,country,about,status,domain,photo_200,followers_count,occupation"

// User represents a VK user profile (subset of fields).
type User struct {
	ID             int64       `json:"id"`
	FirstName      string      `json:"first_name"`
	LastName       string      `json:"last_name"`
	Sex            int         `json:"sex"`
	BDate          string      `json:"bdate,omitempty"`
	City           *NamedItem  `json:"city,omitempty"`
	Country        *NamedItem  `json:"country,omitempty"`
	About          string      `json:"about,omitempty"`
	Status         string      `json:"status,omitempty"`
	Domain         string      `json:"domain,omitempty"`
	Photo200       string      `json:"photo_200,omitempty"`
	FollowersCount int         `json:"followers_count,omitempty"`
	Occupation     *Occupation `json:"occupation,omitempty"`
	Deactivated    string      `json:"deactivated,omitempty"` // "deleted" or "banned"
}

// NamedItem is a VK city/country object.
type NamedItem struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// Occupation represents work/education info.
type Occupation struct {
	Type string `json:"type"` // work / university / school
	Name string `json:"name"`
}

// ─── Groups ───────────────────────────────────────────────────────────────────

// GroupFields to fetch when listing admin groups or resolving a specific group.
const GroupFields = "photo_200,screen_name,members_count,is_admin,admin_level"

// Group represents a VK community/group.
type Group struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	ScreenName   string `json:"screen_name"`
	Photo200     string `json:"photo_200,omitempty"`
	IsClosed     int    `json:"is_closed"` // 0=open, 1=closed, 2=private
	MembersCount int    `json:"members_count,omitempty"`
	// AdminLevel: 1=moderator, 2=editor, 3=administrator (returned with filter=admin)
	AdminLevel int `json:"admin_level,omitempty"`
	IsAdmin    int `json:"is_admin"`
}

// GroupsGetResponse is returned by groups.get.
type GroupsGetResponse struct {
	Count int     `json:"count"`
	Items []Group `json:"items"`
}

// ─── Wall posts ───────────────────────────────────────────────────────────────

// WallPost is a single post on a VK wall.
type WallPost struct {
	ID          int64        `json:"id"`
	OwnerID     int64        `json:"owner_id"`
	FromID      int64        `json:"from_id"`
	Date        int64        `json:"date"` // Unix timestamp
	Text        string       `json:"text"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Likes       CountItem    `json:"likes"`
	Reposts     CountItem    `json:"reposts"`
	Views       CountItem    `json:"views"`
	Comments    CountItem    `json:"comments"`
}

// Attachment wraps any attached media.
type Attachment struct {
	Type string `json:"type"` // photo, video, audio, doc, link, market, poll, ...
}

// CountItem wraps a VK count field.
type CountItem struct {
	Count int `json:"count"`
}

// WallGetResponse is returned by wall.get.
type WallGetResponse struct {
	Count int        `json:"count"`
	Items []WallPost `json:"items"`
}

// ─── Subscriptions ────────────────────────────────────────────────────────────

// SubscriptionsResponse is returned by users.getSubscriptions with extended=0.
type SubscriptionsResponse struct {
	Users  SubscriptionList `json:"users"`
	Groups SubscriptionList `json:"groups"`
}

// SubscriptionsExtendedResponse is returned with extended=1 — mixed list of
// users and groups the authenticated user follows.
type SubscriptionsExtendedResponse struct {
	Count int                    `json:"count"`
	Items []SubscriptionItem     `json:"items"`
}

// SubscriptionList is a paginated list of IDs.
type SubscriptionList struct {
	Count int     `json:"count"`
	Items []int64 `json:"items"`
}

// SubscriptionItem is one entry in the extended subscriptions list.
// It can be either a user profile or a group/public page.
type SubscriptionItem struct {
	ID         int64  `json:"id"`
	Type       string `json:"type"`        // "profile" | "group" | "page" | "event"
	Name       string `json:"name"`        // group name (if type != "profile")
	FirstName  string `json:"first_name"`  // user first name
	LastName   string `json:"last_name"`   // user last name
	ScreenName string `json:"screen_name"`
	Photo200   string `json:"photo_200,omitempty"`
	IsAdmin    int    `json:"is_admin,omitempty"`
	AdminLevel int    `json:"admin_level,omitempty"`
}

// ─── Messages ─────────────────────────────────────────────────────────────────

// MessageSendResponse is the message_id returned by messages.send.
type MessageSendResponse int64

// ─── Long Poll ────────────────────────────────────────────────────────────────

// LongPollServer is returned by groups.getLongPollServer.
type LongPollServer struct {
	Key    string `json:"key"`
	Server string `json:"server"`
	Ts     string `json:"ts"`
}

// LongPollServerResponse wraps LongPollServer in the standard envelope.
type LongPollServerResponse struct {
	Response LongPollServer `json:"response"`
	Error    *APIError      `json:"error,omitempty"`
}

// LongPollUpdate is returned by polling the LP server.
type LongPollUpdate struct {
	Ts      string          `json:"ts"`
	Updates [][]interface{} `json:"updates,omitempty"` // legacy format for groups
	Failed  int             `json:"failed,omitempty"`
}

// GroupEvent is a single event from the groups Long Poll (v2 format).
type GroupEvent struct {
	Type    string          `json:"type"`
	Object  json.RawMessage `json:"object"`
	GroupID int64           `json:"group_id"`
}

// GroupLongPollUpdate is the v2 Long Poll response format.
type GroupLongPollUpdate struct {
	Ts      string       `json:"ts"`
	Updates []GroupEvent `json:"updates,omitempty"`
	Failed  int          `json:"failed,omitempty"`
}

// MessageNew is the object inside a message_new event.
type MessageNew struct {
	Message    IncomingMessage        `json:"message"`
	ClientInfo map[string]interface{} `json:"client_info,omitempty"`
}

// IncomingMessage represents an inbound VK message.
type IncomingMessage struct {
	ID                    int64        `json:"id"`
	Date                  int64        `json:"date"`
	FromID                int64        `json:"from_id"`
	PeerID                int64        `json:"peer_id"`
	Text                  string       `json:"text"`
	Attachments           []Attachment `json:"attachments,omitempty"`
	ConversationMessageID int64        `json:"conversation_message_id,omitempty"`
}
