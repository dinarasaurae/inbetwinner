package handlers

import (
	"github.com/gofiber/fiber/v3"

	"github.com/dinarasaurae/inbetwin-auth-service/internal/models"
	"github.com/dinarasaurae/inbetwin-auth-service/internal/services"
	jwtlib "github.com/dinarasaurae/inbetwin-shared/jwt-go"
)

type AuthHandler struct {
	authService *services.AuthService
	jwtService  *jwtlib.Service
}

func NewAuthHandler(authService *services.AuthService, jwtService *jwtlib.Service) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		jwtService:  jwtService,
	}
}

func (h *AuthHandler) Register(c fiber.Ctx) error {
	var req models.CreateUserRequest

	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse(
			"Invalid request format", err.Error()))
	}

	if req.Email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse(
			"Email is required", nil))
	}
	if req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse(
			"Password is required", nil))
	}
	if len(req.Password) < 6 {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse(
			"Password must be at least 6 characters", nil))
	}

	user, err := h.authService.Register(&req)
	if err != nil {
		return c.Status(fiber.StatusConflict).JSON(jwtlib.NewErrorResponse(
			err.Error(), nil))
	}

	return c.Status(fiber.StatusCreated).JSON(jwtlib.NewSuccessResponse(
		"User registered successfully", user.ToResponse()))
}

func (h *AuthHandler) Login(c fiber.Ctx) error {
	var req models.LoginRequest

	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse(
			"Invalid request format", err.Error()))
	}

	if req.Email == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse(
			"Email and password are required", nil))
	}

	user, accessToken, refreshToken, err := h.authService.Login(&req)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse(
			err.Error(), nil))
	}

	return c.JSON(jwtlib.NewAuthResponse(
		accessToken, refreshToken, h.jwtService.GetAccessTokenExpiration(),
		user.ToResponse(), "Login successful"))
}

func (h *AuthHandler) RefreshToken(c fiber.Ctx) error {
	var req models.RefreshTokenRequest

	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse(
			"Invalid request format", err.Error()))
	}

	if req.RefreshToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse(
			"Refresh token is required", nil))
	}

	accessToken, err := h.authService.RefreshAccessToken(req.RefreshToken)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse(
			err.Error(), nil))
	}

	return c.JSON(jwtlib.NewRefreshTokenResponse(
		accessToken, h.jwtService.GetAccessTokenExpiration()))
}

func (h *AuthHandler) GetProfile(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse(
			"User not authenticated", nil))
	}

	user, err := h.authService.GetUserByID(userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(jwtlib.NewErrorResponse(
			"User not found", nil))
	}

	return c.JSON(jwtlib.NewSuccessResponse(
		"Profile retrieved successfully", user.ToResponse()))
}

func (h *AuthHandler) ChangePassword(c fiber.Ctx) error {
	var req models.ChangePasswordRequest

	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse(
			"Invalid request format", err.Error()))
	}

	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse(
			"User not authenticated", nil))
	}

	if err := h.authService.ChangePassword(userID, &req); err != nil {
		msg := err.Error()
		if msg == "current password is incorrect" {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(jwtlib.NewErrorResponse(msg, nil))
		}
		if msg == "current_password and new_password are required" || msg == "new password must be at least 6 characters" {
			return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse(msg, nil))
		}
		return c.Status(fiber.StatusInternalServerError).JSON(jwtlib.NewErrorResponse(
			"failed to change password", err.Error()))
	}

	return c.JSON(jwtlib.NewSuccessResponse("Password changed successfully", nil))
}

func (h *AuthHandler) UpdateProfile(c fiber.Ctx) error {
	var req models.UpdateUserRequest

	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse(
			"Invalid request format", err.Error()))
	}

	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse(
			"User not authenticated", nil))
	}

	updatedUser, err := h.authService.UpdateUserProfile(userID, &req)
	if err != nil {
		if err.Error() == "at least one field is required" {
			return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse(
				err.Error(), nil))
		}
		if err.Error() == "user not found" {
			return c.Status(fiber.StatusNotFound).JSON(jwtlib.NewErrorResponse(
				err.Error(), nil))
		}
		return c.Status(fiber.StatusInternalServerError).JSON(jwtlib.NewErrorResponse(
			"failed to update profile", err.Error()))
	}

	return c.JSON(jwtlib.NewSuccessResponse(
		"Profile updated successfully", updatedUser.ToResponse()))
}
