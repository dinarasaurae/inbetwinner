package models

import (
	"time"

	"github.com/google/uuid"
)

type OAuthProvider struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	UserID         uuid.UUID  `json:"user_id" db:"user_id"`
	Provider       string     `json:"provider" db:"provider"`
	ProviderUserID string     `json:"provider_user_id" db:"provider_user_id"`
	AccessToken    *string    `json:"-" db:"access_token"`
	RefreshToken   *string    `json:"-" db:"refresh_token"`
	TokenExpiresAt *time.Time `json:"token_expires_at" db:"token_expires_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

type CreateOAuthProviderRequest struct {
	UserID         uuid.UUID  `json:"user_id" validate:"required"`
	Provider       string     `json:"provider" validate:"required,oneof=google yandex"`
	ProviderUserID string     `json:"provider_user_id" validate:"required"`
	AccessToken    *string    `json:"access_token"`
	RefreshToken   *string    `json:"refresh_token"`
	TokenExpiresAt *time.Time `json:"token_expires_at"`
}

type OAuthUserInfo struct {
	ID            string  `json:"id"`
	Email         string  `json:"email"`
	FirstName     *string `json:"first_name"`
	LastName      *string `json:"last_name"`
	AvatarURL     *string `json:"avatar_url"`
	EmailVerified bool    `json:"email_verified"`
}

type GoogleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
	EmailVerified bool   `json:"email_verified"`
	Locale        string `json:"locale"`
}

func (g *GoogleUserInfo) ToOAuthUserInfo() OAuthUserInfo {
	var firstName, lastName, avatarURL *string

	if g.GivenName != "" {
		firstName = &g.GivenName
	}
	if g.FamilyName != "" {
		lastName = &g.FamilyName
	}
	if g.Picture != "" {
		avatarURL = &g.Picture
	}

	return OAuthUserInfo{
		ID:            g.ID,
		Email:         g.Email,
		FirstName:     firstName,
		LastName:      lastName,
		AvatarURL:     avatarURL,
		EmailVerified: g.EmailVerified,
	}
}

type YandexUserInfo struct {
	ID            string `json:"id"`
	Login         string `json:"login"`
	Email         string `json:"default_email"`
	FirstName     string `json:"first_name"`
	LastName      string `json:"last_name"`
	DisplayName   string `json:"display_name"`
	AvatarID      string `json:"default_avatar_id"`
	IsAvatarEmpty bool   `json:"is_avatar_empty"`
}

func (y *YandexUserInfo) ToOAuthUserInfo() OAuthUserInfo {
	var firstName, lastName, avatarURL *string

	if y.FirstName != "" {
		firstName = &y.FirstName
	}
	if y.LastName != "" {
		lastName = &y.LastName
	}
	if !y.IsAvatarEmpty && y.AvatarID != "" {
		avatar := "https://avatars.yandex.net/get-yapic/" + y.AvatarID + "/islands-200"
		avatarURL = &avatar
	}

	return OAuthUserInfo{
		ID:            y.ID,
		Email:         y.Email,
		FirstName:     firstName,
		LastName:      lastName,
		AvatarURL:     avatarURL,
		EmailVerified: true,
	}
}
