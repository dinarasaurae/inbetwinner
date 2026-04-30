package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestUserOAuthImportDisabledReturnsLegacyFallback404(t *testing.T) {
	app := fiber.New()
	handler := NewVKHandler(nil, "", false)
	app.Post("/social/vk/oauth/user/import", handler.UserOAuthImport)

	req := httptest.NewRequest(http.MethodPost, "/social/vk/oauth/user/import", strings.NewReader(`{"access_token":"vk2.a.test","platform":"android"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}
