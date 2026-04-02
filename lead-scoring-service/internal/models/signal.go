package models

import (
	"time"

	"github.com/google/uuid"
)

type BehavioralSignal struct {
	ID          uuid.UUID `json:"id"`
	WorkspaceID uuid.UUID `json:"workspace_id"`
	LeadID      string    `json:"lead_id"`
	SignalType  string    `json:"signal_type"`
	Weight      int       `json:"weight"`
	Message     string    `json:"message"`
	CreatedAt   time.Time `json:"created_at"`
}

// Типы сигналов и их веса
const (
	SignalAskedPrice       = "asked_price"        // +10
	SignalAskedTimeline    = "asked_timeline"      // +8
	SignalMentionedBudget  = "mentioned_budget"    // +12
	SignalRequestedDemo    = "requested_demo"      // +15
	SignalMentionedCompany = "mentioned_company"   // +5
	SignalLongMessage      = "long_message"        // +3
	SignalPositiveSentiment = "positive_sentiment" // +5
	SignalNegativeSentiment = "negative_sentiment" // -15
	SignalQuestion         = "question"            // +3
)
