package handlers

import (
	"github.com/dinarasaurae/inbetwin-rag-service/internal/models"
	"github.com/dinarasaurae/inbetwin-rag-service/internal/services"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type QAHandler struct{ svc *services.QAService }

func NewQAHandler(svc *services.QAService) *QAHandler { return &QAHandler{svc: svc} }

func (h *QAHandler) Create(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	var req models.CreateQARequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	qa, err := h.svc.Create(c.Context(), wid, req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(qa)
}

func (h *QAHandler) List(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	nsID, err := uuid.Parse(c.Query("namespace_id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid namespace_id"})
	}
	list, err := h.svc.List(c.Context(), wid, nsID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if list == nil {
		list = []models.QAPair{}
	}
	return c.JSON(list)
}

func (h *QAHandler) Delete(c fiber.Ctx) error {
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
