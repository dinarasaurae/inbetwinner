package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/dinarasaurae/inbetwin-auth-service/internal/services"
	jwtlib "github.com/dinarasaurae/inbetwin-shared/jwt-go"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

type VKAuthHandler struct {
	svc               *services.AuthService
	jwtService        *jwtlib.Service
	vkAndroidClientID string
}

func NewVKAuthHandler(svc *services.AuthService, jwtService *jwtlib.Service, vkAndroidClientID string) *VKAuthHandler {
	return &VKAuthHandler{svc: svc, jwtService: jwtService, vkAndroidClientID: vkAndroidClientID}
}

// LoginWithVK handles POST /auth/vk
// Accepts a VK access token or a VK ID SDK token, finds or creates user, returns JWT.
func (h *VKAuthHandler) LoginWithVK(c fiber.Ctx) error {
	var req struct {
		AccessToken string `json:"access_token"`
		UserID      int64  `json:"user_id"`
		FirstName   string `json:"first_name"`
		LastName    string `json:"last_name"`
		IDToken     string `json:"id_token"`
		DeviceID    string `json:"device_id"`
	}
	if err := c.Bind().JSON(&req); err != nil || req.AccessToken == "" {
		return c.Status(400).JSON(jwtlib.NewErrorResponse("invalid_request", "access_token required"))
	}

	log.Printf(
		"vk auth: login request user_id=%d device_id_present=%t id_token_present=%t access_token=%s",
		req.UserID,
		req.DeviceID != "",
		req.IDToken != "",
		tokenPreview(req.AccessToken),
	)

	vkUser, err := resolveVKUserInfo(vkLoginRequest{
		AccessToken: req.AccessToken,
		UserID:      req.UserID,
		FirstName:   req.FirstName,
		LastName:    req.LastName,
		IDToken:     req.IDToken,
		DeviceID:    req.DeviceID,
	}, h.vkAndroidClientID)
	if err != nil {
		return c.Status(401).JSON(jwtlib.NewErrorResponse("invalid_token", err.Error()))
	}

	preferredUserID := h.extractPreferredUserID(c)

	// Find or create user
	access, refresh, err := h.svc.LoginWithVKOAuth(
		vkUser.ID,
		vkUser.FirstName,
		vkUser.LastName,
		req.AccessToken,
		preferredUserID,
	)
	if err != nil {
		return c.Status(500).JSON(jwtlib.NewErrorResponse("auth_failed", err.Error()))
	}

	return c.JSON(jwtlib.NewAuthResponse(
		access, refresh, h.jwtService.GetAccessTokenExpiration(),
		nil, "vk_login_success"))
}

func (h *VKAuthHandler) extractPreferredUserID(c fiber.Ctx) *uuid.UUID {
	token, err := jwtlib.ExtractTokenFromHeader(c.Get("Authorization"))
	if err != nil {
		return nil
	}

	claims, err := h.jwtService.ValidateAccessToken(token)
	if err != nil {
		return nil
	}

	userID := claims.UserID
	return &userID
}

type vkUserInfo struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type vkLoginRequest struct {
	AccessToken string
	UserID      int64
	FirstName   string
	LastName    string
	IDToken     string
	DeviceID    string
}

func resolveVKUserInfo(req vkLoginRequest, vkAndroidClientID string) (*vkUserInfo, error) {
	if req.DeviceID != "" && vkAndroidClientID != "" {
		vkUser, err := fetchVKIDUserInfo(req.AccessToken, vkAndroidClientID, req.DeviceID)
		if err == nil {
			if req.UserID == 0 {
				return nil, fmt.Errorf("vk id user_id required")
			}
			vkUser.ID = req.UserID
			log.Printf("vk auth: resolved via VK ID user_info user_id=%d", req.UserID)
			return vkUser, nil
		}
		log.Printf("vk auth: VK ID user_info verify failed device_id=%s user_id=%d err=%v", req.DeviceID, req.UserID, err)
	}

	// Legacy VK OAuth tokens can still be resolved via users.get. VK ID SDK tokens
	// are not accepted by that endpoint, so we keep an SDK payload fallback.
	vkUser, err := fetchVKUserInfo(req.AccessToken)
	if err == nil {
		log.Printf("vk auth: resolved via legacy users.get user_id=%d", vkUser.ID)
		return vkUser, nil
	}
	if req.UserID == 0 {
		return nil, err
	}
	log.Printf("vk auth: using VK ID SDK payload fallback user_id=%d legacy_verify_err=%v", req.UserID, err)
	return &vkUserInfo{
		ID:        req.UserID,
		FirstName: strings.TrimSpace(req.FirstName),
		LastName:  strings.TrimSpace(req.LastName),
	}, nil
}

func tokenPreview(token string) string {
	if token == "" {
		return "missing"
	}
	preview := token
	if len(preview) > 10 {
		preview = preview[:10]
	}
	return fmt.Sprintf("%s…(len=%d)", preview, len(token))
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

func fetchVKIDUserInfo(accessToken, clientID, deviceID string) (*vkUserInfo, error) {
	form := url.Values{}
	form.Set("access_token", accessToken)
	form.Set("device_id", deviceID)

	endpoint := fmt.Sprintf("https://id.vk.ru/oauth2/user_info?client_id=%s", url.QueryEscape(clientID))
	resp, err := http.PostForm(endpoint, form)
	if err != nil {
		return nil, fmt.Errorf("vk id api unreachable: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Error string `json:"error"`
		User  *struct {
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("vk id api parse error: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("vk id api error: %s", result.Error)
	}
	if result.User == nil {
		return nil, fmt.Errorf("vk id user not found")
	}

	return &vkUserInfo{
		ID:        0, // caller should still rely on SDK user_id for stable identifier
		FirstName: strings.TrimSpace(result.User.FirstName),
		LastName:  strings.TrimSpace(result.User.LastName),
	}, nil
}
