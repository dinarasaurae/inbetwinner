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

// GetAuthURL returns the amoCRM OAuth consent URL for the current workspace.
func (h *AmoCRMHandler) GetAuthURL(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	return c.JSON(fiber.Map{"auth_url": h.svc.GetAuthURL(wid)})
}

// Callback is a public route where amoCRM redirects after OAuth consent.
// Query params: code, state (workspaceID), referer (account host).
func (h *AmoCRMHandler) Callback(c fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")
	referer := c.Query("referer")
	if referer == "" {
		referer = c.Query("referrer")
	}
	if code == "" {
		return c.Status(400).JSON(fiber.Map{"error": "missing code"})
	}
	integration, err := h.svc.HandleCallback(c.Context(), code, state, referer)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	// Try to close popup / notify parent; if opened in a regular tab — show a message.
	_ = integration
	return c.Type("html").SendString(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Kommo подключён</title>
<style>
  body{margin:0;display:flex;align-items:center;justify-content:center;min-height:100vh;
       font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#f5f5f5}
  .card{background:#fff;border-radius:16px;padding:40px 32px;max-width:360px;width:90%;
        text-align:center;box-shadow:0 4px 24px rgba(0,0,0,.08)}
  .icon{font-size:48px;margin-bottom:16px}
  h2{margin:0 0 8px;font-size:20px;color:#1a1a1a}
  p{margin:0 0 24px;font-size:15px;color:#666;line-height:1.5}
  .btn{display:inline-block;background:#1966ff;color:#fff;text-decoration:none;
       padding:14px 28px;border-radius:12px;font-size:15px;font-weight:600}
</style>
</head>
<body>
<div class="card">
  <div class="icon">✅</div>
  <h2>Kommo (AmoCRM) подключён!</h2>
  <p>Авторизация прошла успешно.<br>Вернитесь в приложение и нажмите<br><strong>«Я авторизовался»</strong></p>
  <a href="inbetwin://oauth/amocrm/success" class="btn">Открыть приложение</a>
</div>
<script>
  if(window.opener){
    window.opener.postMessage("amocrm_connected","*");
    window.close();
  }
  setTimeout(function(){
    try{ window.location.href="inbetwin://oauth/amocrm/success"; }catch(e){}
  }, 800);
</script>
</body>
</html>`)
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
		"connected":    true,
		"subdomain":    integration.Subdomain,
		"account_host": integration.Subdomain,
		"email":        integration.Email,
		"account_id":   integration.AccountID,
		"token_expiry": integration.TokenExpiry,
	})
}
