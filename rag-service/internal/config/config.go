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

	OpenAIAPIKey         string
	OpenAIEmbeddingModel string
	OpenAIEmbeddingDims  int

	PineconeAPIKey    string
	PineconeIndexName string
	PineconeHost      string

	GoogleClientID     string
	GoogleClientSecret string

	// MinIO / S3-compatible object storage (optional).
	// When MinioEndpoint is empty the storage layer is disabled and original
	// files are not persisted — only the extracted text is stored in Postgres.
	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	MinioBucket    string
	MinioUseSSL    bool

	RagTopK     int
	RagMinScore float64
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}
	dims, _ := strconv.Atoi(getEnv("OPENAI_EMBEDDING_DIMS", "1536"))
	topK, _ := strconv.Atoi(getEnv("RAG_TOP_K", "5"))
	minScore, _ := strconv.ParseFloat(getEnv("RAG_MIN_SCORE", "0.70"), 64)
	minioSSL, _ := strconv.ParseBool(getEnv("MINIO_USE_SSL", "false"))
	return &Config{
		Port:        getEnv("PORT", "3004"),
		Environment: getEnv("ENVIRONMENT", "development"),
		DBHost:      getEnv("DB_HOST", "localhost"),
		DBPort:      getEnv("DB_PORT", "5432"),
		DBUser:      getEnv("DB_USER", "rag_user"),
		DBPassword:  getEnv("DB_PASSWORD", "rag_password"),
		DBName:      getEnv("DB_NAME", "rag_db"),
		DBSSLMode:   getEnv("DB_SSL_MODE", "disable"),

		OpenAIAPIKey:         getEnv("OPENAI_API_KEY", ""),
		OpenAIEmbeddingModel: getEnv("OPENAI_EMBEDDING_MODEL", "text-embedding-3-small"),
		OpenAIEmbeddingDims:  dims,

		PineconeAPIKey:    getEnv("PINECONE_API_KEY", ""),
		PineconeIndexName: getEnv("PINECONE_INDEX_NAME", "inbetwin"),
		PineconeHost:      getEnv("PINECONE_HOST", ""),

		GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),

		MinioEndpoint:  getEnv("MINIO_ENDPOINT", ""),
		MinioAccessKey: getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinioSecretKey: getEnv("MINIO_SECRET_KEY", "minioadmin"),
		MinioBucket:    getEnv("MINIO_BUCKET", "inbetwin-rag"),
		MinioUseSSL:    minioSSL,

		RagTopK:     topK,
		RagMinScore: minScore,
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
