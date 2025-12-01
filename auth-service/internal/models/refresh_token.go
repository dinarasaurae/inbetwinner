package models

import (
	"time"

	"github.com/google/uuid"
)

type RefreshToken struct {
	ID         uuid.UUID `json:"id" db:"id"`
	UserID     uuid.UUID `json:"user_id" db:"user_id"`
	TokenHash  string    `json:"-" db:"token_hash"`
	ExpiresAt  time.Time `json:"expires_at" db:"expires_at"`
	DeviceInfo *string   `json:"device_info" db:"device_info"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

type CreateRefreshTokenRequest struct {
	UserID     uuid.UUID `json:"user_id" validate:"required"`
	Token      string    `json:"token" validate:"required"`
	ExpiresAt  time.Time `json:"expires_at" validate:"required"`
	DeviceInfo *string   `json:"device_info"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}
