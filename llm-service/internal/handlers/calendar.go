package handlers

import (
	"github.com/dinarasaurae/inbetwin-llm-service/internal/services/calendar"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type CalendarHandler struct{ svc *calendar.Service }

func NewCalendarHandler(svc *calendar.Service) *CalendarHandler {
	return &CalendarHandler{svc: svc}
}

func (h *CalendarHandler) GetAuthURL(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	return c.JSON(fiber.Map{"auth_url": h.svc.GetAuthURL(wid)})
}

// Callback — public route, Google redirects here after OAuth consent
func (h *CalendarHandler) Callback(c fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")
	if code == "" {
		return c.Status(400).JSON(fiber.Map{"error": "missing code"})
	}
	integration, err := h.svc.HandleCallback(c.Context(), code, state)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(integration)
}

func (h *CalendarHandler) GetStatus(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	integration, err := h.svc.GetIntegration(c.Context(), wid)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"connected": false})
	}
	return c.JSON(fiber.Map{"connected": true, "integration": integration})
}

func (h *CalendarHandler) CreateEvent(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	var params calendar.CreateEventParams
	if err := c.Bind().JSON(&params); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid body"})
	}
	result, err := h.svc.CreateEvent(c.Context(), wid, params)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(result)
}

func (h *CalendarHandler) ListEvents(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	params := calendar.ListEventsParams{
		CalendarID: c.Query("calendar_id"),
		TimeMin:    c.Query("time_min"),
		TimeMax:    c.Query("time_max"),
	}
	events, err := h.svc.ListEvents(c.Context(), wid, params)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(events)
}
