package handlers

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

type HealthHandler struct {
	redisClient *redis.Client
}

func NewHealthHandler(redisClient *redis.Client) *HealthHandler {
	return &HealthHandler{
		redisClient: redisClient,
	}
}

func (h *HealthHandler) Check(c fiber.Ctx) error {
	health := struct {
		Status  string `json:"status"`
		Redis   string `json:"redis"`
		Service string `json:"service"`
	}{
		Status:  "ok",
		Redis:   "connected",
		Service: "api-gateway",
	}

	ctx := context.Background()
	if err := h.redisClient.Ping(ctx).Err(); err != nil {
		health.Status = "degraded"
		health.Redis = "disconnected"
		return c.Status(fiber.StatusServiceUnavailable).JSON(health)
	}

	return c.JSON(health)
}
