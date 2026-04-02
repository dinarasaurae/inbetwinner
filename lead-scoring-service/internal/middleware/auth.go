package middleware

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

func WorkspaceAuth(c fiber.Ctx) error {
	raw := c.Get("X-User-ID")
	if raw == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing X-User-ID header"})
	}
	uid, err := uuid.Parse(raw)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid X-User-ID"})
	}
	c.Locals("workspaceID", uid)
	return c.Next()
}
