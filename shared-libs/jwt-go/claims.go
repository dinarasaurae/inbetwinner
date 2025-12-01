package jwt

import (
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	UserID           uuid.UUID `json:"user_id"`
	Email            string    `json:"email,omitempty"`
	SubscriptionPlan string    `json:"subscription_plan,omitempty"`
	TokenType        string    `json:"token_type"`
	jwt.RegisteredClaims
}
