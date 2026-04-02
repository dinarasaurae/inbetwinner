package handlers

import (
	"github.com/dinarasaurae/inbetwin-rag-service/internal/models"
	"github.com/dinarasaurae/inbetwin-rag-service/internal/services"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type DocumentHandler struct{ svc *services.DocumentService }

func NewDocumentHandler(svc *services.DocumentService) *DocumentHandler {
	return &DocumentHandler{svc: svc}
}

func (h *DocumentHandler) Create(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	var req models.CreateDocumentRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	doc, err := h.svc.Create(c.Context(), wid, req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(doc)
}

func (h *DocumentHandler) List(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	nsIDStr := c.Query("namespace_id")
	nsID, err := uuid.Parse(nsIDStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid namespace_id"})
	}
	docs, err := h.svc.List(c.Context(), wid, nsID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if docs == nil {
		docs = []models.KnowledgeDocument{}
	}
	return c.JSON(docs)
}

func (h *DocumentHandler) Delete(c fiber.Ctx) error {
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
