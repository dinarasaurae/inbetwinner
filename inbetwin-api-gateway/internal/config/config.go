package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port        string
	Environment string

	JWTSecret            string
	JWTAccessExpiration  time.Duration
	JWTRefreshExpiration time.Duration

	AuthServiceURL        string
	SocialServiceURL      string
	AgentServiceURL       string
	LLMServiceURL         string
	RAGServiceURL         string
	LeadScoringServiceURL string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	RateLimitMax    int
	RateLimitWindow time.Duration
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	jwtAccessExpStr := getEnv("JWT_ACCESS_EXPIRATION", "15m")
	jwtAccessExpiration, err := time.ParseDuration(jwtAccessExpStr)
	if err != nil {
		jwtAccessExpiration = 15 * time.Minute
	}

	jwtRefreshExpStr := getEnv("JWT_REFRESH_EXPIRATION", "7d")
	jwtRefreshExpiration, err := time.ParseDuration(jwtRefreshExpStr)
	if err != nil {
		jwtRefreshExpiration = 7 * 24 * time.Hour
	}

	rateLimitWindowStr := getEnv("RATE_LIMIT_WINDOW", "1m")
	rateLimitWindow, err := time.ParseDuration(rateLimitWindowStr)
	if err != nil {
		rateLimitWindow = 1 * time.Minute
	}

	redisDB, err := strconv.Atoi(getEnv("REDIS_DB", "0"))
	if err != nil {
		redisDB = 0
	}

	rateLimitMax, _ := strconv.Atoi(getEnv("RATE_LIMIT_MAX", "100"))

	return &Config{
		Port:        getEnv("PORT", "8080"),
		Environment: getEnv("ENVIRONMENT", "development"),

		JWTSecret:            getEnv("JWT_SECRET", "change-me-in-production"),
		JWTAccessExpiration:  jwtAccessExpiration,
		JWTRefreshExpiration: jwtRefreshExpiration,

		AuthServiceURL:        getEnv("AUTH_SERVICE_URL", "http://localhost:3001"),
		SocialServiceURL:      getEnv("SOCIAL_SERVICE_URL", "http://localhost:3002"),
		AgentServiceURL:       getEnv("AGENT_SERVICE_URL", "http://localhost:3003"),
		LLMServiceURL:         getEnv("LLM_SERVICE_URL", "http://localhost:3004"),
		RAGServiceURL:         getEnv("RAG_SERVICE_URL", "http://localhost:3005"),
		LeadScoringServiceURL: getEnv("LEAD_SCORING_SERVICE_URL", "http://localhost:3006"),

		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       redisDB,

		RateLimitMax:    rateLimitMax,
		RateLimitWindow: rateLimitWindow,
	}
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
