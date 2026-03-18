package handlers

import (
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/dinarasaurae/inbetwin-social-service/internal/database"
)

type HealthHandler struct {
	db *database.DB
}

func NewHealthHandler(db *database.DB) *HealthHandler {
	return &HealthHandler{db: db}
}

func (h *HealthHandler) Check(c fiber.Ctx) error {
	if err := h.db.HealthCheck(); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"status": "error",
			"error":  "database connection failed",
		})
	}
	return c.JSON(fiber.Map{
		"status":  "ok",
		"service": "social-service",
		"version": "1.0.0",
		"time":    time.Now().Format(time.RFC3339),
	})
}
