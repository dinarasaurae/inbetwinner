package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/dinarasaurae/inbetwin-llm-service/internal/services"
)

// SocialHandler handles structured orchestration requests from social-service.
type SocialHandler struct {
	llm *services.LLMService
}

func NewSocialHandler(llm *services.LLMService) *SocialHandler {
	return &SocialHandler{llm: llm}
}

// ProcessVK handles POST /llm/social/vk/process.
// social-service sends inbound VK message + integration policy context.
// Response is a VKOrchestrationDecision (mode, draft_text, confidence, etc.).
func (h *SocialHandler) ProcessVK(c fiber.Ctx) error {
	workspaceID := c.Locals("workspaceID").(uuid.UUID)

	var req services.SocialMessageRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	req.WorkspaceID = workspaceID

	if req.Message == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "message is required"})
	}
	if req.Platform == "" {
		req.Platform = "vk"
	}
	if req.ChatUserID == "" {
		req.ChatUserID = "unknown"
	}

	decision, err := h.llm.ProcessSocialMessage(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(decision)
}
