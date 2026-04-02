package handlers

import (
	"github.com/dinarasaurae/inbetwin-rag-service/internal/models"
	"github.com/dinarasaurae/inbetwin-rag-service/internal/services"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type NamespaceHandler struct{ svc *services.NamespaceService }

func NewNamespaceHandler(svc *services.NamespaceService) *NamespaceHandler {
	return &NamespaceHandler{svc: svc}
}

func (h *NamespaceHandler) Create(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	var req models.CreateNamespaceRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	ns, err := h.svc.Create(c.Context(), wid, req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(ns)
}

func (h *NamespaceHandler) List(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	list, err := h.svc.List(c.Context(), wid)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if list == nil {
		list = []models.Namespace{}
	}
	return c.JSON(list)
}

func (h *NamespaceHandler) Delete(c fiber.Ctx) error {
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
