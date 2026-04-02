package handlers

import (
	"github.com/dinarasaurae/inbetwin-agent-service/internal/models"
	"github.com/dinarasaurae/inbetwin-agent-service/internal/services"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type AgentHandler struct{ svc *services.AgentService }

func NewAgentHandler(svc *services.AgentService) *AgentHandler {
	return &AgentHandler{svc: svc}
}

func (h *AgentHandler) List(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	agents, err := h.svc.List(c.Context(), wid)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if agents == nil {
		agents = []models.Agent{}
	}
	return c.JSON(agents)
}

func (h *AgentHandler) Create(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	var req models.CreateAgentRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if req.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name required"})
	}
	agent, err := h.svc.Create(c.Context(), wid, req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(agent)
}

func (h *AgentHandler) Get(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	agent, err := h.svc.GetByID(c.Context(), id, wid)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "agent not found"})
	}
	return c.JSON(agent)
}

func (h *AgentHandler) Update(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var req models.CreateAgentRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	agent, err := h.svc.Update(c.Context(), id, wid, req)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(agent)
}

func (h *AgentHandler) Delete(c fiber.Ctx) error {
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

func (h *AgentHandler) AssignTool(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var body struct {
		ToolID   string `json:"tool_id"`
		Priority int    `json:"priority"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	if err := h.svc.AssignTool(c.Context(), id, body.ToolID, body.Priority); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{"message": "tool assigned"})
}

func (h *AgentHandler) UnassignTool(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.svc.UnassignTool(c.Context(), id, c.Params("toolId")); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "tool unassigned"})
}

func (h *AgentHandler) GetTools(c fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	ts, err := h.svc.GetTools(c.Context(), id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if ts == nil {
		ts = []models.AgentTool{}
	}
	return c.JSON(ts)
}

func ListBuiltinTools(c fiber.Ctx) error {
	return c.JSON(models.BuiltinTools)
}
