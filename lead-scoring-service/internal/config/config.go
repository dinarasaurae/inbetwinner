package config

import (
	"log"
	"os"
	"strconv"

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

	HotLeadThreshold int
	NotificationsURL string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file, using env vars")
	}
	threshold, _ := strconv.Atoi(getEnv("HOT_LEAD_THRESHOLD", "80"))
	return &Config{
		Port:             getEnv("PORT", "3006"),
		Environment:      getEnv("ENVIRONMENT", "development"),
		DBHost:           getEnv("DB_HOST", "localhost"),
		DBPort:           getEnv("DB_PORT", "5432"),
		DBUser:           getEnv("DB_USER", "leads_user"),
		DBPassword:       getEnv("DB_PASSWORD", "leads_password"),
		DBName:           getEnv("DB_NAME", "leads_db"),
		DBSSLMode:        getEnv("DB_SSL_MODE", "disable"),
		HotLeadThreshold: threshold,
		NotificationsURL: getEnv("NOTIFICATIONS_URL", ""),
	}
}

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
