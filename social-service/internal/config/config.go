package config

import (
	"encoding/hex"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

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

	// VK integration — register at https://vk.com/editapp
	VKAppID       string
	VKAppSecret   string
	VKRedirectURI string // e.g. "inbetwin://vk-callback" registered in VK app settings
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

		VKAppID:       getEnv("VK_APP_ID", ""),
		VKAppSecret:   getEnv("VK_APP_SECRET", ""),
		VKRedirectURI: getEnv("VK_REDIRECT_URI", "inbetwin://vk-callback"),
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
