package handlers

import (
	"github.com/dinarasaurae/inbetwin-llm-service/internal/services/amocrm"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type AmoCRMHandler struct{ svc *amocrm.Service }

func NewAmoCRMHandler(svc *amocrm.Service) *AmoCRMHandler {
	return &AmoCRMHandler{svc: svc}
}

// GetAuthURL returns the Kommo OAuth consent URL for the current workspace.
func (h *AmoCRMHandler) GetAuthURL(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	return c.JSON(fiber.Map{"auth_url": h.svc.GetAuthURL(wid)})
}

// Callback — public route. Kommo redirects here after OAuth consent.
// Query params: code, state (workspaceID), referer (subdomain, e.g. "mycompany")
func (h *AmoCRMHandler) Callback(c fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")
	// Kommo appends the subdomain as "referer" — e.g. "mycompany" (without .kommo.com)
	subdomain := c.Query("referer")
	if code == "" {
		return c.Status(400).JSON(fiber.Map{"error": "missing code"})
	}
	integration, err := h.svc.HandleCallback(c.Context(), code, state, subdomain)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	// Close the OAuth popup window and signal success to the parent
	_ = integration
	return c.Type("html").SendString(`<script>window.opener&&window.opener.postMessage("amocrm_connected","*");window.close();</script>`)
}

// GetPipelines returns all pipelines and their stages for the current workspace.
func (h *AmoCRMHandler) GetPipelines(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	pipelines, err := h.svc.GetPipelines(c.Context(), wid)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(pipelines)
}

// GetStatus returns connection status for the current workspace.
func (h *AmoCRMHandler) GetStatus(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	integration, err := h.svc.GetIntegration(c.Context(), wid)
	if err != nil {
		return c.JSON(fiber.Map{"connected": false})
	}
	return c.JSON(fiber.Map{
		"connected":   true,
		"subdomain":   integration.Subdomain,
		"email":       integration.Email,
		"account_id":  integration.AccountID,
		"token_expiry": integration.TokenExpiry,
	})
}
