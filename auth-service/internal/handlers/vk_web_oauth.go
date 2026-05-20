package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	jwtlib "github.com/dinarasaurae/inbetwin-shared/jwt-go"
	"github.com/gofiber/fiber/v3"
)

const (
	vkOAuthBaseURL     = "https://oauth.vk.com"
	vkAPIVersion       = "5.199"
	vkWebOAuthStateTTL = 10 * time.Minute
)

type vkWebOAuthState struct {
	ReturnURL string
	ExpiresAt time.Time
}

type vkWebOAuthStateStore struct {
	mu     sync.Mutex
	states map[string]vkWebOAuthState
}

func newVKWebOAuthStateStore() *vkWebOAuthStateStore {
	return &vkWebOAuthStateStore{states: map[string]vkWebOAuthState{}}
}

func (s *vkWebOAuthStateStore) put(state, returnURL string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for key, value := range s.states {
		if now.After(value.ExpiresAt) {
			delete(s.states, key)
		}
	}

	s.states[state] = vkWebOAuthState{
		ReturnURL: returnURL,
		ExpiresAt: now.Add(ttl),
	}
}

func (s *vkWebOAuthStateStore) consume(state string) (vkWebOAuthState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	value, ok := s.states[state]
	if ok {
		delete(s.states, state)
	}
	if !ok || time.Now().After(value.ExpiresAt) {
		return vkWebOAuthState{}, false
	}
	return value, true
}

// WebVKOAuthStart handles GET /auth/vk/start.
// It starts the browser VK OAuth flow and stores a short-lived return URL nonce.
func (h *VKAuthHandler) WebVKOAuthStart(c fiber.Ctx) error {
	if strings.TrimSpace(h.vkWebClientID) == "" || strings.TrimSpace(h.vkWebClientSecret) == "" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(
			jwtlib.NewErrorResponse("vk_web_oauth_not_configured", "VK_APP_ID_WEB and VK_APP_SECRET_WEB are required"),
		)
	}
	if strings.TrimSpace(h.vkAuthRedirectURL) == "" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(
			jwtlib.NewErrorResponse("vk_web_oauth_not_configured", "VK_AUTH_REDIRECT_URI is required"),
		)
	}

	returnURL, ok := h.resolveVKAuthReturnURL(c.Query("return_url"))
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_return_url", "return_url is not allowed"),
		)
	}

	state, err := randomURLSafeToken(32)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			jwtlib.NewErrorResponse("state_generation_failed", err.Error()),
		)
	}

	h.webOAuthStates.put(state, returnURL, vkWebOAuthStateTTL)

	authURL := buildVKWebOAuthAuthorizeURL(h.vkWebClientID, h.vkAuthRedirectURL, state)
	return c.Redirect().To(authURL)
}

// WebVKOAuthCallback handles GET /auth/vk/callback.
// VK redirects here with a legacy OAuth code, then we issue normal app JWTs.
func (h *VKAuthHandler) WebVKOAuthCallback(c fiber.Ctx) error {
	state := c.Query("state")
	pending, ok := h.webOAuthStates.consume(state)
	if !ok {
		return c.Redirect().To(h.vkAuthFallbackErrorURL("invalid_state"))
	}

	if errParam := c.Query("error"); errParam != "" {
		reason := strings.TrimSpace(c.Query("error_description"))
		if reason == "" {
			reason = errParam
		}
		return c.Redirect().To(vkAuthReturnURLWithError(pending.ReturnURL, state, reason))
	}

	code := strings.TrimSpace(c.Query("code"))
	if code == "" {
		return c.Redirect().To(vkAuthReturnURLWithError(pending.ReturnURL, state, "missing_code"))
	}

	vkToken, err := h.exchangeVKWebOAuthCode(c.Context(), code)
	if err != nil {
		return c.Redirect().To(vkAuthReturnURLWithError(pending.ReturnURL, state, err.Error()))
	}

	vkUser, err := fetchVKUserInfo(vkToken.AccessToken)
	if err != nil {
		vkUser = &vkUserInfo{ID: vkToken.UserID}
	}
	if vkUser.ID == 0 {
		vkUser.ID = vkToken.UserID
	}
	if vkUser.ID == 0 {
		return c.Redirect().To(vkAuthReturnURLWithError(pending.ReturnURL, state, "vk_user_id_missing"))
	}

	access, refresh, err := h.svc.LoginWithVKOAuth(vkUser.ID, vkUser.FirstName, vkUser.LastName, vkToken.AccessToken, nil)
	if err != nil {
		return c.Redirect().To(vkAuthReturnURLWithError(pending.ReturnURL, state, err.Error()))
	}

	return c.Redirect().To(vkAuthReturnURLWithTokens(
		pending.ReturnURL,
		state,
		access,
		refresh,
		h.jwtService.GetAccessTokenExpiration(),
	))
}

type vkWebTokenResponse struct {
	AccessToken      string `json:"access_token"`
	ExpiresIn        int    `json:"expires_in"`
	UserID           int64  `json:"user_id"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func (h *VKAuthHandler) exchangeVKWebOAuthCode(ctx context.Context, code string) (*vkWebTokenResponse, error) {
	params := url.Values{}
	params.Set("client_id", h.vkWebClientID)
	params.Set("client_secret", h.vkWebClientSecret)
	params.Set("redirect_uri", h.vkAuthRedirectURL)
	params.Set("code", code)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, vkOAuthBaseURL+"/access_token?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vk token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	var result vkWebTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("vk token parse failed: %w", err)
	}
	if result.Error != "" {
		reason := result.ErrorDescription
		if reason == "" {
			reason = result.Error
		}
		return nil, fmt.Errorf("vk token error: %s", reason)
	}
	if result.AccessToken == "" {
		return nil, fmt.Errorf("vk token missing")
	}

	return &result, nil
}

func buildVKWebOAuthAuthorizeURL(clientID, redirectURI, state string) string {
	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("display", "page")
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("v", vkAPIVersion)
	params.Set("state", state)

	return vkOAuthBaseURL + "/oauth/authorize?" + params.Encode()
}

func (h *VKAuthHandler) resolveVKAuthReturnURL(raw string) (string, bool) {
	if strings.TrimSpace(raw) == "" {
		return strings.TrimRight(h.frontendURL, "/") + "/auth/vk/callback", true
	}
	if isAllowedVKAuthReturnURL(raw, h.frontendURL, h.allowedReturnURLs) {
		return raw, true
	}
	return "", false
}

func isAllowedVKAuthReturnURL(raw, frontendURL string, allowed []string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}

	if sameOrigin(raw, frontendURL) {
		return true
	}

	host := parsed.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}

	for _, candidate := range allowed {
		if sameURLWithoutQuery(raw, candidate) || sameOrigin(raw, candidate) {
			return true
		}
	}

	return false
}

func sameOrigin(left, right string) bool {
	leftURL, err := url.Parse(left)
	if err != nil || leftURL.Scheme == "" || leftURL.Host == "" {
		return false
	}
	rightURL, err := url.Parse(right)
	if err != nil || rightURL.Scheme == "" || rightURL.Host == "" {
		return false
	}
	return strings.EqualFold(leftURL.Scheme, rightURL.Scheme) &&
		strings.EqualFold(leftURL.Host, rightURL.Host)
}

func sameURLWithoutQuery(left, right string) bool {
	leftURL, err := url.Parse(left)
	if err != nil {
		return false
	}
	rightURL, err := url.Parse(right)
	if err != nil {
		return false
	}
	leftURL.RawQuery = ""
	leftURL.Fragment = ""
	rightURL.RawQuery = ""
	rightURL.Fragment = ""
	return leftURL.String() == rightURL.String()
}

func (h *VKAuthHandler) vkAuthFallbackErrorURL(reason string) string {
	base := strings.TrimRight(h.frontendURL, "/") + "/vk-error"
	parsed, err := url.Parse(base)
	if err != nil {
		return "/"
	}
	q := parsed.Query()
	q.Set("reason", reason)
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

func vkAuthReturnURLWithError(returnURL, state, reason string) string {
	values := url.Values{}
	values.Set("error", "vk_auth_failed")
	values.Set("reason", reason)
	if state != "" {
		values.Set("state", state)
	}
	return withFragmentValues(returnURL, values)
}

func vkAuthReturnURLWithTokens(returnURL, state, accessToken, refreshToken string, expiresIn int) string {
	values := url.Values{}
	values.Set("provider", "vk")
	values.Set("access_token", accessToken)
	values.Set("refresh_token", refreshToken)
	values.Set("expires_in", fmt.Sprintf("%d", expiresIn))
	if state != "" {
		values.Set("state", state)
	}
	return withFragmentValues(returnURL, values)
}

func withFragmentValues(raw string, values url.Values) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.Fragment = values.Encode()
	return parsed.String()
}

func randomURLSafeToken(byteCount int) (string, error) {
	buf := make([]byte, byteCount)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
