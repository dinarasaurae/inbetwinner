package handlers

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
	jwtlib "github.com/dinarasaurae/inbetwin-shared/jwt-go"

	"github.com/dinarasaurae/inbetwin-social-service/internal/models"
	"github.com/dinarasaurae/inbetwin-social-service/internal/services"
)

// VKHandler handles VK integration HTTP routes.
type VKHandler struct {
	svc *services.VKService
}

// NewVKHandler creates a new VKHandler.
func NewVKHandler(svc *services.VKService) *VKHandler {
	return &VKHandler{svc: svc}
}

// OAuthStart handles GET /social/vk/oauth/start?group_id={id}
// Returns the VK authorization URL for the mobile app to open.
func (h *VKHandler) OAuthStart(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	groupIDStr := c.Query("group_id")
	groupID, err := strconv.ParseInt(groupIDStr, 10, 64)
	if err != nil || groupID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_group_id", "group_id must be a positive integer"))
	}

	authURL, state, err := h.svc.OAuthStart(c.Context(), userID, groupID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			jwtlib.NewErrorResponse("oauth_start_failed", err.Error()))
	}

	return c.JSON(jwtlib.NewSuccessResponse("", models.OAuthStartResponse{
		AuthURL: authURL,
		State:   state,
	}))
}

// OAuthExchange handles POST /social/vk/oauth/exchange
// Receives the code + state from the mobile app after the user authorizes.
func (h *VKHandler) OAuthExchange(c fiber.Ctx) error {
	_, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	var req models.ConnectVKRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_request", err.Error()))
	}
	if req.Code == "" || req.State == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_request", "code and state are required"))
	}

	integ, err := h.svc.OAuthExchange(c.Context(), req.Code, req.State)
	if err != nil {
		return mapVKError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(jwtlib.NewSuccessResponse("vk_connected", integ))
}

// ListIntegrations handles GET /social/vk/groups
// Returns all connected VK groups for the authenticated user.
func (h *VKHandler) ListIntegrations(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	list, err := h.svc.ListIntegrations(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			jwtlib.NewErrorResponse("query_failed", err.Error()))
	}
	if list == nil {
		list = []*models.VKIntegration{}
	}

	return c.JSON(jwtlib.NewSuccessResponse("", fiber.Map{
		"count":  len(list),
		"groups": list,
	}))
}

// Disconnect handles DELETE /social/vk/disconnect
// Deactivates a VK group integration.
func (h *VKHandler) Disconnect(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	var req struct {
		IntegrationID string `json:"integration_id"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_request", err.Error()))
	}

	integID, err := parseUUID(req.IntegrationID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_integration_id", nil))
	}

	if err := h.svc.Disconnect(c.Context(), userID, integID); err != nil {
		return mapVKError(c, err)
	}

	return c.JSON(jwtlib.NewSuccessResponse("vk_disconnected", nil))
}

// SyncPosts handles POST /social/vk/sync
// Imports recent wall posts from the connected VK group.
func (h *VKHandler) SyncPosts(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	var req models.SyncVKRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_request", err.Error()))
	}

	integID, err := parseUUID(req.IntegrationID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_integration_id", nil))
	}

	imported, err := h.svc.SyncPosts(c.Context(), userID, integID, req.Count)
	if err != nil {
		return mapVKError(c, err)
	}

	return c.JSON(jwtlib.NewSuccessResponse("", fiber.Map{"imported": imported}))
}

// SendMessage handles POST /social/vk/message
// Sends a DM to a lead from the connected VK group.
func (h *VKHandler) SendMessage(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	var req models.VKSendMessageRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_request", err.Error()))
	}
	if req.Text == "" || req.ToVKUserID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_request", "to_vk_user_id and text are required"))
	}

	integID, err := parseUUID(req.IntegrationID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_integration_id", nil))
	}

	if err := h.svc.SendMessage(c.Context(), userID, integID, req.ToVKUserID, req.Text); err != nil {
		return mapVKError(c, err)
	}

	return c.JSON(jwtlib.NewSuccessResponse("message_sent", nil))
}

// EnrichLead handles GET /social/vk/lead/:vk_user_id
// Fetches and/or refreshes the digital twin for a VK lead.
func (h *VKHandler) EnrichLead(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	vkUserID, err := strconv.ParseInt(c.Params("vk_user_id"), 10, 64)
	if err != nil || vkUserID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_vk_user_id", nil))
	}

	integIDStr := c.Query("integration_id")
	integID, err := parseUUID(integIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_integration_id", nil))
	}

	profile, err := h.svc.EnrichLead(c.Context(), userID, integID, vkUserID)
	if err != nil {
		return mapVKError(c, err)
	}

	return c.JSON(jwtlib.NewSuccessResponse("", profile))
}

// ─── Error mapping ────────────────────────────────────────────────────────────

func mapVKError(c fiber.Ctx, err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not_found"):
		return c.Status(fiber.StatusNotFound).JSON(jwtlib.NewErrorResponse("not_found", msg))
	case strings.Contains(msg, "unauthorized") || strings.Contains(msg, "invalid_state"):
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", msg))
	case strings.Contains(msg, "invalid_request") || strings.Contains(msg, "invalid_group_id"):
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("invalid_request", msg))
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(jwtlib.NewErrorResponse("internal_error", msg))
	}
}
