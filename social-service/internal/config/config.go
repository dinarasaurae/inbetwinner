package config

import (
	"encoding/hex"
	"log"
	"os"
	"strconv"
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
	VKWebAppID      string
	VKWebAppSecret  string
	VKWebRedirectURI string // https://yourdomain.com/api/v1/social/vk/oauth/user/callback

	// Android app (ID 54511649)
	VKAndroidAppID     string
	VKAndroidAppSecret string

	// iOS app (ID 54511650)
	VKIOSAppID     string
	VKIOSAppSecret string

	// Mobile deep-link used by both Android and iOS
	VKMobileRedirectURI string // inbetwin://vk-callback

	// Frontend URL — browser is redirected here after web OAuth completes
	FrontendURL string
}

// VKPlatform returns the VK credentials for the given platform ("web", "android", "ios").
// Falls back to web config for unknown platforms.
func (c *Config) VKPlatform(platform string) VKPlatformConfig {
	switch platform {
	case "android":
		return VKPlatformConfig{
			AppID:       c.VKAndroidAppID,
			AppSecret:   c.VKAndroidAppSecret,
			RedirectURI: c.VKMobileRedirectURI,
		}
	case "ios":
		return VKPlatformConfig{
			AppID:       c.VKIOSAppID,
			AppSecret:   c.VKIOSAppSecret,
			RedirectURI: c.VKMobileRedirectURI,
		}
	default: // "web"
		return VKPlatformConfig{
			AppID:       c.VKWebAppID,
			AppSecret:   c.VKWebAppSecret,
			RedirectURI: c.VKWebRedirectURI,
		}
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

		VKWebAppID:       getEnv("VK_APP_ID_WEB", "54511648"),
		VKWebAppSecret:   getEnv("VK_APP_SECRET_WEB", ""),
		VKWebRedirectURI: getEnv("VK_REDIRECT_URI_WEB", "http://localhost:3002/api/v1/social/vk/oauth/user/callback"),

		VKAndroidAppID:     getEnv("VK_APP_ID_ANDROID", "54511649"),
		VKAndroidAppSecret: getEnv("VK_APP_SECRET_ANDROID", ""),

		VKIOSAppID:     getEnv("VK_APP_ID_IOS", "54511650"),
		VKIOSAppSecret: getEnv("VK_APP_SECRET_IOS", ""),

		VKMobileRedirectURI: getEnv("VK_REDIRECT_URI_MOBILE", "inbetwin://vk-callback"),

		FrontendURL: getEnv("FRONTEND_URL", "http://localhost:3000"),
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
