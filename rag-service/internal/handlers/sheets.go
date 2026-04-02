package handlers

import (
	"github.com/dinarasaurae/inbetwin-rag-service/internal/models"
	"github.com/dinarasaurae/inbetwin-rag-service/internal/services"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type SheetsHandler struct{ svc *services.SheetsService }

func NewSheetsHandler(svc *services.SheetsService) *SheetsHandler { return &SheetsHandler{svc: svc} }

func (h *SheetsHandler) Sync(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	var req models.SheetsSyncRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	table, err := h.svc.SyncSheet(c.Context(), wid, req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(table)
}
