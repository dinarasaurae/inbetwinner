package handlers

import (
	"log"

	"github.com/dinarasaurae/inbetwin-rag-service/internal/services"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type GoogleOAuthHandler struct {
	svc         *services.GoogleOAuthService
	redirectURI string
}

func NewGoogleOAuthHandler(svc *services.GoogleOAuthService, redirectURI string) *GoogleOAuthHandler {
	return &GoogleOAuthHandler{svc: svc, redirectURI: redirectURI}
}

// Start handles GET /rag/google/oauth/start
// Returns the Google authorization URL for the workspace to open in browser.
func (h *GoogleOAuthHandler) Start(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)

	authURL, state, err := h.svc.StartOAuth(c.Context(), wid)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"auth_url": authURL,
		"state":    state,
	})
}

// Callback handles GET /rag/google/oauth/callback
// VK-style: receives code+state from Google, exchanges for tokens.
// This endpoint is called by the browser redirect — it is NOT behind workspace auth.
func (h *GoogleOAuthHandler) Callback(c fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")
	errParam := c.Query("error")

	if errParam != "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": errParam})
	}
	if code == "" || state == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing code or state"})
	}

	log.Printf("[google] callback: state=%s code_len=%d", state, len(code))
	if err := h.svc.ExchangeCode(c.Context(), code, state); err != nil {
		log.Printf("[google] ExchangeCode error: %v", err)
		return c.Status(fiber.StatusBadRequest).SendString(`<!DOCTYPE html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Ошибка</title></head><body style="font-family:sans-serif;text-align:center;padding:40px">
<h2>⚠️ Ошибка подключения</h2><p>` + err.Error() + `</p>
<p>Вернитесь в приложение и попробуйте снова.</p></body></html>`)
	}
	log.Printf("[google] token saved for state=%s", state)

	// Show success page — browser can't open inbetwin:// deep-link directly
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.SendString(`<!DOCTYPE html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Google Sheets подключён</title>
<script>
  // Open the app via the registered inbetwin://oauth host (inbetwin://google-callback is NOT registered)
  window.location.href = "inbetwin://oauth/google/success";
  setTimeout(function(){ document.getElementById('msg').style.display='block'; }, 1500);
</script>
</head><body style="font-family:sans-serif;text-align:center;padding:40px;background:#f0fdf4">
<h2 style="color:#16a34a">✅ Google Sheets подключён!</h2>
<p id="msg" style="display:none;color:#555">Можете вернуться в приложение inBeTwin.</p>
</body></html>`)
}

// Status handles GET /rag/google/oauth/status
// Returns whether the workspace has connected Google.
func (h *GoogleOAuthHandler) Status(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	connected := h.svc.IsConnected(c.Context(), wid)
	return c.JSON(fiber.Map{"connected": connected})
}

// Disconnect handles DELETE /rag/google/oauth
func (h *GoogleOAuthHandler) Disconnect(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	if err := h.svc.Disconnect(c.Context(), wid); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}
