package handlers

import (
	"strconv"

	jwtlib "github.com/dinarasaurae/inbetwin-shared/jwt-go"
	"github.com/gofiber/fiber/v3"

	"github.com/dinarasaurae/inbetwin-social-service/internal/models"
)

func (h *VKHandler) GetDiscoveryStatus(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}
	data, err := h.svc.GetDiscoveryStatus(c.Context(), userID)
	if err != nil {
		return mapVKError(c, err)
	}
	return c.JSON(jwtlib.NewSuccessResponse("", data))
}

func (h *VKHandler) UserLegacyOAuthStart(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}
	platform := normalisePlatform(c.Query("platform"))
	authURL, state, implicit, err := h.svc.UserLegacyOAuthStart(c.Context(), userID, platform)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(jwtlib.NewErrorResponse("oauth_start_failed", err.Error()))
	}
	return c.JSON(jwtlib.NewSuccessResponse("", models.VKUserOAuthStartResponse{
		AuthURL:      authURL,
		State:        state,
		Platform:     platform,
		ImplicitFlow: implicit,
	}))
}

func (h *VKHandler) GetUserDataSnapshot(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}
	data, err := h.svc.GetUserDataSnapshot(c.Context(), userID)
	if err != nil {
		return mapVKError(c, err)
	}
	return c.JSON(jwtlib.NewSuccessResponse("", data))
}

func (h *VKHandler) BootstrapContext(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}
	var req models.VKContextBootstrapRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("invalid_request", err.Error()))
	}
	integID, err := parseUUID(req.IntegrationID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("invalid_integration_id", nil))
	}
	data, err := h.svc.BootstrapContext(c.Context(), userID, integID, req.Count, req.IncludeSubscriptions)
	if err != nil {
		return mapVKError(c, err)
	}
	return c.JSON(jwtlib.NewSuccessResponse("", data))
}

func (h *VKHandler) GetWorkspace(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}
	integID, err := parseUUID(c.Query("integration_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("invalid_integration_id", nil))
	}
	data, err := h.svc.GetWorkspace(c.Context(), userID, integID)
	if err != nil {
		return mapVKError(c, err)
	}
	return c.JSON(jwtlib.NewSuccessResponse("", data))
}

func (h *VKHandler) GetAgentSettings(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}
	integID, err := parseUUID(c.Query("integration_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("invalid_integration_id", nil))
	}
	data, err := h.svc.GetAgentSettings(c.Context(), userID, integID)
	if err != nil {
		return mapVKError(c, err)
	}
	return c.JSON(jwtlib.NewSuccessResponse("", data))
}

func (h *VKHandler) UpdateAgentSettings(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}
	var req models.VKAgentSettingsUpdateRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("invalid_request", err.Error()))
	}
	data, err := h.svc.UpdateAgentSettings(c.Context(), userID, req)
	if err != nil {
		return mapVKError(c, err)
	}
	return c.JSON(jwtlib.NewSuccessResponse("", data))
}

func (h *VKHandler) ListDrafts(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}
	integID, err := parseUUID(c.Query("integration_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("invalid_integration_id", nil))
	}
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	data, err := h.svc.ListDrafts(c.Context(), userID, integID, limit)
	if err != nil {
		return mapVKError(c, err)
	}
	return c.JSON(jwtlib.NewSuccessResponse("", fiber.Map{
		"count":  len(data),
		"drafts": data,
	}))
}

func (h *VKHandler) GenerateDraft(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}
	var req models.VKGenerateDraftRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("invalid_request", err.Error()))
	}
	integID, err := parseUUID(req.IntegrationID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("invalid_integration_id", nil))
	}
	msgID, err := parseUUID(req.MessageID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("invalid_message_id", nil))
	}
	data, err := h.svc.GenerateDraft(c.Context(), userID, integID, msgID, req.Force)
	if err != nil {
		return mapVKError(c, err)
	}
	return c.JSON(jwtlib.NewSuccessResponse("", data))
}

func (h *VKHandler) ApproveDraft(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}
	draftID, err := parseUUID(c.Params("draft_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("invalid_draft_id", nil))
	}
	var req models.VKApproveDraftRequest
	_ = c.Bind().JSON(&req)
	data, err := h.svc.ApproveAndSendDraft(c.Context(), userID, draftID, req.TextOverride)
	if err != nil {
		return mapVKError(c, err)
	}
	return c.JSON(jwtlib.NewSuccessResponse("message_sent", data))
}
