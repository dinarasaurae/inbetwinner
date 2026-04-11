package handlers

import (
	"github.com/dinarasaurae/inbetwin-rag-service/internal/models"
	"github.com/dinarasaurae/inbetwin-rag-service/internal/services"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type WebHandler struct{ svc *services.WebService }

func NewWebHandler(svc *services.WebService) *WebHandler { return &WebHandler{svc: svc} }

// Add handles POST /rag/web
func (h *WebHandler) Add(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	var req models.WebSourceCreateRequest
	if err := c.Bind().JSON(&req); err != nil || req.URL == "" {
		return c.Status(400).JSON(fiber.Map{"error": "url and namespace_id are required"})
	}
	src, err := h.svc.Add(c.Context(), wid, req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(src)
}

// List handles GET /rag/web
func (h *WebHandler) List(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	sources, err := h.svc.List(c.Context(), wid)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if sources == nil {
		sources = []models.WebSource{}
	}
	return c.JSON(fiber.Map{"items": sources, "total": len(sources)})
}

// Delete handles DELETE /rag/web/:id
func (h *WebHandler) Delete(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.svc.Delete(c.Context(), id, wid); err != nil {
		if err.Error() == "not_found: web source not found" {
			return c.Status(404).JSON(fiber.Map{"error": "not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}
