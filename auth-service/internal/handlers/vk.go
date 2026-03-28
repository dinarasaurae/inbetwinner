package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gofiber/fiber/v3"
	jwtlib "github.com/dinarasaurae/inbetwin-shared/jwt-go"
	"github.com/dinarasaurae/inbetwin-auth-service/internal/services"
)

type VKAuthHandler struct {
	svc        *services.AuthService
	jwtService *jwtlib.Service
}

func NewVKAuthHandler(svc *services.AuthService, jwtService *jwtlib.Service) *VKAuthHandler {
	return &VKAuthHandler{svc: svc, jwtService: jwtService}
}

// LoginWithVK handles POST /auth/vk
// Accepts a VK ID access token, verifies it with VK API, finds or creates user, returns JWT.
func (h *VKAuthHandler) LoginWithVK(c fiber.Ctx) error {
	var req struct {
		AccessToken string `json:"access_token"`
	}
	if err := c.Bind().JSON(&req); err != nil || req.AccessToken == "" {
		return c.Status(400).JSON(jwtlib.NewErrorResponse("invalid_request", "access_token required"))
	}

	// Verify token with VK API and get user info
	vkUser, err := fetchVKUserInfo(req.AccessToken)
	if err != nil {
		return c.Status(401).JSON(jwtlib.NewErrorResponse("invalid_token", err.Error()))
	}

	// Find or create user
	access, refresh, err := h.svc.LoginWithVKOAuth(vkUser.ID, vkUser.FirstName, vkUser.LastName, req.AccessToken)
	if err != nil {
		return c.Status(500).JSON(jwtlib.NewErrorResponse("auth_failed", err.Error()))
	}

	return c.JSON(jwtlib.NewAuthResponse(
		access, refresh, h.jwtService.GetAccessTokenExpiration(),
		nil, "vk_login_success"))
}

type vkUserInfo struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

func fetchVKUserInfo(accessToken string) (*vkUserInfo, error) {
	url := fmt.Sprintf("https://api.vk.com/method/users.get?access_token=%s&v=5.199&fields=", accessToken)
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("vk api unreachable: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Response []vkUserInfo `json:"response"`
		Error    *struct {
			ErrorMsg string `json:"error_msg"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("vk api parse error: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("vk api error: %s", result.Error.ErrorMsg)
	}
	if len(result.Response) == 0 {
		return nil, fmt.Errorf("vk user not found")
	}
	return &result.Response[0], nil
}
