package middleware

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

func WorkspaceAuth() fiber.Handler {
	return func(c fiber.Ctx) error {
		userIDStr := c.Get("X-User-ID")
		if userIDStr == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing X-User-ID header"})
		}
		uid, err := uuid.Parse(userIDStr)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid X-User-ID"})
		}
		c.Locals("workspaceID", uid)
		return c.Next()
	}
}
