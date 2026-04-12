package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/dinarasaurae/inbetwin-auth-service/internal/models"
	"github.com/dinarasaurae/inbetwin-auth-service/internal/services"
	jwtlib "github.com/dinarasaurae/inbetwin-shared/jwt-go"
)



type NotificationHandler struct {
	svc *services.NotificationService
}

func NewNotificationHandler(svc *services.NotificationService) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

// POST /api/v1/notifications/device-token
// Registers a device FCM token for the authenticated user.
func (h *NotificationHandler) RegisterToken(c fiber.Ctx) error {
	userID, err := extractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("Unauthorized", nil))
	}

	var req models.RegisterDeviceTokenRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("Invalid request", err.Error()))
	}
	if req.Token == "" {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("token is required", nil))
	}

	if err := h.svc.RegisterToken(c.Context(), userID, req); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(jwtlib.NewErrorResponse("Failed to register token", err.Error()))
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"ok": true})
}

// DELETE /api/v1/notifications/device-token
// Removes a device token (call on logout).
func (h *NotificationHandler) UnregisterToken(c fiber.Ctx) error {
	userID, err := extractUserID(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("Unauthorized", nil))
	}

	var req models.RegisterDeviceTokenRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("Invalid request", err.Error()))
	}

	if err := h.svc.UnregisterToken(c.Context(), userID, req.Token); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(jwtlib.NewErrorResponse("Failed to unregister token", err.Error()))
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"ok": true})
}

// extractUserID pulls the user UUID from locals set by AuthMiddleware.
func extractUserID(c fiber.Ctx) (uuid.UUID, error) {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return uuid.Nil, fiber.ErrUnauthorized
	}
	return userID, nil
}
