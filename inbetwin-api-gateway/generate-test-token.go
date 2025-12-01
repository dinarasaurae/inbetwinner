package main

import (
	"fmt"
	"log"
	"time"

	jwtlib "github.com/dinarasaurae/inbetwin-shared/jwt-go"
)

func main() {
	// Используем тот же секрет, что и в .env файле
	secret := "your-super-secret-key-change-this-in-production"
	expiration := 24 * time.Hour

	jwtService := jwtlib.NewService(secret, expiration, 7*24*time.Hour)

	// Генерируем токен для тестового пользователя
	token, err := jwtService.GenerateToken("test-user-123")
	if err != nil {
		log.Fatalf("Failed to generate token: %v", err)
	}

	fmt.Println("🔑 Test JWT Token Generated:")
	fmt.Println("================================")
	fmt.Printf("Token: %s\n", token)
	fmt.Println("================================")
	fmt.Println()
	fmt.Println("📋 Usage:")
	fmt.Printf("curl -H \"Authorization: Bearer %s\" http://localhost:8080/api/v1/social/test\n", token)
	fmt.Println()
	fmt.Println("🕐 Expires in: 24 hours")
	fmt.Printf("👤 User ID: test-user-123\n")
}