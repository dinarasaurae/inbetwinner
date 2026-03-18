package handlers

import (
	"context"
	"crypto/hmac"
	"log"

	"github.com/gofiber/fiber/v3"

	"github.com/dinarasaurae/inbetwin-social-service/internal/models"
	"github.com/dinarasaurae/inbetwin-social-service/internal/services"
)

// WebhookHandler receives Bot API updates from Telegram.
// The route is PUBLIC (no JWT) but protected by a secret token.
type WebhookHandler struct {
	webhookSvc     *services.WebhookService
	expectedSecret string // X-Telegram-Bot-Api-Secret-Token value
}

func NewWebhookHandler(webhookSvc *services.WebhookService, secret string) *WebhookHandler {
	return &WebhookHandler{
		webhookSvc:     webhookSvc,
		expectedSecret: secret,
	}
}

// Receive handles POST /api/v1/social/telegram/webhook
func (h *WebhookHandler) Receive(c fiber.Ctx) error {
	// Always return 200 — Telegram retries on non-200 responses.
	// Reject invalid secret silently to avoid enumeration.
	incoming := c.Get("X-Telegram-Bot-Api-Secret-Token")
	if !hmac.Equal([]byte(incoming), []byte(h.expectedSecret)) {
		log.Printf("webhook: rejected request with invalid secret from %s", c.IP())
		return c.SendStatus(fiber.StatusOK) // silent reject
	}

	var update models.TelegramUpdate
	if err := c.Bind().JSON(&update); err != nil {
		log.Printf("webhook: failed to parse update body: %v", err)
		return c.SendStatus(fiber.StatusOK) // always 200
	}

	// Process asynchronously so Telegram doesn't wait for DB writes.
	go h.webhookSvc.ProcessUpdate(context.Background(), &update)

	return c.SendStatus(fiber.StatusOK)
}
