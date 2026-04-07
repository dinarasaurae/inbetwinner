package models

import (
	"time"

	"github.com/google/uuid"
)

type VKMethodCapability struct {
	Method       string `json:"method"`
	Supported    bool   `json:"supported"`
	ErrorCode    int    `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

type VKDiscoveryStatus struct {
	TokenPlatform                        string             `json:"token_platform"`
	TokenScope                           string             `json:"token_scope"`
	AdminGroups                          VKMethodCapability `json:"admin_groups"`
	Subscriptions                        VKMethodCapability `json:"subscriptions"`
	CanInferAdminGroupsFromSubscriptions bool               `json:"can_infer_admin_groups_from_subscriptions"`
}

type VKSubscriptionSummary struct {
	Count         int      `json:"count"`
	GroupsCount   int      `json:"groups_count"`
	ProfilesCount int      `json:"profiles_count"`
	SampleNames   []string `json:"sample_names"`
}

type VKUserDataSnapshot struct {
	TokenPlatform        string                 `json:"token_platform"`
	TokenScope           string                 `json:"token_scope"`
	Profile              *VKUserProfile         `json:"profile,omitempty"`
	Posts                VKMethodCapability     `json:"posts"`
	PostsCount           int                    `json:"posts_count"`
	PostTopics           []string               `json:"post_topics"`
	Subscriptions        VKMethodCapability     `json:"subscriptions"`
	SubscriptionsSummary *VKSubscriptionSummary `json:"subscriptions_summary,omitempty"`
	Notes                []string               `json:"notes"`
}

type VKContextBootstrapRequest struct {
	IntegrationID        string `json:"integration_id"`
	Count                int    `json:"count"`
	IncludeSubscriptions bool   `json:"include_subscriptions"`
}

type VKBusinessSnapshot struct {
	Summary                string   `json:"summary"`
	TwinStage              string   `json:"twin_stage"`
	Positioning            string   `json:"positioning,omitempty"`
	AudienceSummary        string   `json:"audience_summary,omitempty"`
	OfferSignals           []string `json:"offer_signals"`
	ContentSignals         []string `json:"content_signals"`
	KnowledgeSignals       []string `json:"knowledge_signals"`
	MissingSignals         []string `json:"missing_signals"`
	RecommendedActions     []string `json:"recommended_actions"`
	CommunityAccessEnabled bool     `json:"community_access_enabled"`
	LongPollEnabled        bool     `json:"long_poll_enabled"`
	ReadyForConversations  bool     `json:"ready_for_conversations"`
}

type VKContextBootstrapData struct {
	IntegrationID        string                 `json:"integration_id"`
	GroupID              int64                  `json:"group_id"`
	GroupName            string                 `json:"group_name"`
	GroupScreenName      string                 `json:"group_screen_name"`
	ImportedPosts        int                    `json:"imported_posts"`
	TotalPosts           int                    `json:"total_posts"`
	MessageCount         int                    `json:"message_count"`
	LeadCount            int                    `json:"lead_count"`
	ContextReady         bool                   `json:"context_ready"`
	TopTopics            []string               `json:"top_topics"`
	OwnerProfile         *VKUserProfile         `json:"owner_profile,omitempty"`
	SubscriptionsSummary *VKSubscriptionSummary `json:"subscriptions_summary,omitempty"`
	SubscriptionsNote    string                 `json:"subscriptions_note,omitempty"`
	BusinessSnapshot     *VKBusinessSnapshot    `json:"business_snapshot,omitempty"`
	SetupWarnings        []string               `json:"setup_warnings"`
}

type VKAgentSettings struct {
	ID                uuid.UUID `json:"id"`
	IntegrationID     uuid.UUID `json:"integration_id"`
	DraftFirst        bool      `json:"draft_first"`
	AutoReplyEnabled  bool      `json:"auto_reply_enabled"`
	SafeIntents       []string  `json:"safe_intents"`
	ToneOfVoice       string    `json:"tone_of_voice"`
	ForbiddenPromises []string  `json:"forbidden_promises"`
	EscalationPolicy  string    `json:"escalation_policy"`
	RAGEnabled        bool      `json:"rag_enabled"`
	// OrchestrationMode overrides the service-level LLM_ORCHESTRATION_MODE for
	// this specific integration: "legacy" | "llm_service" | "hybrid".
	// Empty string means "use service default".
	OrchestrationMode string    `json:"orchestration_mode"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type VKAgentSettingsUpdateRequest struct {
	IntegrationID     string   `json:"integration_id"`
	DraftFirst        bool     `json:"draft_first"`
	AutoReplyEnabled  bool     `json:"auto_reply_enabled"`
	SafeIntents       []string `json:"safe_intents"`
	ToneOfVoice       string   `json:"tone_of_voice"`
	ForbiddenPromises []string `json:"forbidden_promises"`
	EscalationPolicy  string   `json:"escalation_policy"`
	RAGEnabled        bool     `json:"rag_enabled"`
}

type VKDraftStatus string

const (
	VKDraftStatusPending  VKDraftStatus = "pending"
	VKDraftStatusApproved VKDraftStatus = "approved"
	VKDraftStatusSent     VKDraftStatus = "sent"
	VKDraftStatusAutoSent VKDraftStatus = "auto_sent"
	VKDraftStatusSkipped  VKDraftStatus = "skipped"
	VKDraftStatusFailed   VKDraftStatus = "failed"
)

type VKReplyDraft struct {
	ID                uuid.UUID     `json:"id"`
	IntegrationID     uuid.UUID     `json:"integration_id"`
	InboundMessageID  uuid.UUID     `json:"inbound_message_id"`
	FromVKUserID      int64         `json:"from_vk_user_id"`
	PeerID            *int64        `json:"peer_id,omitempty"`
	Intent            string        `json:"intent"`
	Confidence        float64       `json:"confidence"`
	SafeIntent        bool          `json:"safe_intent"`
	Status            VKDraftStatus `json:"status"`
	Source            string        `json:"source"`
	DraftText         string        `json:"draft_text"`
	Rationale         string        `json:"rationale"`
	KnowledgeSnippets []string      `json:"knowledge_snippets"`
	SentMessageID     *int64        `json:"sent_message_id,omitempty"`
	ApprovedBy        *uuid.UUID    `json:"approved_by,omitempty"`
	GeneratedAt       time.Time     `json:"generated_at"`
	ApprovedAt        *time.Time    `json:"approved_at,omitempty"`
	SentAt            *time.Time    `json:"sent_at,omitempty"`

	// Observability — which path produced this draft and how long it took.
	OrchestrationSource string     `json:"orchestration_source"` // legacy | llm_service | fallback_legacy
	LLMLatencyMs        *int       `json:"llm_latency_ms,omitempty"`
	FallbackReason      *string    `json:"fallback_reason,omitempty"`
	PromptTokens        *int       `json:"prompt_tokens,omitempty"`
	CompletionTokens    *int       `json:"completion_tokens,omitempty"`
	UsedAgentID         *uuid.UUID `json:"used_agent_id,omitempty"`
	UsedTools           []string   `json:"used_tools"`
}

// VKOrchestrationDecision is the structured response from llm-service
// social orchestration endpoint (/llm/social/vk/process).
type VKOrchestrationDecision struct {
	// Mode indicates the recommended action.
	//   draft       — store as pending draft, await human approval
	//   auto_reply  — safe to send automatically
	//   escalate    — hand off to human, do not send
	Mode              string     `json:"mode"`
	DraftText         string     `json:"draft_text"`
	Confidence        float64    `json:"confidence"`
	Intent            string     `json:"intent"`
	SafeIntent        bool       `json:"safe_intent"`
	Rationale         string     `json:"rationale"`
	KnowledgeSnippets []string   `json:"knowledge_snippets"`
	UsedTools         []string   `json:"used_tools,omitempty"`
	PromptTokens      int        `json:"prompt_tokens"`
	CompletionTokens  int        `json:"completion_tokens"`
	TokensUsed        int        `json:"tokens_used"`
	AgentID           *uuid.UUID `json:"agent_id,omitempty"`
}

type VKGenerateDraftRequest struct {
	IntegrationID string `json:"integration_id"`
	MessageID     string `json:"message_id"`
	Force         bool   `json:"force"`
}

type VKApproveDraftRequest struct {
	TextOverride string `json:"text_override"`
}

type VKWorkspacePost struct {
	ID            uuid.UUID `json:"id"`
	VKPostID      int64     `json:"vk_post_id"`
	Text          string    `json:"text,omitempty"`
	HasMedia      bool      `json:"has_media"`
	MediaType     string    `json:"media_type,omitempty"`
	LikesCount    int       `json:"likes_count"`
	CommentsCount int       `json:"comments_count"`
	PostedAt      time.Time `json:"posted_at"`
}

type VKWorkspaceMessage struct {
	ID           uuid.UUID     `json:"id"`
	FromVKUserID int64         `json:"from_vk_user_id"`
	PeerID       *int64        `json:"peer_id,omitempty"`
	Text         string        `json:"text,omitempty"`
	IsIncoming   bool          `json:"is_incoming"`
	IsProcessed  bool          `json:"is_processed"`
	ReceivedAt   time.Time     `json:"received_at"`
	Draft        *VKReplyDraft `json:"draft,omitempty"`
}

type VKWorkspaceLead struct {
	ID             uuid.UUID  `json:"id"`
	VKUserID       int64      `json:"vk_user_id"`
	FirstName      string     `json:"first_name"`
	LastName       string     `json:"last_name"`
	City           string     `json:"city,omitempty"`
	Country        string     `json:"country,omitempty"`
	About          string     `json:"about,omitempty"`
	Status         string     `json:"status,omitempty"`
	FollowersCount int        `json:"followers_count"`
	LastEnrichedAt *time.Time `json:"last_enriched_at,omitempty"`
}

type VKIntentConfidence struct {
	Intent        string  `json:"intent"`
	AvgConfidence float64 `json:"avg_confidence"`
	Count         int     `json:"count"`
}

type VKWorkspaceAnalytics struct {
	IncomingMessages         int                  `json:"incoming_messages"`
	OutgoingMessages         int                  `json:"outgoing_messages"`
	PendingDrafts            int                  `json:"pending_drafts"`
	Leads                    int                  `json:"leads"`
	FirstResponseTimeMinutes float64              `json:"first_response_time_minutes"`
	HandoffRate              float64              `json:"handoff_rate"`
	LeadConversionRate       float64              `json:"lead_conversion_rate"`
	ConfidenceByIntent       []VKIntentConfidence `json:"confidence_by_intent"`
}

type VKWorkspaceData struct {
	Integration      *VKIntegration       `json:"integration"`
	BusinessSnapshot *VKBusinessSnapshot  `json:"business_snapshot,omitempty"`
	RecentPosts      []VKWorkspacePost    `json:"recent_posts"`
	RecentMessages   []VKWorkspaceMessage `json:"recent_messages"`
	Leads            []VKWorkspaceLead    `json:"leads"`
	Drafts           []VKReplyDraft       `json:"drafts"`
	Recommendations  []string             `json:"recommendations"`
	Analytics        VKWorkspaceAnalytics `json:"analytics"`
	AgentSettings    *VKAgentSettings     `json:"agent_settings,omitempty"`
}
