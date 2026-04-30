package handlers

import (
	"log"
	"strings"

	"github.com/dinarasaurae/inbetwin-llm-service/internal/services/zoho"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type ZohoHandler struct{ svc *zoho.Service }

func NewZohoHandler(svc *zoho.Service) *ZohoHandler {
	return &ZohoHandler{svc: svc}
}

// GetAuthURL returns the Zoho OAuth consent URL for the current workspace.
func (h *ZohoHandler) GetAuthURL(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	return c.JSON(fiber.Map{"auth_url": h.svc.GetAuthURL(wid)})
}

// Callback is a public route where Zoho redirects after OAuth consent.
// Query params: code, state (workspaceID), accounts-server (optional datacenter URL).
func (h *ZohoHandler) Callback(c fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")
	accountsServer := c.Query("accounts-server")

	log.Printf("[zoho] callback received: state=%s accounts-server=%s code_len=%d",
		state, accountsServer, len(code))

	if code == "" {
		return c.Status(400).JSON(fiber.Map{"error": "missing code"})
	}

	integration, err := h.svc.HandleCallback(c.Context(), code, state, accountsServer)
	if err != nil {
		errMsg := err.Error()
		log.Printf("[zoho] callback error: %s", errMsg)

		// Stale / already-used code → show user-friendly page instead of raw 500
		if strings.Contains(errMsg, "invalid_code") || strings.Contains(errMsg, "invalid_grant") {
			return c.Status(400).Type("html").SendString(`<!DOCTYPE html><html><body>
<p>Код авторизации уже использован или истёк. Пожалуйста, вернитесь в приложение и нажмите «Подключить» снова.</p>
<script>window.opener&&window.opener.postMessage("zoho_error","*");setTimeout(()=>window.close(),3000);</script>
</body></html>`)
		}
		return c.Status(500).JSON(fiber.Map{"error": errMsg})
	}
	_ = integration

	// Try to close popup / notify parent; if opened in a regular tab — show a message.
	return c.Type("html").SendString(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Zoho CRM подключён</title>
<style>
  body{margin:0;display:flex;align-items:center;justify-content:center;min-height:100vh;
       font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#f5f5f5}
  .card{background:#fff;border-radius:16px;padding:40px 32px;max-width:360px;width:90%;
        text-align:center;box-shadow:0 4px 24px rgba(0,0,0,.08)}
  .icon{font-size:48px;margin-bottom:16px}
  h2{margin:0 0 8px;font-size:20px;color:#1a1a1a}
  p{margin:0 0 24px;font-size:15px;color:#666;line-height:1.5}
  .btn{display:inline-block;background:#E42527;color:#fff;text-decoration:none;
       padding:14px 28px;border-radius:12px;font-size:15px;font-weight:600}
</style>
</head>
<body>
<div class="card">
  <div class="icon">✅</div>
  <h2>Zoho CRM подключён!</h2>
  <p>Авторизация прошла успешно.<br>Вернитесь в приложение и нажмите<br><strong>«Я авторизовался»</strong></p>
  <a href="inbetwin://oauth/zoho/success" class="btn">Открыть приложение</a>
</div>
<script>
  // If opened as popup — notify parent and close
  if(window.opener){
    window.opener.postMessage("zoho_connected","*");
    window.close();
  }
  // Try deep link after short delay (fallback for tab-based flow)
  setTimeout(function(){
    try{ window.location.href="inbetwin://oauth/zoho/success"; }catch(e){}
  }, 800);
</script>
</body>
</html>`)
}

// GetStatus returns the Zoho CRM connection status for the current workspace.
func (h *ZohoHandler) GetStatus(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	log.Printf("[zoho] GetStatus: workspace_id=%s", wid)
	integration, err := h.svc.GetIntegration(c.Context(), wid)
	if err != nil {
		log.Printf("[zoho] GetStatus: no integration for workspace_id=%s err=%v", wid, err)
		return c.JSON(fiber.Map{"connected": false})
	}
	return c.JSON(fiber.Map{
		"connected":    true,
		"org_name":     integration.OrgName,
		"org_id":       integration.OrgID,
		"api_domain":   integration.APIDomain,
		"token_expiry": integration.TokenExpiry,
	})
}

// GetDealStages returns the Deal stage options (Zoho's "pipeline" equivalent).
func (h *ZohoHandler) GetDealStages(c fiber.Ctx) error {
	wid := c.Locals("workspaceID").(uuid.UUID)
	stages, err := h.svc.GetDealStages(c.Context(), wid)
	if err != nil {
		log.Printf("[zoho] GetDealStages error workspace=%s: %v", wid, err)
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(stages)
}
