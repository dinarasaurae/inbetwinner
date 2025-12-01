package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID               uuid.UUID `json:"id" db:"id"`
	Email            string    `json:"email" db:"email"`
	PasswordHash     *string   `json:"-" db:"password_hash"` // может быть nil для OAuth
	FirstName        *string   `json:"first_name" db:"first_name"`
	LastName         *string   `json:"last_name" db:"last_name"`
	AvatarURL        *string   `json:"avatar_url" db:"avatar_url"`
	EmailVerified    bool      `json:"email_verified" db:"email_verified"`
	SubscriptionPlan string    `json:"subscription_plan" db:"subscription_plan"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

type CreateUserRequest struct {
	Email     string  `json:"email" validate:"required,email"`
	Password  string  `json:"password" validate:"required,min=6"`
	FirstName *string `json:"first_name" validate:"omitempty,min=1,max=100"`
	LastName  *string `json:"last_name" validate:"omitempty,min=1,max=100"`
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type UpdateUserRequest struct {
	FirstName *string `json:"first_name" validate:"omitempty,min=1,max=100"`
	LastName  *string `json:"last_name" validate:"omitempty,min=1,max=100"`
	AvatarURL *string `json:"avatar_url" validate:"omitempty,url"`
}

type UserResponse struct {
	ID               uuid.UUID `json:"id"`
	Email            string    `json:"email"`
	FirstName        *string   `json:"first_name"`
	LastName         *string   `json:"last_name"`
	AvatarURL        *string   `json:"avatar_url"`
	EmailVerified    bool      `json:"email_verified"`
	SubscriptionPlan string    `json:"subscription_plan"`
	CreatedAt        time.Time `json:"created_at"`
}

func (u *User) ToResponse() UserResponse {
	return UserResponse{
		ID:               u.ID,
		Email:            u.Email,
		FirstName:        u.FirstName,
		LastName:         u.LastName,
		AvatarURL:        u.AvatarURL,
		EmailVerified:    u.EmailVerified,
		SubscriptionPlan: u.SubscriptionPlan,
		CreatedAt:        u.CreatedAt,
	}
}
