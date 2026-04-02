package handlers

import (
	"github.com/dinarasaurae/inbetwin-llm-service/internal/services"
	histsvc "github.com/dinarasaurae/inbetwin-llm-service/internal/services/history"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type ChatHandler struct {
	llm  *services.LLMService
	hist *histsvc.Service
}

func NewChatHandler(llm *services.LLMService, hist *histsvc.Service) *ChatHandler {
	return &ChatHandler{llm: llm, hist: hist}
}

func (h *ChatHandler) Chat(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	var req services.ChatRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	req.WorkspaceID = wid
	if req.Platform == "" {
		req.Platform = "api"
	}
	if req.ChatUserID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "chat_user_id required"})
	}
	resp, err := h.llm.ProcessMessage(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(resp)
}

func (h *ChatHandler) GetHistory(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	chatUserID := c.Params("chat_user_id")
	msgs, err := h.hist.GetHistory(c.Context(), wid, chatUserID, 50)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(msgs)
}

func (h *ChatHandler) ClearHistory(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	chatUserID := c.Params("chat_user_id")
	if err := h.hist.ClearHistory(c.Context(), wid, chatUserID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "history cleared"})
}
