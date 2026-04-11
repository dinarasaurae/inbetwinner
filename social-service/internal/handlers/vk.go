package handlers

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	jwtlib "github.com/dinarasaurae/inbetwin-shared/jwt-go"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/dinarasaurae/inbetwin-social-service/internal/models"
	"github.com/dinarasaurae/inbetwin-social-service/internal/services"
)

// VKHandler handles VK integration HTTP routes.
type VKHandler struct {
	svc     *services.VKService
	frontendURL string
}

// NewVKHandler creates a new VKHandler.
func NewVKHandler(svc *services.VKService, frontendURL string) *VKHandler {
	return &VKHandler{svc: svc, frontendURL: frontendURL}
}

// ─── User OAuth ───────────────────────────────────────────────────────────────

// UserOAuthStart handles GET /social/vk/oauth/user/start?platform=web|android|ios
// Returns the VK authorisation URL for the client to open.
func (h *VKHandler) UserOAuthStart(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	platform := normalisePlatform(c.Query("platform"))

	authURL, state, implicit, err := h.svc.UserOAuthStart(c.Context(), userID, platform)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			jwtlib.NewErrorResponse("oauth_start_failed", err.Error()))
	}

	return c.JSON(jwtlib.NewSuccessResponse("", models.VKUserOAuthStartResponse{
		AuthURL:      authURL,
		State:        state,
		Platform:     platform,
		ImplicitFlow: implicit,
	}))
}

// UserOAuthExchange handles POST /social/vk/oauth/user/exchange
// Called by the mobile app after intercepting the VK deep-link redirect.
func (h *VKHandler) UserOAuthExchange(c fiber.Ctx) error {
	_, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	var req models.VKUserOAuthExchangeRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_request", err.Error()))
	}
	if req.Code == "" || req.State == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_request", "code and state are required"))
	}
	req.Platform = normalisePlatform(req.Platform)

	conn, err := h.svc.UserOAuthExchange(c.Context(), req)
	if err != nil {
		return mapVKError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(jwtlib.NewSuccessResponse("vk_user_connected", conn))
}

// UserOAuthCallback handles GET /social/vk/oauth/user/callback   (PUBLIC — no JWT)
// VK redirects the user's browser here after web OAuth.
func (h *VKHandler) UserOAuthCallback(c fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")
	errParam := c.Query("error")
	platform, _ := h.svc.OAuthPlatformForState(state)

	if errParam != "" {
		desc := c.Query("error_description")
		return c.Redirect().To(userOAuthReturnURL(platform, state, "", desc, h.frontendURL))
	}
	if code == "" || state == "" {
		return c.Redirect().To(userOAuthReturnURL(platform, state, "", "missing_params", h.frontendURL))
	}

	deviceID := c.Query("device_id") // VK ID PKCE — returned alongside the code
	_, err := h.svc.UserOAuthCallback(c.Context(), code, state, deviceID)
	if err != nil {
		return c.Redirect().To(userOAuthReturnURL(platform, state, "", err.Error(), h.frontendURL))
	}

	// After connecting the user account, redirect to the group-selection page.
	return c.Redirect().To(userOAuthReturnURL(platform, state, "1", "", h.frontendURL))
}

// ─── Group OAuth ──────────────────────────────────────────────────────────────

// OAuthStart handles GET /social/vk/oauth/start?group_id={id}&platform=web|android|ios
// Returns the VK authorisation URL with group_ids so VK issues a community token.
func (h *VKHandler) OAuthStart(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	groupIDStr := c.Query("group_id")
	groupRef := c.Query("group_ref")
	var (
		groupID int64
		err error
	)
	if groupRef != "" {
		groupID, err = h.svc.ResolveGroupRef(c.Context(), userID, groupRef)
	} else {
		groupID, err = strconv.ParseInt(groupIDStr, 10, 64)
		if err == nil && groupID <= 0 {
			err = fmt.Errorf("group_id must be a positive integer")
		}
	}
	if err != nil || groupID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_group_id", "Provide group_id or group_ref"))
	}

	platform := normalisePlatform(c.Query("platform"))

	authURL, state, err := h.svc.OAuthStart(c.Context(), userID, groupID, platform)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			jwtlib.NewErrorResponse("oauth_start_failed", err.Error()))
	}

	return c.JSON(jwtlib.NewSuccessResponse("", models.OAuthStartResponse{
		AuthURL:  authURL,
		State:    state,
		Platform: platform,
		GroupID:  groupID,
	}))
}

// OAuthExchange handles POST /social/vk/oauth/exchange
// Called by the mobile app to complete group OAuth.
func (h *VKHandler) OAuthExchange(c fiber.Ctx) error {
	_, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	var req models.ConnectVKRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_request", err.Error()))
	}
	if req.Code == "" || req.State == "" {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_request", "code and state are required"))
	}

	integ, err := h.svc.OAuthExchange(c.Context(), req.Code, req.State)
	if err != nil {
		return mapVKError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(jwtlib.NewSuccessResponse("vk_group_connected", integ))
}

// OAuthCallback handles GET /social/vk/oauth/callback   (PUBLIC — no JWT)
// VK redirects the user's browser here after web group OAuth.
func (h *VKHandler) OAuthCallback(c fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")
	errParam := c.Query("error")
	platform, _ := h.svc.OAuthPlatformForState(state)

	if errParam != "" {
		desc := c.Query("error_description")
		return c.Redirect().To(groupOAuthErrorReturnURL(platform, state, desc, h.frontendURL))
	}
	if code == "" || state == "" {
		return c.Redirect().To(groupOAuthErrorReturnURL(platform, state, "missing_params", h.frontendURL))
	}

	integ, platform, err := h.svc.OAuthCallback(c.Context(), code, state)
	if err != nil {
		return c.Redirect().To(groupOAuthErrorReturnURL(platform, state, err.Error(), h.frontendURL))
	}

	return c.Redirect().To(groupOAuthReturnURL(platform, state, integ, h.frontendURL))
}

func userOAuthReturnURL(platform, state, connected, reason, frontendURL string) string {
	switch normalisePlatform(platform) {
	case "android":
		if reason != "" {
			return fmt.Sprintf("inbetwin://vk-callback?error=%s&state=%s", url.QueryEscape(reason), state)
		}
		return fmt.Sprintf("inbetwin://vk-callback?legacy_user_connected=%s&state=%s", connected, state)
	case "ios":
		if reason != "" {
			return fmt.Sprintf("https://inbetwin.ru/vk-redirect?error=%s&state=%s", url.QueryEscape(reason), state)
		}
		return fmt.Sprintf("https://inbetwin.ru/vk-redirect?legacy_user_connected=%s&state=%s", connected, state)
	default:
		if reason != "" {
			return fmt.Sprintf("%s/vk-error?reason=%s", frontendURL, reason)
		}
		return frontendURL + "/dashboard/vk/groups"
	}
}

func groupOAuthReturnURL(platform, state string, integ *models.VKIntegration, frontendURL string) string {
	switch normalisePlatform(platform) {
	case "android":
		return fmt.Sprintf(
			"inbetwin://vk-callback?group_connected=1&state=%s&group_id=%d&integration_id=%s",
			state, integ.GroupID, integ.ID.String(),
		)
	case "ios":
		return fmt.Sprintf(
			"https://inbetwin.ru/vk-redirect?group_connected=1&state=%s&group_id=%d&integration_id=%s",
			state, integ.GroupID, integ.ID.String(),
		)
	default:
		return fmt.Sprintf(
			"%s/dashboard/vk/groups?group_connected=%d",
			frontendURL, integ.GroupID,
		)
	}
}

func groupOAuthErrorReturnURL(platform, state, reason, frontendURL string) string {
	switch normalisePlatform(platform) {
	case "android":
		return fmt.Sprintf("inbetwin://vk-callback?error=%s&state=%s", url.QueryEscape(reason), state)
	case "ios":
		return fmt.Sprintf("https://inbetwin.ru/vk-redirect?error=%s&state=%s", url.QueryEscape(reason), state)
	default:
		return fmt.Sprintf("%s/vk-error?reason=%s", frontendURL, reason)
	}
}

// ─── Data endpoints ───────────────────────────────────────────────────────────

// GetAdminGroups handles GET /social/vk/groups/admin
// Lists VK groups where the authenticated user is admin.
func (h *VKHandler) GetAdminGroups(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	groups, err := h.svc.GetAdminGroups(c.Context(), userID)
	if err != nil {
		return mapVKError(c, err)
	}
	if groups == nil {
		groups = []models.VKAdminGroup{}
	}

	return c.JSON(jwtlib.NewSuccessResponse("", fiber.Map{
		"count":  len(groups),
		"groups": groups,
	}))
}

// GetUserProfile handles GET /social/vk/user/profile
func (h *VKHandler) GetUserProfile(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	profile, err := h.svc.GetUserProfile(c.Context(), userID)
	if err != nil {
		return mapVKError(c, err)
	}

	return c.JSON(jwtlib.NewSuccessResponse("", profile))
}

// GetUserSubscriptions handles GET /social/vk/user/subscriptions
func (h *VKHandler) GetUserSubscriptions(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	subs, err := h.svc.GetUserSubscriptions(c.Context(), userID)
	if err != nil {
		return mapVKError(c, err)
	}

	return c.JSON(jwtlib.NewSuccessResponse("", subs))
}

// GetUserPosts handles GET /social/vk/user/posts?count=50
func (h *VKHandler) GetUserPosts(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	count, _ := strconv.Atoi(c.Query("count", "50"))

	posts, err := h.svc.GetUserPosts(c.Context(), userID, count)
	if err != nil {
		return mapVKError(c, err)
	}

	return c.JSON(jwtlib.NewSuccessResponse("", posts))
}

// ─── Connected groups & management ───────────────────────────────────────────

// ListIntegrations handles GET /social/vk/groups
func (h *VKHandler) ListIntegrations(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	list, err := h.svc.ListIntegrations(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			jwtlib.NewErrorResponse("query_failed", err.Error()))
	}
	if list == nil {
		list = []*models.VKIntegration{}
	}

	return c.JSON(jwtlib.NewSuccessResponse("", fiber.Map{
		"count":  len(list),
		"groups": list,
	}))
}

// Disconnect handles DELETE /social/vk/disconnect
func (h *VKHandler) Disconnect(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	var req struct {
		IntegrationID string `json:"integration_id"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_request", err.Error()))
	}

	integID, err := parseUUID(req.IntegrationID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_integration_id", nil))
	}

	if err := h.svc.Disconnect(c.Context(), userID, integID); err != nil {
		return mapVKError(c, err)
	}

	return c.JSON(jwtlib.NewSuccessResponse("vk_disconnected", nil))
}

// SyncPosts handles POST /social/vk/sync
func (h *VKHandler) SyncPosts(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	var req models.SyncVKRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_request", err.Error()))
	}

	integID, err := parseUUID(req.IntegrationID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_integration_id", nil))
	}

	imported, err := h.svc.SyncPosts(c.Context(), userID, integID, req.Count)
	if err != nil {
		return mapVKError(c, err)
	}

	return c.JSON(jwtlib.NewSuccessResponse("", fiber.Map{"imported": imported}))
}

// SendMessage handles POST /social/vk/message
func (h *VKHandler) SendMessage(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	var req models.VKSendMessageRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_request", err.Error()))
	}
	if req.Text == "" || req.ToVKUserID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_request", "to_vk_user_id and text are required"))
	}

	integID, err := parseUUID(req.IntegrationID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_integration_id", nil))
	}

	if err := h.svc.SendMessage(c.Context(), userID, integID, req.ToVKUserID, req.Text); err != nil {
		return mapVKError(c, err)
	}

	return c.JSON(jwtlib.NewSuccessResponse("message_sent", nil))
}

// EnrichLead handles GET /social/vk/lead/:vk_user_id
func (h *VKHandler) EnrichLead(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	vkUserID, err := strconv.ParseInt(c.Params("vk_user_id"), 10, 64)
	if err != nil || vkUserID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_vk_user_id", nil))
	}

	integIDStr := c.Query("integration_id")
	integID, err := parseUUID(integIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_integration_id", nil))
	}

	profile, err := h.svc.EnrichLead(c.Context(), userID, integID, vkUserID)
	if err != nil {
		return mapVKError(c, err)
	}

	return c.JSON(jwtlib.NewSuccessResponse("", profile))
}

// SaveCommunityTokenByInteg handles POST /social/vk/community-access/token
// Mobile-friendly: accepts {integration_id, community_token} in body.
func (h *VKHandler) SaveCommunityTokenByInteg(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	var body struct {
		IntegrationID string `json:"integration_id"`
		CommunityToken string `json:"community_token"`
	}
	if err := c.Bind().JSON(&body); err != nil || strings.TrimSpace(body.CommunityToken) == "" || strings.TrimSpace(body.IntegrationID) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("invalid_request", "integration_id and community_token are required"))
	}

	integID, err := uuid.Parse(body.IntegrationID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("invalid_request", "invalid integration_id"))
	}

	integ, err := h.svc.SaveCommunityTokenByInteg(c.Context(), userID, integID, strings.TrimSpace(body.CommunityToken))
	if err != nil {
		return mapVKError(c, err)
	}
	return c.JSON(jwtlib.NewSuccessResponse("community token saved", integ))
}

// CommunityAccessStart handles GET /social/vk/community-access/start?integration_id=xxx&platform=xxx
// Mobile-friendly alias: looks up group_id from integration_id, then starts the VK community OAuth flow.
func (h *VKHandler) CommunityAccessStart(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	integIDStr := c.Query("integration_id")
	integID, err := uuid.Parse(integIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(
			jwtlib.NewErrorResponse("invalid_request", "integration_id is required and must be a valid UUID"))
	}

	platform := normalisePlatform(c.Query("platform"))

	groupID, err := h.svc.GetGroupIDByIntegration(c.Context(), userID, integID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(jwtlib.NewErrorResponse("not_found", err.Error()))
	}

	authURL, state, err := h.svc.OAuthStart(c.Context(), userID, groupID, platform)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(
			jwtlib.NewErrorResponse("oauth_start_failed", err.Error()))
	}

	return c.JSON(jwtlib.NewSuccessResponse("", models.OAuthStartResponse{
		AuthURL:  authURL,
		State:    state,
		Platform: platform,
		GroupID:  groupID,
	}))
}

// SaveCommunityToken handles POST /social/vk/groups/:group_id/token
// Allows saving a manually created community token (from VK community management panel).
func (h *VKHandler) SaveCommunityToken(c fiber.Ctx) error {
	userID, ok := jwtlib.GetUserID(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", nil))
	}

	groupIDStr := c.Params("group_id")
	groupID, err := strconv.ParseInt(groupIDStr, 10, 64)
	if err != nil || groupID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("invalid_group_id", "group_id must be a positive integer"))
	}

	var body struct {
		Token string `json:"token"`
	}
	if err := c.Bind().JSON(&body); err != nil || strings.TrimSpace(body.Token) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("invalid_request", "token is required"))
	}

	integ, err := h.svc.SaveCommunityToken(c.Context(), userID, groupID, strings.TrimSpace(body.Token))
	if err != nil {
		return mapVKError(c, err)
	}
	return c.JSON(jwtlib.NewSuccessResponse("community token saved", integ))
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func normalisePlatform(p string) string {
	switch strings.ToLower(p) {
	case "android":
		return "android"
	case "ios":
		return "ios"
	default:
		return "web"
	}
}

func mapVKError(c fiber.Ctx, err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "vk error 1051"):
		return c.Status(fiber.StatusUnprocessableEntity).JSON(jwtlib.NewErrorResponse("profile_type_unsupported", msg))
	case strings.Contains(msg, "not_found"):
		return c.Status(fiber.StatusNotFound).JSON(jwtlib.NewErrorResponse("not_found", msg))
	case strings.Contains(msg, "unauthorized") || strings.Contains(msg, "invalid_state"):
		return c.Status(fiber.StatusUnauthorized).JSON(jwtlib.NewErrorResponse("unauthorized", msg))
	case strings.Contains(msg, "invalid_request") || strings.Contains(msg, "invalid_group_id"):
		return c.Status(fiber.StatusBadRequest).JSON(jwtlib.NewErrorResponse("invalid_request", msg))
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(jwtlib.NewErrorResponse("internal_error", msg))
	}
}
