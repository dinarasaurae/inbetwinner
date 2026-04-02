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

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	OpenAIAPIKey    string
	OpenAIModel     string
	OpenAITemp      float32
	OpenAIMaxTokens int

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string

	RAGServiceURL   string
	AgentServiceURL string

	MaxToolIterations int
	HistorySize       int

	RequestTimeout time.Duration
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file, using env vars")
	}
	temp, _ := strconv.ParseFloat(getEnv("OPENAI_TEMPERATURE", "0.7"), 32)
	maxTok, _ := strconv.Atoi(getEnv("OPENAI_MAX_TOKENS", "2000"))
	maxIter, _ := strconv.Atoi(getEnv("MAX_TOOL_ITERATIONS", "5"))
	histSize, _ := strconv.Atoi(getEnv("HISTORY_SIZE", "20"))
	return &Config{
		Port:        getEnv("PORT", "3005"),
		Environment: getEnv("ENVIRONMENT", "development"),
		DBHost:      getEnv("DB_HOST", "localhost"),
		DBPort:      getEnv("DB_PORT", "5432"),
		DBUser:      getEnv("DB_USER", "llm_user"),
		DBPassword:  getEnv("DB_PASSWORD", "llm_password"),
		DBName:      getEnv("DB_NAME", "llm_db"),
		DBSSLMode:   getEnv("DB_SSL_MODE", "disable"),

		OpenAIAPIKey:    getEnv("OPENAI_API_KEY", ""),
		OpenAIModel:     getEnv("OPENAI_MODEL", "gpt-4o-mini"),
		OpenAITemp:      float32(temp),
		OpenAIMaxTokens: maxTok,

		GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURL:  getEnv("GOOGLE_REDIRECT_URL", "http://localhost:3005/llm/google/callback"),

		RAGServiceURL:   getEnv("RAG_SERVICE_URL", "http://rag-service:3004"),
		AgentServiceURL: getEnv("AGENT_SERVICE_URL", "http://agent-service:3003"),

		MaxToolIterations: maxIter,
		HistorySize:       histSize,
		RequestTimeout:    30 * time.Second,
	}
}

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
