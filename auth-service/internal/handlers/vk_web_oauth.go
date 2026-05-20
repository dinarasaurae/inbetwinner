package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	jwtlib "github.com/dinarasaurae/inbetwin-shared/jwt-go"
	"github.com/gofiber/fiber/v3"
)

const (
	vkIDAuthURL        = "https://id.vk.com/authorize"
	vkIDTokenURL       = "https://id.vk.ru/oauth2/auth"
	vkWebOAuthStateTTL = 10 * time.Minute
)

type vkWebOAuthState struct {
	ReturnURL    string
	CodeVerifier string
	ExpiresAt    time.Time
}

type vkWebOAuthStateStore struct {
	mu     sync.Mutex
	states map[string]vkWebOAuthState
}

func newVKWebOAuthStateStore() *vkWebOAuthStateStore {
	return &vkWebOAuthStateStore{states: map[string]vkWebOAuthState{}}
}

func (s *vkWebOAuthStateStore) put(state, returnURL, codeVerifier string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for key, value := range s.states {
		if now.After(value.ExpiresAt) {
			delete(s.states, key)
		}
	}

	s.states[state] = vkWebOAuthState{
		ReturnURL:    returnURL,
		CodeVerifier: codeVerifier,
		ExpiresAt:    now.Add(ttl),
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
	if strings.TrimSpace(h.vkWebClientID) == "" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(
			jwtlib.NewErrorResponse("vk_web_oauth_not_configured", "VK_APP_ID_WEB is required"),
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
	codeVerifier, codeChallenge, err := newPKCEPair()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			jwtlib.NewErrorResponse("pkce_generation_failed", err.Error()),
		)
	}

	h.webOAuthStates.put(state, returnURL, codeVerifier, vkWebOAuthStateTTL)

	authURL := buildVKWebOAuthAuthorizeURL(h.vkWebClientID, h.vkAuthRedirectURL, state, codeChallenge)
	return c.Redirect().To(authURL)
}

// WebVKOAuthCallback handles GET /auth/vk/callback.
// VK redirects here with a VK ID OAuth code, then we issue normal app JWTs.
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
	if pending.CodeVerifier == "" {
		return c.Redirect().To(vkAuthReturnURLWithError(pending.ReturnURL, state, "missing_pkce_verifier"))
	}

	deviceID := strings.TrimSpace(c.Query("device_id"))
	vkToken, err := h.exchangeVKWebOAuthCode(c.Context(), code, deviceID, pending.CodeVerifier)
	if err != nil {
		return c.Redirect().To(vkAuthReturnURLWithError(pending.ReturnURL, state, err.Error()))
	}

	vkUser := &vkUserInfo{ID: vkToken.vkUserID()}
	if deviceID != "" {
		if verifiedUser, err := fetchVKIDUserInfo(vkToken.AccessToken, h.vkWebClientID, deviceID); err == nil {
			if vkUser.ID != 0 {
				verifiedUser.ID = vkUser.ID
			}
			vkUser = verifiedUser
		}
	}
	if vkUser.ID == 0 {
		if legacyUser, err := fetchVKUserInfo(vkToken.AccessToken); err == nil {
			vkUser = legacyUser
		}
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
	AccessToken      string          `json:"access_token"`
	ExpiresIn        int             `json:"expires_in"`
	UserID           json.RawMessage `json:"user_id"`
	Error            string          `json:"error"`
	ErrorDescription string          `json:"error_description"`
}

func (h *VKAuthHandler) exchangeVKWebOAuthCode(ctx context.Context, code, deviceID, codeVerifier string) (*vkWebTokenResponse, error) {
	params := url.Values{}
	params.Set("grant_type", "authorization_code")
	params.Set("client_id", h.vkWebClientID)
	params.Set("code", code)
	params.Set("code_verifier", codeVerifier)
	params.Set("redirect_uri", h.vkAuthRedirectURL)
	params.Set("state", "")
	if deviceID != "" {
		params.Set("device_id", deviceID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, vkIDTokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vk token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("vk token read failed: %w", err)
	}

	var result vkWebTokenResponse
	if err := json.Unmarshal(body, &result); err != nil {
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

func (r *vkWebTokenResponse) vkUserID() int64 {
	if len(r.UserID) == 0 {
		return 0
	}

	var numeric int64
	if err := json.Unmarshal(r.UserID, &numeric); err == nil {
		return numeric
	}

	var stringValue string
	if err := json.Unmarshal(r.UserID, &stringValue); err == nil {
		var parsed int64
		if _, scanErr := fmt.Sscanf(stringValue, "%d", &parsed); scanErr == nil {
			return parsed
		}
	}

	return 0
}

func buildVKWebOAuthAuthorizeURL(clientID, redirectURI, state, codeChallenge string) string {
	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", "vkid.personal_info")
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")

	return vkIDAuthURL + "?" + params.Encode()
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

func newPKCEPair() (verifier string, challenge string, err error) {
	verifier, err = randomURLSafeToken(32)
	if err != nil {
		return "", "", err
	}

	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}
