package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/redis/go-redis/v9"

	"github.com/dinarasaurae/inbetwin-api-gateway/internal/config"
	"github.com/dinarasaurae/inbetwin-api-gateway/internal/handlers"
	"github.com/dinarasaurae/inbetwin-api-gateway/internal/proxy"
	jwtlib "github.com/dinarasaurae/inbetwin-shared/jwt-go"
)

func main() {
	cfg := config.Load()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("Connected to Redis")

	jwtService := jwtlib.NewService(cfg.JWTSecret, cfg.JWTAccessExpiration, cfg.JWTRefreshExpiration)
	proxyService := proxy.NewService()
	healthHandler := handlers.NewHealthHandler(redisClient)

	app := fiber.New(fiber.Config{
		AppName:      "inBeTwin API Gateway",
		ServerHeader: "inBeTwin-FastHTTP",

		CaseSensitive:     true, // так быстрее роутинг
		StrictRouting:     true, // строгая маршрутизация
		ReduceMemoryUsage: true, // минимизируем аллокации

		ReadBufferSize:  8192,
		WriteBufferSize: 8192,

		Concurrency: 512 * 1024,

		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,

		ErrorHandler: func(c fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			var e *fiber.Error
			if errors.As(err, &e) {
				code = e.Code
			}

			log.Printf("Error: %v", err)

			return c.Status(code).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
	})

	app.Use(recover.New())

	app.Use(logger.New(logger.Config{
		Format:     "[${time}] ${status} - ${method} ${path} (${latency})\n",
		TimeFormat: "2006-01-02 15:04:05",
		TimeZone:   "Local",
	}))

	app.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		AllowCredentials: false,
		MaxAge:           3600,
	}))

	app.Use(limiter.New(limiter.Config{
		Max:        cfg.RateLimitMax,
		Expiration: cfg.RateLimitWindow,

		KeyGenerator: func(c fiber.Ctx) string {
			return c.IP()
		},

		Next: func(c fiber.Ctx) bool {
			// Пропускаем rate limiting для health checks и опций
			return c.Path() == "/health" || c.Method() == "OPTIONS"
		},

		LimitReached: func(c fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":       "rate limit exceeded",
				"retry_after": cfg.RateLimitWindow.Seconds(),
			})
		},
	}))

	app.Get("/health", healthHandler.Check)

	api := app.Group("/api/v1")

	auth := api.Group("/auth")
	auth.All("/*", func(c fiber.Ctx) error {
		return proxyService.ProxyRequest(c, cfg.AuthServiceURL)
	})

	// ── Public routes — no JWT ───────────────────────────────────────────────

	// Telegram webhook — verified by X-Telegram-Bot-Api-Secret-Token header downstream.
	api.Post("/social/telegram/webhook", func(c fiber.Ctx) error {
		return proxyService.ProxyRequest(c, cfg.SocialServiceURL)
	})

	// VK web OAuth callbacks — VK redirects the user's browser here.
	// No JWT: user is identified via the state nonce stored in the social-service.
	api.Get("/social/vk/oauth/user/callback", func(c fiber.Ctx) error {
		return proxyService.ProxyRequest(c, cfg.SocialServiceURL)
	})
	api.Get("/social/vk/oauth/callback", func(c fiber.Ctx) error {
		return proxyService.ProxyRequest(c, cfg.SocialServiceURL)
	})

	// Google OAuth callback — Google redirects the browser here after consent.
	// No JWT: user is identified via the state nonce stored in the rag-service.
	api.Get("/rag/google/oauth/callback", func(c fiber.Ctx) error {
		return proxyService.ProxyRequest(c, cfg.RAGServiceURL)
	})

	// amoCRM OAuth callback — amoCRM redirects the browser here after consent.
	// No JWT: workspace is identified via the OAuth state value.
	api.Get("/llm/amocrm/callback", func(c fiber.Ctx) error {
		return proxyService.ProxyRequest(c, cfg.LLMServiceURL)
	})

	// Zoho CRM OAuth callback — Zoho redirects the browser here after consent.
	// No JWT: workspace is identified via the OAuth state value.
	api.Get("/llm/zoho/callback", func(c fiber.Ctx) error {
		return proxyService.ProxyRequest(c, cfg.LLMServiceURL)
	})

	protected := api.Group("", jwtlib.AuthMiddleware(jwtService))

	social := protected.Group("/social")
	social.All("/*", func(c fiber.Ctx) error {
		return proxyService.ProxyRequest(c, cfg.SocialServiceURL)
	})

	agent := protected.Group("/agent")
	agent.All("/*", func(c fiber.Ctx) error {
		return proxyService.ProxyRequest(c, cfg.AgentServiceURL)
	})

	llm := protected.Group("/llm")
	llm.All("/*", func(c fiber.Ctx) error {
		return proxyService.ProxyRequest(c, cfg.LLMServiceURL)
	})

	rag := protected.Group("/rag")
	rag.All("/*", func(c fiber.Ctx) error {
		return proxyService.ProxyRequest(c, cfg.RAGServiceURL)
	})

	leads := protected.Group("/leads")
	leads.All("/*", func(c fiber.Ctx) error {
		return proxyService.ProxyRequest(c, cfg.LeadScoringServiceURL)
	})

	notifications := protected.Group("/notifications")
	notifications.All("/*", func(c fiber.Ctx) error {
		return proxyService.ProxyRequest(c, cfg.AuthServiceURL)
	})

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		addr := fmt.Sprintf(":%s", cfg.Port)
		log.Printf("Server starting on http://localhost%s", addr)
		log.Printf("Environment: %s", cfg.Environment)

		if err := app.Listen(addr); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down gracefully")

	if err := app.Shutdown(); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	if err := redisClient.Close(); err != nil {
		log.Printf("Error closing Redis: %v", err)
	}

	log.Println("Server stopped")
}
