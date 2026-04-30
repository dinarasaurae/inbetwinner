package handlers

import (
	"github.com/dinarasaurae/inbetwin-rag-service/internal/services"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type SearchHandler struct{ svc *services.SearchService }

func NewSearchHandler(svc *services.SearchService) *SearchHandler { return &SearchHandler{svc: svc} }

func (h *SearchHandler) Search(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	var req services.SearchRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	req.WorkspaceID = wid
	results, err := h.svc.HybridSearch(c.Context(), req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if results == nil {
		results = []services.SearchResult{}
	}
	return c.JSON(fiber.Map{
		"results": results,
		"count":   len(results),
		"total":   len(results),
	})
}
