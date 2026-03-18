package handlers

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v3"
	jwtlib "github.com/dinarasaurae/inbetwin-shared/jwt-go"

	"github.com/dinarasaurae/inbetwin-social-service/internal/models"
	"github.com/dinarasaurae/inbetwin-social-service/internal/services"
)

// TelegramHandler handles all user-facing Telegram integration endpoints.
type TelegramHandler struct {
	svc *services.TelegramService
}

func NewTelegramHandler(svc *services.TelegramService) *TelegramHandler {
	return &TelegramHandler{svc: svc}
}

// Connect handles POST /api/v1/social/telegram/connect
func (h *TelegramHandler) Connect(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(
			jwtlib.NewErrorResponse("unauthenticated", nil))
	}

	var req models.ConnectTelegramRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_request_body", err.Error()))
	}

	if req.ID == 0 || req.Hash == "" || req.AuthDate == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("missing_required_fields", "id, hash, auth_date, channel_username are required"))
	}
	if req.ChannelUsername == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("missing_channel_username", nil))
	}
	if !req.ConsentAcknowledged {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("consent_not_acknowledged",
				"You must acknowledge consent before connecting a channel"))
	}

	integration, err := h.svc.ConnectChannel(
		c.Context(), userID, req, c.IP(), c.Get("User-Agent"),
	)
	if err != nil {
		return mapServiceError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(
		jwtlib.NewSuccessResponse("Telegram channel connected", integration.ToResponse()))
}

// Disconnect handles DELETE /api/v1/social/telegram/disconnect
func (h *TelegramHandler) Disconnect(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(
			jwtlib.NewErrorResponse("unauthenticated", nil))
	}

	if err := h.svc.DisconnectChannel(c.Context(), userID); err != nil {
		return mapServiceError(c, err)
	}

	return c.JSON(jwtlib.NewSuccessResponse(
		"Integration disconnected and data deleted", nil))
}

// Status handles GET /api/v1/social/telegram/status
func (h *TelegramHandler) Status(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(
			jwtlib.NewErrorResponse("unauthenticated", nil))
	}

	status, err := h.svc.GetStatus(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			jwtlib.NewErrorResponse("internal_error", nil))
	}

	return c.JSON(jwtlib.NewSuccessResponse("", status))
}

// Posts handles GET /api/v1/social/telegram/posts
// Requires RequireConsent middleware to run first.
func (h *TelegramHandler) Posts(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(
			jwtlib.NewErrorResponse("unauthenticated", nil))
	}

	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	posts, total, err := h.svc.GetPosts(c.Context(), userID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			jwtlib.NewErrorResponse("internal_error", nil))
	}

	return c.JSON(jwtlib.NewSuccessResponse("", models.PostsResponse{
		Items:  posts,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}))
}

// mapServiceError converts service-layer errors to HTTP responses.
func mapServiceError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, services.ErrInvalidTelegramHash):
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_telegram_hash",
				"Telegram auth data signature is invalid"))
	case errors.Is(err, services.ErrAuthDataExpired):
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("auth_data_expired",
				"Telegram auth data is older than 5 minutes. Please re-authenticate."))
	case errors.Is(err, services.ErrConsentRequired):
		return c.Status(fiber.StatusForbidden).JSON(
			jwtlib.NewErrorResponse("consent_required",
				"You must acknowledge the data consent to connect a channel"))
	case errors.Is(err, services.ErrBotNotAdmin):
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("bot_not_channel_admin",
				"The inBeTwin bot must be added as administrator of the channel before connecting"))
	case errors.Is(err, services.ErrChannelNotFound):
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("channel_not_found",
				"Channel not found or is not accessible by the bot"))
	case errors.Is(err, services.ErrAlreadyConnected):
		return c.Status(fiber.StatusConflict).JSON(
			jwtlib.NewErrorResponse("already_connected",
				"This channel is already connected to your account"))
	case errors.Is(err, services.ErrIntegrationNotFound):
		return c.Status(fiber.StatusNotFound).JSON(
			jwtlib.NewErrorResponse("integration_not_found",
				"No active Telegram integration found"))
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(
			jwtlib.NewErrorResponse("internal_error", nil))
	}
}
