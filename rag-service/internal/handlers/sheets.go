package handlers

import (
	"errors"
	"log"

	"github.com/dinarasaurae/inbetwin-rag-service/internal/models"
	"github.com/dinarasaurae/inbetwin-rag-service/internal/services"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type SheetsHandler struct{ svc *services.SheetsService }

func NewSheetsHandler(svc *services.SheetsService) *SheetsHandler { return &SheetsHandler{svc: svc} }

// Sync handles POST /rag/sheets/sync
func (h *SheetsHandler) Sync(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	var req models.SheetsSyncRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if req.SpreadsheetID == "" || req.NamespaceID == uuid.Nil {
		return c.Status(400).JSON(fiber.Map{"error": "spreadsheet_id and namespace_id are required"})
	}
	table, err := h.svc.SyncSheet(c.Context(), wid, req)
	if err != nil {
		log.Printf("[sheets/sync] ERROR workspace=%s spreadsheet=%s: %v", wid, req.SpreadsheetID, err)
		return c.Status(statusForSheetsSyncError(err)).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(table)
}

func statusForSheetsSyncError(err error) int {
	var reqErr *services.RequestError
	if errors.As(err, &reqErr) {
		return reqErr.Status
	}
	return fiber.StatusInternalServerError
}

// List handles GET /rag/sheets
func (h *SheetsHandler) List(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	tables, err := h.svc.List(c.Context(), wid)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if tables == nil {
		tables = []models.KnowledgeTable{}
	}
	return c.JSON(fiber.Map{"items": tables, "total": len(tables)})
}

// Delete handles DELETE /rag/sheets/:id
func (h *SheetsHandler) Delete(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.svc.Delete(c.Context(), id, wid); err != nil {
		if err.Error() == "not_found: table not found" {
			return c.Status(404).JSON(fiber.Map{"error": "not found"})
		}
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}
