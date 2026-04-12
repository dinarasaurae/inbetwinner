package models

import (
	"time"

	"github.com/google/uuid"
)

type DeviceToken struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Token     string    `json:"token"`
	Platform  string    `json:"platform"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type RegisterDeviceTokenRequest struct {
	Token    string `json:"token"`
	Platform string `json:"platform"` // "android" | "ios"
}
