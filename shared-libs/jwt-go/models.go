package jwt

import (
	"github.com/google/uuid"
)

type APIResponse struct {
	Status  string      `json:"status"`          // изменено для интеграционных тестов
	Success bool        `json:"success"`         // оставлено для обратной совместимости
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type AuthResponse struct {
	Status       string      `json:"status"`          // добавлено для интеграционных тестов
	Success      bool        `json:"success"`         // оставлено для обратной совместимости
	Message      string      `json:"message,omitempty"`
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	ExpiresIn    int         `json:"expires_in"`
	User         interface{} `json:"user"`
}

type RefreshTokenResponse struct {
	Status      string `json:"status"`      // добавлено для интеграционных тестов
	Success     bool   `json:"success"`     // оставлено для обратной совместимости
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type ErrorResponse struct {
	Status  string      `json:"status"`          // изменено для интеграционных тестов
	Success bool        `json:"success"`         // оставлено для обратной совместимости
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
		Status:  "success",        // добавлено для интеграционных тестов
		Success: true,             // оставлено для обратной совместимости
		Message: message,
		Data:    data,
	}
}

func NewErrorResponse(error string, details interface{}) ErrorResponse {
	return ErrorResponse{
		Status:  "error",          // добавлено для интеграционных тестов
		Success: false,            // оставлено для обратной совместимости
		Error:   error,
		Details: details,
	}
}

func NewAuthResponse(accessToken, refreshToken string, expiresIn int, user interface{}, message string) AuthResponse {
	return AuthResponse{
		Status:       "success",   // добавлено для интеграционных тестов
		Success:      true,        // оставлено для обратной совместимости
		Message:      message,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    expiresIn,
		User:         user,
	}
}

func NewRefreshTokenResponse(accessToken string, expiresIn int) RefreshTokenResponse {
	return RefreshTokenResponse{
		Status:      "success",    // добавлено для интеграционных тестов
		Success:     true,         // оставлено для обратной совместимости
		AccessToken: accessToken,
		ExpiresIn:   expiresIn,
	}
}
