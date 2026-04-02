package handlers

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/dinarasaurae/inbetwin-lead-scoring-service/internal/models"
	"github.com/dinarasaurae/inbetwin-lead-scoring-service/internal/services"
)

type ScoringHandler struct {
	scorer *services.ScorerService
}

func NewScoringHandler(scorer *services.ScorerService) *ScoringHandler {
	return &ScoringHandler{scorer: scorer}
}

// POST /scoring/score
func (h *ScoringHandler) Score(c fiber.Ctx) error {
	workspaceID := c.Locals("workspaceID").(uuid.UUID)

	var req models.ScoreRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	req.WorkspaceID = workspaceID

	if req.LeadID == "" || req.Message == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "lead_id and message are required"})
	}
	if req.Platform == "" {
		req.Platform = "unknown"
	}

	resp, err := h.scorer.Score(c.Context(), req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(resp)
}

// GET /scoring/leads/:leadID
func (h *ScoringHandler) GetScore(c fiber.Ctx) error {
	workspaceID := c.Locals("workspaceID").(uuid.UUID)
	leadID := c.Params("leadID")

	ls, err := h.scorer.GetScore(c.Context(), workspaceID, leadID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if ls == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "lead not found"})
	}
	return c.JSON(ls)
}

// GET /scoring/leads
func (h *ScoringHandler) ListScores(c fiber.Ctx) error {
	workspaceID := c.Locals("workspaceID").(uuid.UUID)
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	if limit > 100 {
		limit = 100
	}

	scores, err := h.scorer.ListScores(c.Context(), workspaceID, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if scores == nil {
		scores = []models.LeadScore{}
	}
	return c.JSON(scores)
}

// GET /scoring/leads/:leadID/signals
func (h *ScoringHandler) GetSignals(c fiber.Ctx) error {
	workspaceID := c.Locals("workspaceID").(uuid.UUID)
	leadID := c.Params("leadID")

	signals, err := h.scorer.GetSignals(c.Context(), workspaceID, leadID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if signals == nil {
		signals = []models.BehavioralSignal{}
	}
	return c.JSON(signals)
}
