package config

import (
	"encoding/hex"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// VKPlatformConfig bundles the credentials and redirect URI for one VK app.
type VKPlatformConfig struct {
	AppID       string
	AppSecret   string
	RedirectURI string
}

type Config struct {
	Port        string
	Environment string

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	JWTSecret            string
	JWTAccessExpiration  time.Duration
	JWTRefreshExpiration time.Duration

	// Bot API (optional — for webhook-based new-post monitoring)
	TelegramBotToken      string
	TelegramWebhookSecret string
	TelegramWebhookURL    string

	// MTProto (primary — acts as the user themselves)
	// Obtain AppID and AppHash from https://my.telegram.org
	TelegramAppID   int
	TelegramAppHash string

	EncryptionKey []byte // 32 bytes decoded from 64-char hex env var

	// ── VK per-platform credentials ───────────────────────────────────────────
	// Register apps at https://vk.com/editapp
	// Web app (ID 54511648)
	VKWebAppID       string
	VKWebAppSecret   string
	VKWebRedirectURI string // https://yourdomain.com/api/v1/social/vk/oauth/user/callback
	// Separate public HTTPS callback for community OAuth via oauth.vk.com.
	VKGroupRedirectURI string // https://yourdomain.com/api/v1/social/vk/oauth/callback
	// Community OAuth scopes requested when issuing a group token.
	// VK only allows a small fixed subset here, so the runtime also sanitizes
	// this list before building the oauth.vk.com URL.
	VKCommunityScopes string

	// Android app (ID 54511649)
	VKAndroidAppID     string
	VKAndroidAppSecret string
	// VK ID SDK redirect: vk{clientId}://vk.ru — registered via vkidManifestPlaceholders in build.gradle
	VKAndroidRedirectURI string

	// iOS app (ID 54511650)
	VKIOSAppID     string
	VKIOSAppSecret string
	// VK ID SDK redirect: vk{clientId}://vk.ru — registered via CFBundleURLSchemes in Info.plist
	VKIOSRedirectURI string

	// Legacy browser OAuth redirect used by the old mobile VK connect flow.
	// This stays separate from VK ID SDK redirects so we can preserve the
	// historically working auto-token path without disturbing app login.
	VKLegacyMobileRedirectURI string
	// Some mobile builds send the VK SDK token directly to social-service via
	// /oauth/user/import. Disabled by default so the app falls back to the
	// historically working browser OAuth flow.
	VKAllowUserTokenImport bool

	// Frontend URL — browser is redirected here after web OAuth completes
	FrontendURL string

	LLMProvider string
	LLMAPIKey   string
	LLMBaseURL  string
	LLMModel    string

	RAGServiceURL  string
	LLMServiceURL  string
	AuthServiceURL string

	// LLMOrchestrationMode controls the inbound-message processing path.
	//   legacy      — use the built-in vk_agent local draft provider
	//   llm_service — route through llm-service orchestrator only
	//   hybrid      — try llm-service first; fall back to legacy on error/timeout
	LLMOrchestrationMode string

	// Pinterest Business API access token — used to enrich leads' public
	// Pinterest profile (board names → interests) for the digital twin.
	// Obtain at: https://developers.pinterest.com/docs/getting-started/authentication/
	PinterestAccessToken string

	// Facebook Page Access Token — used for Graph API Messenger profile lookups.
	// Required only when Facebook Messenger integration is active.
	// Obtain at: https://developers.facebook.com/docs/messenger-platform/
	FacebookPageToken string
}

// VKPlatform returns the VK credentials for the given platform ("web", "android", "ios").
// Falls back to web config for unknown platforms.
func (c *Config) VKPlatform(platform string) VKPlatformConfig {
	switch platform {
	case "android":
		return VKPlatformConfig{
			AppID:       c.VKAndroidAppID,
			AppSecret:   c.VKAndroidAppSecret,
			RedirectURI: c.VKAndroidRedirectURI,
		}
	case "ios":
		return VKPlatformConfig{
			AppID:       c.VKIOSAppID,
			AppSecret:   c.VKIOSAppSecret,
			RedirectURI: c.VKIOSRedirectURI,
		}
	default: // "web"
		return VKPlatformConfig{
			AppID:       c.VKWebAppID,
			AppSecret:   c.VKWebAppSecret,
			RedirectURI: c.VKWebRedirectURI,
		}
	}
}

// VKCommunityPlatform returns the VK app config used for community OAuth.
// Group/community tokens must be issued via oauth.vk.com with a redirect URI
// that is explicitly registered in the app settings, so we always use the
// web app credentials plus the dedicated public callback URL here.
func (c *Config) VKCommunityPlatform() VKPlatformConfig {
	return VKPlatformConfig{
		AppID:       c.VKWebAppID,
		AppSecret:   c.VKWebAppSecret,
		RedirectURI: c.VKGroupRedirectURI,
	}
}

// VKLegacyBrowserUserPlatform returns the browser-based OAuth config used by
// the old VK connect wizard step 1. Even on mobile we route this through the
// web app because current VK console settings expose trusted redirect URLs only
// for the web app, while Android/iOS app configs are limited to package/bundle
// identity for VK ID SDK redirects.
func (c *Config) VKLegacyBrowserUserPlatform() VKPlatformConfig {
	return VKPlatformConfig{
		AppID:       c.VKWebAppID,
		AppSecret:   c.VKWebAppSecret,
		RedirectURI: c.VKWebRedirectURI,
	}
}

// VKLegacyMobilePlatform returns the old mobile browser OAuth config used by
// the pre-workspace VK connect wizard. It keeps the platform-specific app IDs
// but routes the callback back through the app deep link instead of VK ID SDK.
func (c *Config) VKLegacyMobilePlatform(platform string) VKPlatformConfig {
	switch platform {
	case "android":
		return VKPlatformConfig{
			AppID:       c.VKAndroidAppID,
			AppSecret:   c.VKAndroidAppSecret,
			RedirectURI: c.VKLegacyMobileRedirectURI,
		}
	case "ios":
		return VKPlatformConfig{
			AppID:       c.VKIOSAppID,
			AppSecret:   c.VKIOSAppSecret,
			RedirectURI: c.VKLegacyMobileRedirectURI,
		}
	default:
		return c.VKPlatform(platform)
	}
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	jwtAccessExp := parseDuration("JWT_ACCESS_EXPIRATION", "15m", 15*time.Minute)
	jwtRefreshExp := parseDuration("JWT_REFRESH_EXPIRATION", "7d", 7*24*time.Hour)

	encKey := decodeEncryptionKey(getEnv("ENCRYPTION_KEY", ""))

	return &Config{
		Port:        getEnv("PORT", "3002"),
		Environment: getEnv("ENVIRONMENT", "development"),

		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "social_user"),
		DBPassword: getEnv("DB_PASSWORD", "social_password"),
		DBName:     getEnv("DB_NAME", "social_db"),
		DBSSLMode:  getEnv("DB_SSL_MODE", "disable"),

		JWTSecret:            getEnv("JWT_SECRET", "change-me-in-production"),
		JWTAccessExpiration:  jwtAccessExp,
		JWTRefreshExpiration: jwtRefreshExp,

		TelegramBotToken:      getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramWebhookSecret: getEnv("TELEGRAM_WEBHOOK_SECRET", ""),
		TelegramWebhookURL:    getEnv("TELEGRAM_WEBHOOK_URL", ""),
		TelegramAppID:         parseTelegramAppID(getEnv("TELEGRAM_APP_ID", "0")),
		TelegramAppHash:       getEnv("TELEGRAM_APP_HASH", ""),
		EncryptionKey:         encKey,

		VKWebAppID:         getEnv("VK_APP_ID_WEB", "54511648"),
		VKWebAppSecret:     getEnv("VK_APP_SECRET_WEB", ""),
		VKWebRedirectURI:   getEnv("VK_REDIRECT_URI_WEB", "http://localhost:3002/api/v1/social/vk/oauth/user/callback"),
		VKGroupRedirectURI: getEnv("VK_REDIRECT_URI_GROUP", "http://localhost:3002/api/v1/social/vk/oauth/callback"),
		VKCommunityScopes:  getEnv("VK_COMMUNITY_SCOPES", "messages,manage,photos,docs,wall,stories"),

		VKAndroidAppID:     getEnv("VK_APP_ID_ANDROID", "54511649"),
		VKAndroidAppSecret: getEnv("VK_APP_SECRET_ANDROID", ""),
		// VK ID OAuth 2.1 redirect — vk{clientId}://vk.ru/blank.html
		VKAndroidRedirectURI: getEnv("VK_REDIRECT_URI_ANDROID", "vk54511649://vk.ru/blank.html"),

		VKIOSAppID:     getEnv("VK_APP_ID_IOS", "54511650"),
		VKIOSAppSecret: getEnv("VK_APP_SECRET_IOS", ""),
		// VK ID OAuth 2.1 redirect — vk{clientId}://vk.ru/blank.html
		VKIOSRedirectURI:          getEnv("VK_REDIRECT_URI_IOS", "vk54511650://vk.ru/blank.html"),
		VKLegacyMobileRedirectURI: getEnv("VK_REDIRECT_URI_MOBILE", "inbetwin://vk-callback"),
		VKAllowUserTokenImport:    strings.EqualFold(getEnv("VK_ALLOW_USER_TOKEN_IMPORT", "true"), "true"),

		FrontendURL: getEnv("FRONTEND_URL", "http://localhost:3000"),

		LLMProvider: getFirstEnv("openai", "LLM_PROVIDER"),
		LLMAPIKey:   getFirstEnv("", "LLM_API_KEY", "OPENAI_API_KEY"),
		LLMBaseURL:  getFirstEnv("", "LLM_BASE_URL", "OPENAI_BASE_URL"),
		LLMModel:    getFirstEnv("gpt-4o-mini", "LLM_MODEL", "OPENAI_MODEL"),

		RAGServiceURL:        getEnv("RAG_SERVICE_URL", "http://rag-service:3004"),
		LLMServiceURL:        getEnv("LLM_SERVICE_URL", "http://llm-service:3005"),
		AuthServiceURL:       getEnv("AUTH_SERVICE_URL", "http://auth-service:3001"),
		LLMOrchestrationMode: getEnv("LLM_ORCHESTRATION_MODE", "legacy"),
		PinterestAccessToken: getEnv("PINTEREST_ACCESS_TOKEN", ""),
		FacebookPageToken:    getEnv("FACEBOOK_PAGE_TOKEN", ""),
	}
}

func (c *Config) GetDatabaseURL() string {
	return "postgres://" + c.DBUser + ":" + c.DBPassword +
		"@" + c.DBHost + ":" + c.DBPort +
		"/" + c.DBName + "?sslmode=" + c.DBSSLMode
}

func (c *Config) IsDevelopment() bool {
	return c.Environment == "development"
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getFirstEnv(defaultValue string, keys ...string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return defaultValue
}

func parseDuration(envKey, envDefault string, fallback time.Duration) time.Duration {
	s := getEnv(envKey, envDefault)
	d, err := time.ParseDuration(s)
	if err != nil {
		log.Printf("Invalid %s format: %s, using default", envKey, s)
		return fallback
	}
	return d
}

func parseTelegramAppID(s string) int {
	id, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return id
}

func decodeEncryptionKey(hexKey string) []byte {
	if hexKey == "" {
		log.Println("WARNING: ENCRYPTION_KEY not set, token encryption disabled")
		return nil
	}
	key, err := hex.DecodeString(hexKey)
	if err != nil || len(key) != 32 {
		log.Fatalf("ENCRYPTION_KEY must be a 64-character hex string (32 bytes): %v", err)
	}
	return key
}
