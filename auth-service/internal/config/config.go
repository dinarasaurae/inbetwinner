package config

import (
	"log"
	"os"
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

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string

	YandexClientID     string
	YandexClientSecret string
	YandexRedirectURL  string

	VKAndroidClientID string

	FrontendURL string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	jwtAccessExpStr := getEnv("JWT_ACCESS_EXPIRATION", "15m")
	jwtAccessExp, err := time.ParseDuration(jwtAccessExpStr)
	if err != nil {
		log.Printf("Invalid JWT_ACCESS_EXPIRATION format: %s, using 15m", jwtAccessExpStr)
		jwtAccessExp = 15 * time.Minute
	}

	jwtRefreshExpStr := getEnv("JWT_REFRESH_EXPIRATION", "7d")
	jwtRefreshExp, err := time.ParseDuration(jwtRefreshExpStr)
	if err != nil {
		log.Printf("Invalid JWT_REFRESH_EXPIRATION format: %s, using 7d", jwtRefreshExpStr)
		jwtRefreshExp = 7 * 24 * time.Hour
	}

	return &Config{
		Port:        getEnv("PORT", "3001"),
		Environment: getEnv("ENVIRONMENT", "development"),

		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "auth_user"),
		DBPassword: getEnv("DB_PASSWORD", "auth_password"),
		DBName:     getEnv("DB_NAME", "auth_db"),
		DBSSLMode:  getEnv("DB_SSL_MODE", "disable"),

		JWTSecret:            getEnv("JWT_SECRET", "change-me-in-production"),
		JWTAccessExpiration:  jwtAccessExp,
		JWTRefreshExpiration: jwtRefreshExp,

		GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURL:  getEnv("GOOGLE_REDIRECT_URL", "http://localhost:3001/api/v1/auth/oauth/google/callback"),

		YandexClientID:     getEnv("YANDEX_CLIENT_ID", ""),
		YandexClientSecret: getEnv("YANDEX_CLIENT_SECRET", ""),
		YandexRedirectURL:  getEnv("YANDEX_REDIRECT_URL", "http://localhost:3001/api/v1/auth/oauth/yandex/callback"),

		VKAndroidClientID: getEnv("VK_APP_ID_ANDROID", "54511649"),

		FrontendURL: getEnv("FRONTEND_URL", "http://localhost:3000"),
	}
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func (c *Config) GetDatabaseURL() string {
	return "postgres://" + c.DBUser + ":" + c.DBPassword +
		"@" + c.DBHost + ":" + c.DBPort +
		"/" + c.DBName + "?sslmode=" + c.DBSSLMode
}

func (c *Config) IsDevelopment() bool {
	return c.Environment == "development"
}
