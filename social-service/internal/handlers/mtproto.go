package handlers

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	jwtlib "github.com/dinarasaurae/inbetwin-shared/jwt-go"

	"github.com/dinarasaurae/inbetwin-social-service/internal/mtproto"
)

// MtprotoHandler handles all userbot (MTProto) endpoints.
// Every operation is performed as the authenticated user themselves —
// not as a bot.
type MtprotoHandler struct {
	svc *mtproto.Service
}

func NewMtprotoHandler(svc *mtproto.Service) *MtprotoHandler {
	return &MtprotoHandler{svc: svc}
}

// SendCode handles POST /social/telegram/auth/send-code
// Triggers Telegram to send an OTP to the user's phone.
func (h *MtprotoHandler) SendCode(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthenticated", nil))
	}

	var req struct {
		Phone string `json:"phone"` // E.164: +79001234567
	}
	if err := c.Bind().JSON(&req); err != nil || req.Phone == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_request", "phone is required (E.164 format)"))
	}

	hash, timeout, err := h.svc.SendCode(c.Context(), userID, req.Phone)
	if err != nil {
		return mapMtprotoError(c, err)
	}

	return c.JSON(jwtlib.NewSuccessResponse("Code sent. Enter it within the timeout.", fiber.Map{
		"phone_code_hash": hash,
		"timeout_seconds": timeout,
	}))
}

// SignIn handles POST /social/telegram/auth/sign-in
// Completes authentication with the OTP code.
func (h *MtprotoHandler) SignIn(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthenticated", nil))
	}

	var req struct {
		Code string `json:"code"` // OTP received in Telegram
	}
	if err := c.Bind().JSON(&req); err != nil || req.Code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_request", "code is required"))
	}

	user, err := h.svc.SignIn(c.Context(), userID, req.Code)
	if err != nil {
		return mapMtprotoError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(jwtlib.NewSuccessResponse(
		"Telegram account connected. You can now connect a channel.", fiber.Map{
			"tg_user_id":   user.TgUserID,
			"tg_username":  user.TgUsername,
			"tg_first_name": user.TgFirstName,
			"tg_last_name":  user.TgLastName,
		}))
}

// SignOut handles DELETE /social/telegram/auth/sign-out
// Invalidates the session on Telegram's side and removes it from the DB.
func (h *MtprotoHandler) SignOut(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthenticated", nil))
	}

	if err := h.svc.SignOut(c.Context(), userID); err != nil {
		return mapMtprotoError(c, err)
	}

	return c.JSON(jwtlib.NewSuccessResponse("Telegram session revoked", nil))
}

// GetChannels handles GET /social/telegram/channels
// Returns channels where the authenticated user is admin or creator.
func (h *MtprotoHandler) GetChannels(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthenticated", nil))
	}

	channels, err := h.svc.GetAdminChannels(c.Context(), userID)
	if err != nil {
		return mapMtprotoError(c, err)
	}

	return c.JSON(jwtlib.NewSuccessResponse("", fiber.Map{
		"channels": channels,
		"count":    len(channels),
	}))
}

// Sync handles POST /social/telegram/sync
// Imports post history for a connected channel.
func (h *MtprotoHandler) Sync(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthenticated", nil))
	}

	var req struct {
		IntegrationID string `json:"integration_id"`
		ChannelID     int64  `json:"channel_id"`
		AccessHash    int64  `json:"access_hash"`
		OffsetMsgID   int    `json:"offset_msg_id"` // 0 = full import
		Limit         int    `json:"limit"`          // max 100
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("invalid_request", err.Error()))
	}

	intID, err := parseUUID(req.IntegrationID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("invalid_integration_id", nil))
	}

	posts, err := h.svc.ImportHistory(
		c.Context(), userID, intID,
		req.ChannelID, req.AccessHash,
		req.OffsetMsgID, req.Limit,
	)
	if err != nil {
		return mapMtprotoError(c, err)
	}

	return c.JSON(jwtlib.NewSuccessResponse("", fiber.Map{
		"imported": len(posts),
		"posts":    posts,
	}))
}

// SendMessage handles POST /social/telegram/message
// Sends a message to a channel AS the authenticated user (not as a bot).
func (h *MtprotoHandler) SendMessage(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthenticated", nil))
	}

	var req struct {
		ChannelID  int64  `json:"channel_id"`
		AccessHash int64  `json:"access_hash"`
		Text       string `json:"text"`
	}
	if err := c.Bind().JSON(&req); err != nil || req.Text == "" {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("invalid_request", "text is required"))
	}

	msgID, err := h.svc.SendMessage(c.Context(), userID, req.ChannelID, req.AccessHash, req.Text)
	if err != nil {
		return mapMtprotoError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(jwtlib.NewSuccessResponse("Message sent", fiber.Map{
		"message_id": msgID,
	}))
}

// ReplyToComment handles POST /social/telegram/reply
// Replies to a comment in a channel post discussion AS the authenticated user.
func (h *MtprotoHandler) ReplyToComment(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthenticated", nil))
	}

	var req struct {
		ChannelID    int64  `json:"channel_id"`
		AccessHash   int64  `json:"access_hash"`
		PostMsgID    int    `json:"post_message_id"`   // the channel post
		ReplyToMsgID int    `json:"reply_to_message_id"` // 0 = reply to the post itself
		Text         string `json:"text"`
	}
	if err := c.Bind().JSON(&req); err != nil || req.Text == "" {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("invalid_request", "text is required"))
	}

	msgID, err := h.svc.ReplyToComment(
		c.Context(), userID,
		req.ChannelID, req.AccessHash,
		req.PostMsgID, req.ReplyToMsgID,
		req.Text,
	)
	if err != nil {
		return mapMtprotoError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(jwtlib.NewSuccessResponse("Reply sent", fiber.Map{
		"message_id": msgID,
	}))
}

// mapMtprotoError converts MTProto service errors to HTTP responses.
func mapMtprotoError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, mtproto.ErrNoSession):
		return c.Status(fiber.StatusUnauthorized).JSON(
			jwtlib.NewErrorResponse("no_telegram_session",
				"Please authenticate via /auth/send-code and /auth/sign-in first"))
	case errors.Is(err, mtproto.ErrAuthPending):
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("auth_pending",
				"Call /auth/send-code before /auth/sign-in"))
	case errors.Is(err, mtproto.ErrAlreadyAuthed):
		return c.Status(fiber.StatusConflict).JSON(
			jwtlib.NewErrorResponse("already_authenticated",
				"Your Telegram account is already connected. Sign out first to re-authenticate."))
	case errors.Is(err, mtproto.ErrSignInFailed):
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("sign_in_failed", "Invalid code or code expired"))
	case errors.Is(err, mtproto.ErrNotChannelAdmin):
		return c.Status(fiber.StatusForbidden).JSON(
			jwtlib.NewErrorResponse("not_channel_admin",
				"You must be an administrator of the channel"))
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(
			jwtlib.NewErrorResponse("internal_error", err.Error()))
	}
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

func (h *MtprotoHandler) parseInt64Param(c fiber.Ctx, key string) (int64, error) {
	return strconv.ParseInt(c.Params(key), 10, 64)
}
