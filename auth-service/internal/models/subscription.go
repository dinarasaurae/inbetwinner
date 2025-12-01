package models

import (
	"time"

	"github.com/google/uuid"
)

type Subscription struct {
	ID                     uuid.UUID  `json:"id" db:"id"`
	UserID                 uuid.UUID  `json:"user_id" db:"user_id"`
	Plan                   string     `json:"plan" db:"plan"`     // 'free', 'pro', 'business'
	Status                 string     `json:"status" db:"status"` // 'active', 'cancelled', 'expired', 'past_due', 'trialing'
	StartedAt              time.Time  `json:"started_at" db:"started_at"`
	ExpiresAt              *time.Time `json:"expires_at" db:"expires_at"`
	PaymentProvider        *string    `json:"payment_provider" db:"payment_provider"` // 'stripe', 'yookassa'
	ExternalSubscriptionID *string    `json:"external_subscription_id" db:"external_subscription_id"`
	TrialEndsAt            *time.Time `json:"trial_ends_at" db:"trial_ends_at"`
	CreatedAt              time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at" db:"updated_at"`
}

type CreateSubscriptionRequest struct {
	UserID                 uuid.UUID  `json:"user_id" validate:"required"`
	Plan                   string     `json:"plan" validate:"required,oneof=free pro business"`
	Status                 string     `json:"status" validate:"required,oneof=active cancelled expired past_due trialing"`
	ExpiresAt              *time.Time `json:"expires_at"`
	PaymentProvider        *string    `json:"payment_provider" validate:"omitempty,oneof=stripe yookassa"`
	ExternalSubscriptionID *string    `json:"external_subscription_id"`
	TrialEndsAt            *time.Time `json:"trial_ends_at"`
}

type UpdateSubscriptionRequest struct {
	Plan                   *string    `json:"plan" validate:"omitempty,oneof=free pro business"`
	Status                 *string    `json:"status" validate:"omitempty,oneof=active cancelled expired past_due trialing"`
	ExpiresAt              *time.Time `json:"expires_at"`
	PaymentProvider        *string    `json:"payment_provider" validate:"omitempty,oneof=stripe yookassa"`
	ExternalSubscriptionID *string    `json:"external_subscription_id"`
	TrialEndsAt            *time.Time `json:"trial_ends_at"`
}

type SubscriptionPlan struct {
	Name          string   `json:"name"`
	Price         int      `json:"price"`
	Currency      string   `json:"currency"`
	MessagesLimit *int     `json:"messages_limit"`
	TrialDays     int      `json:"trial_days"`
	Features      []string `json:"features"`
}

func GetAvailablePlans() map[string]SubscriptionPlan {
	return map[string]SubscriptionPlan{
		"free": {
			Name:          "Free",
			Price:         0,
			Currency:      "USD",
			MessagesLimit: intPtr(100),
			TrialDays:     7,
			Features:      []string{"Basic AI agent", "100 messages/month", "Email support"},
		},
		"pro": {
			Name:          "Pro",
			Price:         4900,
			Currency:      "USD",
			MessagesLimit: nil,
			TrialDays:     14,
			Features:      []string{"Advanced AI agent", "Unlimited messages", "Priority support", "Custom integrations"},
		},
		"business": {
			Name:          "Business",
			Price:         9900,
			Currency:      "USD",
			MessagesLimit: nil,
			TrialDays:     14,
			Features:      []string{"Enterprise AI agent", "Unlimited messages", "24/7 support", "API access", "Custom tools", "Analytics dashboard"},
		},
	}
}

func intPtr(i int) *int {
	return &i
}
