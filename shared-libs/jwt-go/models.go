package jwt

import (
	"github.com/google/uuid"
)

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type AuthResponse struct {
	Success      bool        `json:"success"`
	Message      string      `json:"message,omitempty"`
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	ExpiresIn    int         `json:"expires_in"`
	User         interface{} `json:"user"`
}

type RefreshTokenResponse struct {
	Success     bool   `json:"success"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type ErrorResponse struct {
	Success bool        `json:"success"`
	Error   string      `json:"error"`
	Details interface{} `json:"details,omitempty"`
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

type UserInfo struct {
	ID               uuid.UUID `json:"id"`
	Email            string    `json:"email"`
	SubscriptionPlan string    `json:"subscription_plan"`
}

func NewSuccessResponse(message string, data interface{}) APIResponse {
	return APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	}
}

func NewErrorResponse(error string, details interface{}) ErrorResponse {
	return ErrorResponse{
		Success: false,
		Error:   error,
		Details: details,
	}
}

func NewAuthResponse(accessToken, refreshToken string, expiresIn int, user interface{}, message string) AuthResponse {
	return AuthResponse{
		Success:      true,
		Message:      message,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    expiresIn,
		User:         user,
	}
}

func NewRefreshTokenResponse(accessToken string, expiresIn int) RefreshTokenResponse {
	return RefreshTokenResponse{
		Success:     true,
		AccessToken: accessToken,
		ExpiresIn:   expiresIn,
	}
}
