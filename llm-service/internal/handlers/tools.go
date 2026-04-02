package handlers

import (
	"github.com/dinarasaurae/inbetwin-llm-service/internal/models"
	"github.com/dinarasaurae/inbetwin-llm-service/internal/services"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type ToolHandler struct{ svc *services.ToolService }

func NewToolHandler(svc *services.ToolService) *ToolHandler { return &ToolHandler{svc: svc} }

func (h *ToolHandler) List(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	list, err := h.svc.List(c.Context(), wid)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if list == nil {
		list = []models.Tool{}
	}
	return c.JSON(list)
}

func (h *ToolHandler) Create(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	var req models.CreateToolRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	t, err := h.svc.Create(c.Context(), wid, req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(t)
}

func (h *ToolHandler) Update(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var req models.CreateToolRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	t, err := h.svc.Update(c.Context(), id, wid, req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(t)
}

func (h *ToolHandler) Delete(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.svc.Delete(c.Context(), id, wid); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}

func (h *ToolHandler) GetExecutions(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	toolName := c.Params("name")
	execs, err := h.svc.GetExecutions(c.Context(), wid, toolName, 50)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(execs)
}
