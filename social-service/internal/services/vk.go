package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/dinarasaurae/inbetwin-social-service/internal/config"
	"github.com/dinarasaurae/inbetwin-social-service/internal/crypto"
	"github.com/dinarasaurae/inbetwin-social-service/internal/database"
	"github.com/dinarasaurae/inbetwin-social-service/internal/models"
	vkapi "github.com/dinarasaurae/inbetwin-social-service/internal/vk"
)

// ─── OAuth state store ────────────────────────────────────────────────────────

type pendingOAuth struct {
	userID       uuid.UUID
	groupID      int64  // 0 for user OAuth, >0 for group OAuth
	platform     string // "web" | "android" | "ios"
	codeVerifier string // PKCE — sent to VK ID during token exchange
	expiry       time.Time
}

// ─── VKService ────────────────────────────────────────────────────────────────

// VKService handles VK group integration lifecycle.
type VKService struct {
	db            *database.DB
	enc           *crypto.Encryptor
	cfg           *config.Config
	llmClient     *VKLLMClient // nil when LLM_SERVICE_URL is not configured
	draftProvider ChatCompletionProvider

	// oauthStates stores short-lived CSRF nonces for all OAuth flows.
	oauthMu     sync.Mutex
	oauthStates map[string]pendingOAuth

	// workers tracks running Long Poll goroutines per integration ID.
	workerMu      sync.Mutex
	workerCancels map[uuid.UUID]context.CancelFunc
}

// NewVKService creates a new VKService.
// llmClient is wired from cfg.LLMServiceURL; nil means legacy-only mode.
func NewVKService(db *database.DB, enc *crypto.Encryptor, cfg *config.Config) *VKService {
	var llmClient *VKLLMClient
	if cfg.LLMServiceURL != "" {
		llmClient = NewVKLLMClient(cfg.LLMServiceURL)
	}
	svc := &VKService{
		db:            db,
		enc:           enc,
		cfg:           cfg,
		llmClient:     llmClient,
		draftProvider: NewChatCompletionProvider(cfg),
		oauthStates:   make(map[string]pendingOAuth),
		workerCancels: make(map[uuid.UUID]context.CancelFunc),
	}
	go svc.cleanExpiredStates()
	return svc
}

func (s *VKService) cleanExpiredStates() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		s.oauthMu.Lock()
		for k, v := range s.oauthStates {
			if v.expiry.Before(now) {
				delete(s.oauthStates, k)
			}
		}
		s.oauthMu.Unlock()
	}
}

// ─── User OAuth flow ──────────────────────────────────────────────────────────
// Step 1 of connecting VK: user authorises the inBeTwin app.
// After this we can list admin groups and fetch the user's own profile data.

// UserOAuthStart generates a VK ID OAuth 2.1 authorisation URL (PKCE required).
// platform: "web" | "android" | "ios"
//
// VK ID OAuth 2.1 changes vs old oauth.vk.com:
//   - Endpoint:       id.vk.ru/oauth2/auth
//   - PKCE mandatory: code_challenge + code_challenge_method=S256
//   - No client_secret needed (code_verifier replaces it in exchange)
//   - Implicit flow removed (response_type=token no longer works)
//   - device_id returned in callback, required for token exchange
func (s *VKService) UserOAuthStart(ctx context.Context, userID uuid.UUID, platform string) (authURL, state string, implicit bool, err error) {
	// Mobile platforms must use the legacy oauth.vk.com path so that step 1
	// (user OAuth) and step 2 (group/community OAuth) share the same browser
	// session domain. VK ID OAuth 2.1 authenticates on id.vk.ru; when the
	// wizard then opens oauth.vk.com for the group token the browser has no
	// session there and shows a second login prompt, breaking the auto-flow.
	if platform == "android" || platform == "ios" {
		return s.UserLegacyOAuthStart(ctx, userID, platform)
	}

	pc := s.cfg.VKPlatform(platform)
	if pc.AppID == "" {
		return "", "", false, fmt.Errorf("VK app ID not configured for platform %q", platform)
	}

	// Generate PKCE code_verifier (43-128 chars, URL-safe base64)
	verifierBytes := make([]byte, 32)
	if _, err = rand.Read(verifierBytes); err != nil {
		return "", "", false, fmt.Errorf("pkce verifier: %w", err)
	}
	codeVerifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	// code_challenge = BASE64URL(SHA256(code_verifier)), no padding
	h := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(h[:])

	state = s.newState()
	s.oauthMu.Lock()
	s.oauthStates[state] = pendingOAuth{
		userID:       userID,
		groupID:      0,
		platform:     platform,
		codeVerifier: codeVerifier,
		expiry:       time.Now().Add(15 * time.Minute),
	}
	s.oauthMu.Unlock()

	// VK ID OAuth 2.1 authorization URL (opened in browser/WebView)
	// Scopes: vkid.personal_info (basic profile) + groups + wall + offline
	authURL = fmt.Sprintf(
		"%s?response_type=code&client_id=%s&redirect_uri=%s"+
			"&scope=%s&state=%s&code_challenge=%s&code_challenge_method=S256",
		vkapi.VKIDAuthURL,
		url.QueryEscape(pc.AppID),
		url.QueryEscape(pc.RedirectURI),
		url.QueryEscape("vkid.personal_info groups wall offline"),
		state,
		codeChallenge,
	)
	return authURL, state, false, nil
}

// UserLegacyOAuthStart builds the pre-VK-ID browser auth URL on oauth.vk.com.
// We keep this preparatory step close to the historically working flow:
// the same browser-based authorize URL with code exchange, because the old
// auto-token path relied on that auth surface being reused for group OAuth.
//
// For android/ios the web app (54511648) is used intentionally.
// The platform-specific apps (54511649/54511650) only have vk{id}://vk.ru/blank.html
// registered as redirect URIs for VK ID SDK — oauth.vk.com rejects any other scheme
// with a Security Error.  The web app's server-side callback URL is already registered
// and the handler redirects the browser to inbetwin://vk-callback after token exchange,
// which the app intercepts via its intent filter.  Both wizard steps then run on
// oauth.vk.com so the browser session is shared and VK skips the second login prompt.
func (s *VKService) UserLegacyOAuthStart(ctx context.Context, userID uuid.UUID, platform string) (authURL, state string, implicit bool, err error) {
	var pc config.VKPlatformConfig
	if platform == "android" || platform == "ios" {
		// Use web app with its registered HTTPS redirect URI.
		// Group OAuth (step 2) also uses the web app, so both steps share
		// the same oauth.vk.com browser session — no second login needed.
		pc = s.cfg.VKPlatform("web")
	} else {
		pc = s.cfg.VKLegacyMobilePlatform(platform)
	}
	if pc.AppID == "" {
		return "", "", false, fmt.Errorf("VK app ID not configured for platform %q", platform)
	}

	state = s.newState()
	s.oauthMu.Lock()
	s.oauthStates[state] = pendingOAuth{
		userID:   userID,
		groupID:  0,
		platform: platform,
		expiry:   time.Now().Add(10 * time.Minute),
	}
	s.oauthMu.Unlock()

	authURL = fmt.Sprintf(
		"%s/authorize?client_id=%s&display=page&redirect_uri=%s"+
			"&scope=groups,wall,offline&response_type=code&v=%s&state=%s",
		vkapi.OAuthBase,
		url.QueryEscape(pc.AppID),
		url.QueryEscape(pc.RedirectURI),
		vkapi.APIVersion,
		state,
	)
	log.Printf("vk: legacy user oauth start user=%s platform=%s app_id=%s redirect_uri=%s auth_url=%s", userID, platform, pc.AppID, pc.RedirectURI, authURL)
	return authURL, state, false, nil
}

// UserOAuthExchange is called by the mobile app after intercepting the deep-link
// redirect from VK.
//
// VK ID OAuth 2.1: code + device_id (from callback) + code_verifier (PKCE, stored server-side)
func (s *VKService) UserOAuthExchange(ctx context.Context, req models.VKUserOAuthExchangeRequest) (*models.VKUserConnection, error) {
	pending, ok := s.popState(req.State)
	if !ok {
		return nil, fmt.Errorf("invalid_state: OAuth state not found or expired")
	}
	if req.Platform != "" {
		pending.platform = req.Platform
	}

	if req.AccessToken != "" {
		return s.finishUserOAuthImplicit(ctx, req.AccessToken, req.VKUserID, pending)
	}
	if pending.codeVerifier == "" {
		return s.finishUserOAuthLegacy(ctx, req.Code, pending)
	}
	return s.finishUserOAuth(ctx, req.Code, req.DeviceID, pending)
}

// UserOAuthCallback is called by the server-side web callback
// (VK redirects the browser here). device_id comes from the query param.
func (s *VKService) UserOAuthCallback(ctx context.Context, code, state, deviceID string) (*models.VKUserConnection, error) {
	pending, ok := s.popState(state)
	if !ok {
		return nil, fmt.Errorf("invalid_state: OAuth state not found or expired")
	}
	// Legacy wizard flow (UserLegacyOAuthStart) stores no codeVerifier because
	// it uses oauth.vk.com (not VK ID 2.1 PKCE).  Route accordingly.
	if pending.codeVerifier == "" {
		return s.finishUserOAuthLegacy(ctx, code, pending)
	}
	return s.finishUserOAuth(ctx, code, deviceID, pending)
}

func (s *VKService) OAuthPlatformForState(state string) (string, bool) {
	pending, ok := s.peekState(state)
	if !ok {
		return "", false
	}
	return pending.platform, true
}

// finishUserOAuth exchanges the VK ID authorization code for an access token
// using the PKCE flow (id.vk.ru/oauth2/auth, no client_secret needed).
// deviceID is the VK-specific value returned in the redirect URI alongside the code.
func (s *VKService) finishUserOAuth(ctx context.Context, code, deviceID string, pending pendingOAuth) (*models.VKUserConnection, error) {
	pc := s.cfg.VKPlatform(pending.platform)

	resp, err := s.exchangeCodePKCE(ctx, code, deviceID, pending.codeVerifier, pc)
	if err != nil {
		return nil, fmt.Errorf("token_exchange: %w", err)
	}

	userToken := resp["access_token"]
	if userToken == "" {
		return nil, fmt.Errorf("user_token_missing: no access_token in OAuth response")
	}
	// VK ID returns user_id inside the token JWT or as a separate field.
	vkUserIDStr := resp["user_id"]
	var vkUserID int64
	fmt.Sscanf(vkUserIDStr, "%d", &vkUserID)
	if vkUserID == 0 {
		if me, meErr := vkapi.NewUserClient(userToken).UsersGetMe(ctx); meErr == nil {
			vkUserID = me.ID
		}
	}

	return s.saveUserConnection(ctx, pending.userID, vkUserID, pending.platform, userToken)
}

func (s *VKService) finishUserOAuthLegacy(ctx context.Context, code string, pending pendingOAuth) (*models.VKUserConnection, error) {
	// For android/ios the start used the web app (see UserLegacyOAuthStart),
	// so the exchange must also use web app credentials.
	var pc config.VKPlatformConfig
	if pending.platform == "android" || pending.platform == "ios" {
		pc = s.cfg.VKPlatform("web")
	} else {
		pc = s.cfg.VKLegacyMobilePlatform(pending.platform)
	}

	resp, err := s.exchangeCode(ctx, code, pc)
	if err != nil {
		return nil, fmt.Errorf("legacy_token_exchange: %w", err)
	}

	userToken := resp["access_token"]
	if userToken == "" {
		return nil, fmt.Errorf("user_token_missing: no access_token in OAuth response")
	}
	vkUserIDStr := resp["user_id"]
	var vkUserID int64
	fmt.Sscanf(vkUserIDStr, "%d", &vkUserID)
	if vkUserID == 0 {
		if me, meErr := vkapi.NewUserClient(userToken).UsersGetMe(ctx); meErr == nil {
			vkUserID = me.ID
		}
	}

	return s.saveUserConnection(ctx, pending.userID, vkUserID, pending.platform, userToken)
}

// finishUserOAuthImplicit stores a token that arrived directly in the redirect
// URI fragment (implicit flow, response_type=token). No code exchange needed.
func (s *VKService) finishUserOAuthImplicit(ctx context.Context, accessToken string, vkUserID int64, pending pendingOAuth) (*models.VKUserConnection, error) {
	if accessToken == "" {
		return nil, fmt.Errorf("implicit_flow: access_token is empty")
	}

	return s.saveUserConnection(ctx, pending.userID, vkUserID, pending.platform, accessToken)
}

// ImportUserOAuthToken stores a VK access token that was already obtained by the
// mobile app during app login, avoiding a second VK account auth step.
func (s *VKService) ImportUserOAuthToken(ctx context.Context, userID uuid.UUID, accessToken, platform string) (*models.VKUserConnection, error) {
	if strings.TrimSpace(accessToken) == "" {
		return nil, fmt.Errorf("invalid_request: access_token is empty")
	}
	me, err := vkapi.NewUserClient(accessToken).UsersGetMe(ctx)
	if err != nil {
		return nil, fmt.Errorf("invalid_token: %w", err)
	}
	return s.saveUserConnection(ctx, userID, me.ID, platform, accessToken)
}

func (s *VKService) saveUserConnection(ctx context.Context, userID uuid.UUID, vkUserID int64, platform, accessToken string) (*models.VKUserConnection, error) {
	if vkUserID == 0 {
		return nil, fmt.Errorf("vk_user_id_missing: failed to resolve current VK user")
	}

	tokenEnc, tokenIV, err := s.encryptToken([]byte(accessToken))
	if err != nil {
		return nil, err
	}

	conn := &models.VKUserConnection{
		ID:             uuid.New(),
		UserID:         userID,
		VKUserID:       vkUserID,
		Platform:       platform,
		AccessTokenEnc: tokenEnc,
		AccessTokenIV:  tokenIV,
		Scope:          "groups,wall,offline",
		ConnectedAt:    time.Now(),
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO vk_user_connections
			(id, user_id, vk_user_id, platform,
			 access_token_enc, access_token_iv, scope)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (user_id) DO UPDATE SET
			vk_user_id       = EXCLUDED.vk_user_id,
			platform         = EXCLUDED.platform,
			access_token_enc = EXCLUDED.access_token_enc,
			access_token_iv  = EXCLUDED.access_token_iv,
			scope            = EXCLUDED.scope,
			updated_at       = now()`,
		conn.ID, conn.UserID, conn.VKUserID, conn.Platform,
		conn.AccessTokenEnc, conn.AccessTokenIV, conn.Scope,
	)
	if err != nil {
		return nil, fmt.Errorf("save_user_connection: %w", err)
	}

	return conn, nil
}

// ─── Group OAuth flow ─────────────────────────────────────────────────────────
// Step 2: connect a specific admin group and get the community token.

// OAuthStart generates a VK authorisation URL that includes group_ids so VK
// issues a community token for that specific group.
// platform: "web" | "android" | "ios"
func (s *VKService) OAuthStart(ctx context.Context, userID uuid.UUID, groupID int64, platform string) (string, string, error) {
	state := s.newState()

	pc := s.communityAccessPlatform(platform)
	if pc.AppID == "" {
		return "", "", fmt.Errorf("VK community app ID not configured for platform %q", platform)
	}

	s.oauthMu.Lock()
	s.oauthStates[state] = pendingOAuth{
		userID:   userID,
		groupID:  groupID,
		platform: platform,
		expiry:   time.Now().Add(10 * time.Minute),
	}
	s.oauthMu.Unlock()

	authURL := s.buildCommunityOAuthURL(pc, groupID, state)
	log.Printf("vk: group oauth start user=%s group=%d platform=%s app_id=%s redirect_uri=%s auth_url=%s", userID, groupID, platform, pc.AppID, pc.RedirectURI, authURL)
	return authURL, state, nil
}

func (s *VKService) CommunityAccessStart(ctx context.Context, userID, integrationID uuid.UUID, platform string) (string, string, int64, error) {
	var groupID int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT group_id FROM vk_integrations WHERE id=$1 AND user_id=$2 AND is_active=TRUE`,
		integrationID, userID,
	).Scan(&groupID); err != nil {
		return "", "", 0, fmt.Errorf("not_found: integration not found")
	}

	pc := s.communityAccessPlatform(platform)
	if pc.AppID == "" {
		return "", "", 0, fmt.Errorf("VK community app ID not configured")
	}

	state := s.newState()
	s.oauthMu.Lock()
	s.oauthStates[state] = pendingOAuth{
		userID:   userID,
		groupID:  groupID,
		platform: platform,
		expiry:   time.Now().Add(10 * time.Minute),
	}
	s.oauthMu.Unlock()

	authURL := s.buildCommunityOAuthURL(pc, groupID, state)
	log.Printf("vk: community access start user=%s integration=%s group=%d platform=%s app_id=%s redirect_uri=%s", userID, integrationID, groupID, platform, pc.AppID, pc.RedirectURI)
	return authURL, state, groupID, nil
}

// SaveCommunityToken stores a manually created VK community token for an
// already connected integration and starts Long Poll if the token is valid.
func (s *VKService) SaveCommunityToken(ctx context.Context, userID, integrationID uuid.UUID, communityToken string) (*models.VKIntegration, error) {
	integ, _, err := s.loadIntegration(ctx, userID, integrationID)
	if err != nil {
		return nil, err
	}

	token := strings.TrimSpace(communityToken)
	if token == "" {
		return nil, fmt.Errorf("invalid_request: community token is required")
	}

	client := vkapi.NewClient(token, integ.GroupID)
	if _, err := client.GroupsGetLongPollServer(ctx, integ.GroupID); err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "longpoll for this group is not enabled") {
			return nil, fmt.Errorf("invalid_group_token: token does not match this group or lacks required rights")
		}
	}

	tokenEnc, tokenIV, err := s.encryptToken([]byte(token))
	if err != nil {
		return nil, err
	}

	integ.GroupTokenEnc = tokenEnc
	integ.GroupTokenIV = tokenIV
	integ, err = s.saveIntegration(ctx, integ)
	if err != nil {
		return nil, err
	}
	integ.CommunityAccessEnabled = true

	go s.startWorker(context.Background(), integ, token)
	return integ, nil
}

func (s *VKService) connectGroupWithUserToken(ctx context.Context, userID uuid.UUID, groupID int64) (*models.VKIntegration, error) {
	userToken, err := s.loadUserToken(ctx, userID)
	if err != nil {
		return nil, err
	}

	group, err := vkapi.NewUserClient(userToken).GroupsGetByID(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("fetch_group: %w", err)
	}
	if group.ID == 0 {
		return nil, fmt.Errorf("fetch_group: empty group response")
	}
	if group.IsAdmin == 0 && group.AdminLevel == 0 {
		log.Printf("vk: direct connect group lookup returned no admin flags user=%s group=%d name=%q screen_name=%q", userID, groupID, group.Name, group.ScreenName)
	}

	integ := &models.VKIntegration{
		ID:              uuid.New(),
		UserID:          userID,
		GroupID:         groupID,
		GroupName:       group.Name,
		GroupScreenName: group.ScreenName,
		GroupPhoto:      group.Photo200,
		IsActive:        true,
		ConnectedAt:     time.Now(),
	}
	return s.saveIntegration(ctx, integ)
}

func (s *VKService) ResolveGroupRef(ctx context.Context, userID uuid.UUID, groupRef string) (int64, error) {
	ref := strings.TrimSpace(strings.ToLower(groupRef))
	ref = strings.TrimPrefix(ref, "https://")
	ref = strings.TrimPrefix(ref, "http://")
	ref = strings.TrimPrefix(ref, "vk.com/")
	ref = strings.TrimPrefix(ref, "/")
	if ref == "" {
		return 0, fmt.Errorf("invalid_group_id: group_ref is empty")
	}
	if strings.HasPrefix(ref, "club") {
		ref = strings.TrimPrefix(ref, "club")
	}
	if strings.HasPrefix(ref, "public") {
		ref = strings.TrimPrefix(ref, "public")
	}
	if gid, err := strconv.ParseInt(ref, 10, 64); err == nil && gid > 0 {
		return gid, nil
	}

	userToken, err := s.loadUserToken(ctx, userID)
	if err != nil {
		return 0, err
	}
	group, err := vkapi.NewUserClient(userToken).GroupsGetByRef(ctx, ref)
	if err != nil {
		return 0, fmt.Errorf("not_found: could not resolve group_ref %q", groupRef)
	}
	return group.ID, nil
}

// OAuthExchange is called by the mobile app after intercepting the deep-link.
func (s *VKService) OAuthExchange(ctx context.Context, code, state, accessToken string) (*models.VKIntegration, error) {
	pending, ok := s.popState(state)
	if !ok {
		return nil, fmt.Errorf("invalid_state: OAuth state not found or expired")
	}
	if accessToken != "" {
		return s.finishGroupOAuthImplicit(ctx, accessToken, pending)
	}
	return s.finishGroupOAuth(ctx, code, pending)
}

// OAuthCallback is called by the server-side web callback for group OAuth.
func (s *VKService) OAuthCallback(ctx context.Context, code, state string) (*models.VKIntegration, string, error) {
	pending, ok := s.popState(state)
	if !ok {
		return nil, "", fmt.Errorf("invalid_state: OAuth state not found or expired")
	}
	integ, err := s.finishGroupOAuth(ctx, code, pending)
	return integ, pending.platform, err
}

func (s *VKService) finishGroupOAuth(ctx context.Context, code string, pending pendingOAuth) (*models.VKIntegration, error) {
	pc := s.communityAccessPlatform(pending.platform)

	tokenResp, err := s.exchangeCode(ctx, code, pc)
	if err != nil {
		return nil, fmt.Errorf("token_exchange: %w", err)
	}

	groupToken, err := extractGroupToken(tokenResp, pending.groupID)
	if err != nil {
		return nil, err
	}

	// Fetch group metadata using the user's own token (if present) or the group token.
	userToken := tokenResp["access_token"]
	if userToken == "" {
		userToken = groupToken
	}
	userClient := vkapi.NewUserClient(userToken)
	group, err := userClient.GroupsGetByID(ctx, pending.groupID)
	if err != nil {
		return nil, fmt.Errorf("fetch_group: %w", err)
	}

	tokenEnc, tokenIV, err := s.encryptToken([]byte(groupToken))
	if err != nil {
		return nil, err
	}

	integ := &models.VKIntegration{
		ID:              uuid.New(),
		UserID:          pending.userID,
		GroupID:         pending.groupID,
		GroupTokenEnc:   tokenEnc,
		GroupTokenIV:    tokenIV,
		GroupName:       group.Name,
		GroupScreenName: group.ScreenName,
		GroupPhoto:      group.Photo200,
		IsActive:        true,
		ConnectedAt:     time.Now(),
	}

	integ, err = s.saveIntegration(ctx, integ)
	if err != nil {
		return nil, err
	}

	go s.startWorker(context.Background(), integ, groupToken)

	return integ, nil
}

func (s *VKService) finishGroupOAuthImplicit(ctx context.Context, groupToken string, pending pendingOAuth) (*models.VKIntegration, error) {
	groupToken = strings.TrimSpace(groupToken)
	if groupToken == "" {
		return nil, fmt.Errorf("group_token_missing: access_token is empty")
	}

	userToken, err := s.loadUserToken(ctx, pending.userID)
	if err != nil {
		userToken = groupToken
	}

	group, err := vkapi.NewUserClient(userToken).GroupsGetByID(ctx, pending.groupID)
	if err != nil && userToken != groupToken {
		group, err = vkapi.NewUserClient(groupToken).GroupsGetByID(ctx, pending.groupID)
	}
	if err != nil {
		return nil, fmt.Errorf("fetch_group: %w", err)
	}

	tokenEnc, tokenIV, err := s.encryptToken([]byte(groupToken))
	if err != nil {
		return nil, err
	}

	integ := &models.VKIntegration{
		ID:              uuid.New(),
		UserID:          pending.userID,
		GroupID:         pending.groupID,
		GroupTokenEnc:   tokenEnc,
		GroupTokenIV:    tokenIV,
		GroupName:       group.Name,
		GroupScreenName: group.ScreenName,
		GroupPhoto:      group.Photo200,
		IsActive:        true,
		ConnectedAt:     time.Now(),
	}

	integ, err = s.saveIntegration(ctx, integ)
	if err != nil {
		return nil, err
	}

	go s.startWorker(context.Background(), integ, groupToken)
	return integ, nil
}

func (s *VKService) communityAccessPlatform(platform string) config.VKPlatformConfig {
	switch strings.ToLower(platform) {
	case "android", "ios":
		return s.cfg.VKLegacyMobilePlatform(platform)
	default:
		return s.cfg.VKCommunityPlatform()
	}
}

func (s *VKService) saveIntegration(ctx context.Context, integ *models.VKIntegration) (*models.VKIntegration, error) {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO vk_integrations
			(id, user_id, group_id, group_token_enc, group_token_iv,
			 group_name, group_screen_name, group_photo, is_active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,TRUE)
		ON CONFLICT (user_id, group_id) DO UPDATE SET
			group_token_enc   = COALESCE(EXCLUDED.group_token_enc, vk_integrations.group_token_enc),
			group_token_iv    = COALESCE(EXCLUDED.group_token_iv, vk_integrations.group_token_iv),
			group_name        = EXCLUDED.group_name,
			group_screen_name = EXCLUDED.group_screen_name,
			group_photo       = EXCLUDED.group_photo,
			is_active         = TRUE`,
		integ.ID, integ.UserID, integ.GroupID,
		integ.GroupTokenEnc, integ.GroupTokenIV,
		integ.GroupName, integ.GroupScreenName, integ.GroupPhoto,
	)
	if err != nil {
		return nil, fmt.Errorf("save_integration: %w", err)
	}

	// Re-fetch to get the actual UUID (ON CONFLICT may have kept the old ID).
	if err := s.db.QueryRowContext(ctx,
		`SELECT id FROM vk_integrations WHERE user_id=$1 AND group_id=$2`,
		integ.UserID, integ.GroupID,
	).Scan(&integ.ID); err != nil {
		return nil, fmt.Errorf("save_integration: %w", err)
	}
	return integ, nil
}

func (s *VKService) buildCommunityOAuthURL(pc config.VKPlatformConfig, groupID int64, state string) string {
	return fmt.Sprintf(
		"%s/authorize?client_id=%s&display=%s&redirect_uri=%s"+
			"&scope=messages,wall,photos,offline&response_type=code&v=%s"+
			"&group_ids=%d&state=%s",
		vkapi.OAuthBase,
		url.QueryEscape(pc.AppID),
		s.oauthDisplay(""),
		url.QueryEscape(pc.RedirectURI),
		vkapi.APIVersion,
		groupID,
		state,
	)
}

func (s *VKService) oauthDisplay(platform string) string {
	// Keep oauth.vk.com browser flows aligned with the last known working
	// mobile behavior. Historically VK returned the callback correctly when
	// Android/iOS also used display=page here.
	return "page"
}

func (s *VKService) oauthDisplayForRedirect(redirectURI string) string {
	if isVKNativeRedirectURI(redirectURI) {
		return "mobile"
	}
	return "page"
}

func isMobileRedirectURI(redirectURI string) bool {
	return strings.HasPrefix(redirectURI, "vk") || strings.HasPrefix(redirectURI, "inbetwin://")
}

func isVKNativeRedirectURI(redirectURI string) bool {
	return strings.HasPrefix(redirectURI, "vk")
}

// ─── Data collection: user-level ─────────────────────────────────────────────

// GetAdminGroups returns the VK groups where the authenticated user is an admin.
// IsConnected is set to true when the group is already connected in inBeTwin.
func (s *VKService) GetAdminGroups(ctx context.Context, userID uuid.UUID) ([]models.VKAdminGroup, error) {
	userToken, err := s.loadUserToken(ctx, userID)
	if err != nil {
		return nil, err
	}

	client := vkapi.NewUserClient(userToken)
	groups, err := client.GroupsGetAdmin(ctx)
	if err != nil {
		return nil, fmt.Errorf("groups.get: %w", err)
	}

	// Build a set of already-connected group IDs for this user.
	connected := make(map[int64]bool)
	rows, err := s.db.QueryContext(ctx,
		`SELECT group_id FROM vk_integrations WHERE user_id=$1 AND is_active=TRUE`, userID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var gid int64
			if rows.Scan(&gid) == nil {
				connected[gid] = true
			}
		}
	}

	result := make([]models.VKAdminGroup, 0, len(groups))
	for _, g := range groups {
		result = append(result, models.VKAdminGroup{
			GroupID:      g.ID,
			Name:         g.Name,
			ScreenName:   g.ScreenName,
			Photo:        g.Photo200,
			MembersCount: g.MembersCount,
			IsConnected:  connected[g.ID],
		})
	}
	return result, nil
}

// GetUserProfile fetches and returns the current user's own VK profile.
func (s *VKService) GetUserProfile(ctx context.Context, userID uuid.UUID) (*models.VKUserProfile, error) {
	userToken, err := s.loadUserToken(ctx, userID)
	if err != nil {
		return nil, err
	}

	client := vkapi.NewUserClient(userToken)
	u, err := client.UsersGetMe(ctx)
	if err != nil {
		return nil, fmt.Errorf("users.get: %w", err)
	}

	profile := &models.VKUserProfile{
		VKUserID:       u.ID,
		FirstName:      u.FirstName,
		LastName:       u.LastName,
		Sex:            u.Sex,
		BDate:          u.BDate,
		About:          u.About,
		Status:         u.Status,
		Domain:         u.Domain,
		PhotoURL:       u.Photo200,
		FollowersCount: u.FollowersCount,
	}
	if u.City != nil {
		profile.City = u.City.Title
	}
	if u.Country != nil {
		profile.Country = u.Country.Title
	}
	if u.Occupation != nil {
		profile.OccupationType = u.Occupation.Type
		profile.OccupationName = u.Occupation.Name
	}
	return profile, nil
}

// GetUserSubscriptions fetches the groups and pages the user follows.
func (s *VKService) GetUserSubscriptions(ctx context.Context, userID uuid.UUID) (*vkapi.SubscriptionsExtendedResponse, error) {
	userToken, err := s.loadUserToken(ctx, userID)
	if err != nil {
		return nil, err
	}

	client := vkapi.NewUserClient(userToken)
	return client.UsersGetSubscriptions(ctx, 100)
}

// GetUserPosts fetches the user's own wall posts.
func (s *VKService) GetUserPosts(ctx context.Context, userID uuid.UUID, count int) (*vkapi.WallGetResponse, error) {
	userToken, err := s.loadUserToken(ctx, userID)
	if err != nil {
		return nil, err
	}
	if count <= 0 || count > 100 {
		count = 50
	}

	var vkUserID int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT vk_user_id FROM vk_user_connections WHERE user_id=$1`, userID,
	).Scan(&vkUserID); err != nil {
		return nil, fmt.Errorf("not_found: user VK connection not found")
	}

	client := vkapi.NewUserClient(userToken)
	return client.WallGet(ctx, vkUserID, count, 0)
}

// ─── Integration management ───────────────────────────────────────────────────

// ListIntegrations returns all active VK integrations for a user.
func (s *VKService) ListIntegrations(ctx context.Context, userID uuid.UUID) ([]*models.VKIntegration, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			i.id, i.user_id, i.group_id, i.group_name, i.group_screen_name, i.group_photo,
			COALESCE(octet_length(i.group_token_enc), 0) AS token_len,
			i.is_active, i.connected_at,
			(SELECT COUNT(*) FROM vk_posts p WHERE p.integration_id=i.id) AS posts_count,
			(SELECT COUNT(*) FROM vk_messages m WHERE m.integration_id=i.id) AS message_count,
			(SELECT COUNT(*) FROM vk_lead_profiles l WHERE l.integration_id=i.id) AS lead_count
		FROM vk_integrations i
		WHERE i.user_id = $1 AND i.is_active = TRUE
		ORDER BY i.connected_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*models.VKIntegration
	for rows.Next() {
		i := &models.VKIntegration{}
		var groupName, screenName, photo sql.NullString
		var tokenLen int
		if err := rows.Scan(&i.ID, &i.UserID, &i.GroupID,
			&groupName, &screenName, &photo, &tokenLen,
			&i.IsActive, &i.ConnectedAt, &i.PostsCount, &i.MessageCount, &i.LeadCount); err != nil {
			return nil, err
		}
		i.GroupName = groupName.String
		i.GroupScreenName = screenName.String
		i.GroupPhoto = photo.String
		i.CommunityAccessEnabled = tokenLen > 0
		i.ContextReady = i.PostsCount > 0 || i.MessageCount > 0
		list = append(list, i)
	}
	return list, rows.Err()
}

// Disconnect deactivates a VK integration and stops its Long Poll worker.
func (s *VKService) Disconnect(ctx context.Context, userID uuid.UUID, integrationID uuid.UUID) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE vk_integrations SET is_active = FALSE
		WHERE id = $1 AND user_id = $2`, integrationID, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("not_found: integration not found or unauthorized")
	}
	s.stopWorker(integrationID)
	return nil
}

// ─── Wall post sync ───────────────────────────────────────────────────────────

// SyncPosts imports wall posts from the VK group's wall.
func (s *VKService) SyncPosts(ctx context.Context, userID uuid.UUID, integrationID uuid.UUID, count int) (int, error) {
	if count <= 0 || count > 100 {
		count = 50
	}

	integ, _, err := s.loadIntegration(ctx, userID, integrationID)
	if err != nil {
		return 0, err
	}

	userToken, err := s.loadUserToken(ctx, userID)
	if err != nil {
		return 0, err
	}
	client := vkapi.NewUserClient(userToken)
	ownerID := -integ.GroupID

	result, err := client.WallGet(ctx, ownerID, count, 0)
	if err != nil {
		return 0, fmt.Errorf("wall.get: %w", err)
	}

	imported := 0
	for _, p := range result.Items {
		post := buildVKPost(p, integrationID, userID)
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO vk_posts
				(id, integration_id, user_id, vk_post_id, owner_id, text,
				 media_type, has_media, likes_count, reposts_count,
				 views_count, comments_count, extracted_urls, posted_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
			ON CONFLICT (integration_id, vk_post_id) DO NOTHING`,
			post.ID, post.IntegrationID, post.UserID, post.VKPostID, post.OwnerID,
			post.Text, post.MediaType, post.HasMedia,
			post.LikesCount, post.RepostsCount, post.ViewsCount, post.CommentsCount,
			pq.Array(post.ExtractedURLs), post.PostedAt,
		)
		if err != nil {
			log.Printf("[vk] sync: insert post %d: %v", p.ID, err)
			continue
		}
		imported++
	}
	return imported, nil
}

// ─── Messages ─────────────────────────────────────────────────────────────────

// SendMessage sends a text message to a lead from the VK group.
func (s *VKService) SendMessage(ctx context.Context, userID uuid.UUID, integrationID uuid.UUID, toVKUserID int64, text string) error {
	_, err := s.sendMessageInternal(ctx, userID, integrationID, toVKUserID, text)
	return err
}

func (s *VKService) sendMessageInternal(ctx context.Context, userID uuid.UUID, integrationID uuid.UUID, toVKUserID int64, text string) (int64, error) {
	integ, groupToken, err := s.loadIntegration(ctx, userID, integrationID)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(groupToken) == "" {
		return 0, fmt.Errorf("community_oauth_required: для сообщений сначала включите доступ сообщества")
	}

	humanDelayVK(ctx)

	client := vkapi.NewClient(groupToken, integ.GroupID)
	msgID, err := client.MessagesSend(ctx, toVKUserID, text)
	if err != nil {
		return 0, fmt.Errorf("messages.send: %w", err)
	}

	msgIDVal := msgID
	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO vk_messages
			(integration_id, from_vk_user_id, message_id, text, is_incoming, is_processed)
		VALUES ($1,$2,$3,$4,FALSE,TRUE)
		ON CONFLICT DO NOTHING`,
		integrationID, toVKUserID, msgIDVal, text,
	)
	return msgIDVal, nil
}

// ─── Lead enrichment ──────────────────────────────────────────────────────────

// EnrichLead fetches the VK user's public profile and persists it as a digital twin.
func (s *VKService) EnrichLead(ctx context.Context, userID uuid.UUID, integrationID uuid.UUID, vkUserID int64) (*models.VKLeadProfile, error) {
	integ, groupToken, err := s.loadIntegration(ctx, userID, integrationID)
	if err != nil {
		return nil, err
	}
	_ = integ
	if strings.TrimSpace(groupToken) == "" {
		return nil, fmt.Errorf("community_oauth_required: для лидов сначала включите доступ сообщества")
	}

	client := vkapi.NewClient(groupToken, 0)
	users, err := client.UsersGet(ctx, []int64{vkUserID})
	if err != nil || len(users) == 0 {
		return nil, fmt.Errorf("users.get: %w", err)
	}
	u := users[0]

	profile := &models.VKLeadProfile{
		ID:             uuid.New(),
		IntegrationID:  integrationID,
		VKUserID:       vkUserID,
		FirstName:      u.FirstName,
		LastName:       u.LastName,
		Sex:            u.Sex,
		BDate:          u.BDate,
		Domain:         u.Domain,
		About:          u.About,
		Status:         u.Status,
		PhotoURL:       u.Photo200,
		FollowersCount: u.FollowersCount,
	}
	if u.City != nil {
		profile.City = u.City.Title
	}
	if u.Country != nil {
		profile.Country = u.Country.Title
	}
	if u.Occupation != nil {
		profile.OccupationType = u.Occupation.Type
		profile.OccupationName = u.Occupation.Name
	}
	now := time.Now()
	profile.LastEnrichedAt = &now

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO vk_lead_profiles
			(id, integration_id, vk_user_id, first_name, last_name, sex,
			 bdate, city, country, about, status, domain, photo_url,
			 followers_count, occupation_type, occupation_name, last_enriched_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		ON CONFLICT (integration_id, vk_user_id) DO UPDATE SET
			first_name       = EXCLUDED.first_name,
			last_name        = EXCLUDED.last_name,
			sex              = EXCLUDED.sex,
			bdate            = EXCLUDED.bdate,
			city             = EXCLUDED.city,
			country          = EXCLUDED.country,
			about            = EXCLUDED.about,
			status           = EXCLUDED.status,
			domain           = EXCLUDED.domain,
			photo_url        = EXCLUDED.photo_url,
			followers_count  = EXCLUDED.followers_count,
			occupation_type  = EXCLUDED.occupation_type,
			occupation_name  = EXCLUDED.occupation_name,
			last_enriched_at = EXCLUDED.last_enriched_at`,
		profile.ID, profile.IntegrationID, profile.VKUserID,
		profile.FirstName, profile.LastName, profile.Sex,
		nullStr(profile.BDate), nullStr(profile.City), nullStr(profile.Country),
		nullStr(profile.About), nullStr(profile.Status), nullStr(profile.Domain),
		nullStr(profile.PhotoURL), profile.FollowersCount,
		nullStr(profile.OccupationType), nullStr(profile.OccupationName),
		profile.LastEnrichedAt,
	)
	return profile, err
}

// ─── Long Poll worker management ──────────────────────────────────────────────

// StartAllWorkers loads all active VK integrations and starts a Long Poll worker
// for each. Called once on service startup.
func (s *VKService) StartAllWorkers(ctx context.Context) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, group_id, group_token_enc, group_token_iv,
		       group_name, group_screen_name, group_photo, long_poll_ts
		FROM vk_integrations WHERE is_active = TRUE`)
	if err != nil {
		log.Printf("[vk] StartAllWorkers: query: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		i := &models.VKIntegration{}
		var groupName, screenName, groupPhoto, longPollTS sql.NullString
		if err := rows.Scan(
			&i.ID, &i.UserID, &i.GroupID,
			&i.GroupTokenEnc, &i.GroupTokenIV,
			&groupName, &screenName, &groupPhoto,
			&longPollTS,
		); err != nil {
			log.Printf("[vk] StartAllWorkers: scan: %v", err)
			continue
		}
		i.GroupName = groupName.String
		i.GroupScreenName = screenName.String
		i.GroupPhoto = groupPhoto.String
		i.LongPollTs = longPollTS.String

		groupToken, err := s.decryptToken(i.GroupTokenEnc, i.GroupTokenIV)
		if err != nil {
			log.Printf("[vk] StartAllWorkers: decrypt token for %s: %v", i.ID, err)
			continue
		}
		if strings.TrimSpace(groupToken) == "" {
			continue
		}
		go s.startWorker(ctx, i, groupToken)
	}
}

func (s *VKService) startWorker(ctx context.Context, integ *models.VKIntegration, groupToken string) {
	workerCtx, cancel := context.WithCancel(ctx)

	s.workerMu.Lock()
	if old, ok := s.workerCancels[integ.ID]; ok {
		old()
	}
	s.workerCancels[integ.ID] = cancel
	s.workerMu.Unlock()

	client := vkapi.NewClient(groupToken, integ.GroupID)
	integID := integ.ID
	initialTs := integ.LongPollTs

	worker := vkapi.NewWorker(
		client,
		integ.GroupID,
		s.handleIncomingMessage,
		func() string { return initialTs },
		func(ts string) { s.persistLPTs(integID, ts) },
	)
	worker.Run(workerCtx)
}

func (s *VKService) stopWorker(integrationID uuid.UUID) {
	s.workerMu.Lock()
	defer s.workerMu.Unlock()
	if cancel, ok := s.workerCancels[integrationID]; ok {
		cancel()
		delete(s.workerCancels, integrationID)
	}
}

// handleIncomingMessage is called by the Long Poll worker on each message_new event.
func (s *VKService) handleIncomingMessage(ctx context.Context, groupID int64, msg vkapi.IncomingMessage) {
	var integID, ownerUserID uuid.UUID
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id FROM vk_integrations WHERE group_id=$1 AND is_active=TRUE LIMIT 1`,
		groupID,
	).Scan(&integID, &ownerUserID)
	if err != nil {
		log.Printf("[vk-lp] group %d: integration not found: %v", groupID, err)
		return
	}

	text := &msg.Text
	if msg.Text == "" {
		text = nil
	}
	convMsgID := msg.ConversationMessageID

	var inboundID uuid.UUID
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO vk_messages
			(integration_id, from_vk_user_id, message_id,
			 conversation_message_id, text, is_incoming, is_processed, received_at)
		VALUES ($1,$2,$3,$4,$5,TRUE,FALSE,$6)
		ON CONFLICT (integration_id, message_id) DO NOTHING
		RETURNING id`,
		integID, msg.FromID, msg.ID,
		nullInt64(convMsgID), text, time.Unix(msg.Date, 0).UTC(),
	).Scan(&inboundID)
	if err != nil {
		if err == sql.ErrNoRows {
			return
		}
		log.Printf("[vk-lp] group %d: save message: %v", groupID, err)
		return
	}

	go func() {
		enCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var exists bool
		_ = s.db.QueryRowContext(enCtx,
			`SELECT EXISTS(SELECT 1 FROM vk_lead_profiles WHERE integration_id=$1 AND vk_user_id=$2)`,
			integID, msg.FromID,
		).Scan(&exists)

		if !exists {
			if _, err := s.EnrichLead(enCtx, ownerUserID, integID, msg.FromID); err != nil {
				log.Printf("[vk-lp] enrich lead %d: %v", msg.FromID, err)
			}
		}
		s.processInboundMessage(enCtx, ownerUserID, integID, inboundID)
	}()
}

func (s *VKService) persistLPTs(integID uuid.UUID, ts string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = s.db.ExecContext(ctx,
		`UPDATE vk_integrations SET long_poll_ts=$1 WHERE id=$2`, ts, integID)
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

// loadUserToken retrieves and decrypts the user-level VK access token.
func (s *VKService) loadUserToken(ctx context.Context, userID uuid.UUID) (string, error) {
	var enc, iv []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT access_token_enc, access_token_iv FROM vk_user_connections WHERE user_id=$1`,
		userID,
	).Scan(&enc, &iv)
	if err != nil {
		return "", fmt.Errorf("not_found: VK user connection not found — complete user OAuth first")
	}
	return s.decryptToken(enc, iv)
}

// loadIntegration fetches the integration and decrypts its group token.
func (s *VKService) loadIntegration(ctx context.Context, userID, integrationID uuid.UUID) (*models.VKIntegration, string, error) {
	i := &models.VKIntegration{}
	var groupName, screenName, longPollTS sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, group_id, group_token_enc, group_token_iv,
		       group_name, group_screen_name, long_poll_ts
		FROM vk_integrations
		WHERE id=$1 AND user_id=$2 AND is_active=TRUE`,
		integrationID, userID,
	).Scan(&i.ID, &i.UserID, &i.GroupID,
		&i.GroupTokenEnc, &i.GroupTokenIV,
		&groupName, &screenName, &longPollTS)
	if err != nil {
		return nil, "", fmt.Errorf("not_found: %w", err)
	}
	i.GroupName = groupName.String
	i.GroupScreenName = screenName.String
	i.LongPollTs = longPollTS.String
	if len(i.GroupTokenEnc) == 0 {
		return i, "", nil
	}
	token, err := s.decryptToken(i.GroupTokenEnc, i.GroupTokenIV)
	return i, token, err
}

// exchangeCodePKCE exchanges a VK ID authorization code using PKCE.
// Uses id.vk.ru/oauth2/auth — no client_secret needed, code_verifier replaces it.
// deviceID is the VK-specific value returned alongside the code in the redirect.
func (s *VKService) exchangeCodePKCE(ctx context.Context, code, deviceID, codeVerifier string, pc config.VKPlatformConfig) (map[string]string, error) {
	params := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {pc.AppID},
		"code":          {code},
		"code_verifier": {codeVerifier},
		"redirect_uri":  {pc.RedirectURI},
		"state":         {""},
	}
	if deviceID != "" {
		params.Set("device_id", deviceID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		vkapi.VKIDTokenURL,
		strings.NewReader(params.Encode()),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]json.RawMessage
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	if errField, ok := result["error"]; ok {
		var errMsg string
		_ = json.Unmarshal(errField, &errMsg)
		desc := ""
		if d, ok := result["error_description"]; ok {
			_ = json.Unmarshal(d, &desc)
		}
		return nil, fmt.Errorf("vk id oauth error: %s — %s", errMsg, desc)
	}

	flat := make(map[string]string)
	for k, v := range result {
		var sv string
		if json.Unmarshal(v, &sv) == nil {
			flat[k] = sv
		} else {
			flat[k] = string(v)
		}
	}
	return flat, nil
}

// exchangeCode calls the legacy VK OAuth token endpoint (oauth.vk.com).
// Used for group/community tokens only.
func (s *VKService) exchangeCode(ctx context.Context, code string, pc config.VKPlatformConfig) (map[string]string, error) {
	params := url.Values{
		"client_id":     {pc.AppID},
		"client_secret": {pc.AppSecret},
		"redirect_uri":  {pc.RedirectURI},
		"code":          {code},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		vkapi.OAuthBase+"/access_token",
		strings.NewReader(params.Encode()),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]json.RawMessage
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	if errField, ok := result["error"]; ok {
		var errMsg string
		_ = json.Unmarshal(errField, &errMsg)
		return nil, fmt.Errorf("vk oauth error: %s", errMsg)
	}

	flat := make(map[string]string)
	for k, v := range result {
		var sv string
		if json.Unmarshal(v, &sv) == nil {
			flat[k] = sv
		} else {
			flat[k] = string(v)
		}
	}
	return flat, nil
}

// extractGroupToken finds the group access token in the OAuth response map.
func extractGroupToken(resp map[string]string, groupID int64) (string, error) {
	key := fmt.Sprintf("access_token_%d", groupID)
	if t, ok := resp[key]; ok && t != "" {
		return t, nil
	}
	if t, ok := resp["access_token"]; ok && t != "" {
		return t, nil
	}
	return "", fmt.Errorf("group_token_missing: no token found for group %d", groupID)
}

func (s *VKService) encryptToken(plain []byte) (enc, iv []byte, err error) {
	if s.enc != nil {
		enc, iv, err = s.enc.Encrypt(plain)
		if err != nil {
			return nil, nil, fmt.Errorf("encrypt_token: %w", err)
		}
		return enc, iv, nil
	}
	// Dev mode — no encryption.
	return plain, []byte("DEV_NO_IV_TWELVE!"), nil
}

func (s *VKService) decryptToken(enc, iv []byte) (string, error) {
	if len(enc) == 0 {
		return "", nil
	}
	if s.enc == nil {
		return string(enc), nil
	}
	plain, err := s.enc.Decrypt(enc, iv)
	if err != nil {
		return "", fmt.Errorf("decrypt_token: %w", err)
	}
	return string(plain), nil
}

func (s *VKService) newState() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("vk: generate state: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func (s *VKService) peekState(state string) (pendingOAuth, bool) {
	s.oauthMu.Lock()
	defer s.oauthMu.Unlock()
	pending, ok := s.oauthStates[state]
	if !ok || pending.expiry.Before(time.Now()) {
		return pendingOAuth{}, false
	}
	return pending, true
}

func (s *VKService) popState(state string) (pendingOAuth, bool) {
	s.oauthMu.Lock()
	defer s.oauthMu.Unlock()
	pending, ok := s.oauthStates[state]
	if !ok || pending.expiry.Before(time.Now()) {
		delete(s.oauthStates, state)
		return pendingOAuth{}, false
	}
	delete(s.oauthStates, state)
	return pending, true
}

// buildVKPost converts a vkapi.WallPost to models.VKPost.
func buildVKPost(p vkapi.WallPost, integrationID, userID uuid.UUID) *models.VKPost {
	post := &models.VKPost{
		ID:            uuid.New(),
		IntegrationID: integrationID,
		UserID:        userID,
		VKPostID:      p.ID,
		OwnerID:       p.OwnerID,
		PostedAt:      time.Unix(p.Date, 0).UTC(),
	}
	if p.Text != "" {
		post.Text = &p.Text
	}
	if len(p.Attachments) > 0 {
		post.HasMedia = true
		t := p.Attachments[0].Type
		post.MediaType = &t
	}
	likes := p.Likes.Count
	reposts := p.Reposts.Count
	views := p.Views.Count
	comments := p.Comments.Count
	post.LikesCount = &likes
	post.RepostsCount = &reposts
	if views > 0 {
		post.ViewsCount = &views
	}
	post.CommentsCount = &comments
	return post
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullInt64(n int64) interface{} {
	if n == 0 {
		return nil
	}
	return n
}

func humanDelayVK(ctx context.Context) {
	jitter := time.Duration(1000+time.Now().UnixNano()%2000) * time.Millisecond
	select {
	case <-time.After(jitter):
	case <-ctx.Done():
	}
}
