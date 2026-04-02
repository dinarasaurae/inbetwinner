package config

import (
	"log"
	"os"

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

	LLMServiceURL string
	RAGServiceURL string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file, using env vars")
	}
	return &Config{
		Port:          getEnv("PORT", "3005"),
		Environment:   getEnv("ENVIRONMENT", "development"),
		DBHost:        getEnv("DB_HOST", "localhost"),
		DBPort:        getEnv("DB_PORT", "5432"),
		DBUser:        getEnv("DB_USER", "agent_user"),
		DBPassword:    getEnv("DB_PASSWORD", "agent_password"),
		DBName:        getEnv("DB_NAME", "agent_db"),
		DBSSLMode:     getEnv("DB_SSL_MODE", "disable"),
		LLMServiceURL: getEnv("LLM_SERVICE_URL", "http://llm-service:3004"),
		RAGServiceURL: getEnv("RAG_SERVICE_URL", "http://rag-service:3003"),
	}
}

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
