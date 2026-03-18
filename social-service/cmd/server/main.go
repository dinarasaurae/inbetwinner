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
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"

	jwtlib "github.com/dinarasaurae/inbetwin-shared/jwt-go"

	"github.com/dinarasaurae/inbetwin-social-service/internal/config"
	"github.com/dinarasaurae/inbetwin-social-service/internal/crypto"
	"github.com/dinarasaurae/inbetwin-social-service/internal/database"
	"github.com/dinarasaurae/inbetwin-social-service/internal/handlers"
	"github.com/dinarasaurae/inbetwin-social-service/internal/middleware"
	"github.com/dinarasaurae/inbetwin-social-service/internal/mtproto"
	"github.com/dinarasaurae/inbetwin-social-service/internal/services"
)

func main() {
	cfg := config.Load()

	db, err := database.NewPostgresConnection(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.RunMigrations(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Set up AES-256-GCM encryptor. Nil is tolerated in dev (warnings logged in config).
	var enc *crypto.Encryptor
	if len(cfg.EncryptionKey) == 32 {
		enc, err = crypto.NewEncryptor(cfg.EncryptionKey)
		if err != nil {
			log.Fatalf("Failed to create encryptor: %v", err)
		}
	}

	jwtService := jwtlib.NewService(cfg.JWTSecret, cfg.JWTAccessExpiration, cfg.JWTRefreshExpiration)

	telegramSvc := services.NewTelegramService(db, enc, cfg)
	webhookSvc := services.NewWebhookService(db)

	// Init fetches bot info and optionally registers the webhook URL with Telegram.
	if cfg.TelegramBotToken != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := telegramSvc.Init(ctx); err != nil {
			log.Printf("WARNING: telegram init failed: %v", err)
		}
		cancel()
	} else {
		log.Println("WARNING: TELEGRAM_BOT_TOKEN not set — Telegram integration disabled")
	}

	// MTProto (userbot) service — acts as the authenticated user, not as a bot.
	mtprotoSvc := mtproto.NewService(db, enc, cfg)
	mtprotoHandler := handlers.NewMtprotoHandler(mtprotoSvc)

	healthHandler := handlers.NewHealthHandler(db)
	telegramHandler := handlers.NewTelegramHandler(telegramSvc)
	webhookHandler := handlers.NewWebhookHandler(webhookSvc, cfg.TelegramWebhookSecret)

	app := fiber.New(fiber.Config{
		AppName:      "inBeTwin Social Service",
		ServerHeader: "inBeTwin-Social",
		ErrorHandler: func(c fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			var e *fiber.Error
			if errors.As(err, &e) {
				code = e.Code
			}
			log.Printf("Error: %v", err)
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
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

	app.Get("/health", healthHandler.Check)

	api := app.Group("/api/v1")

	// Public webhook route — no JWT, verified by X-Telegram-Bot-Api-Secret-Token header.
	api.Post("/social/telegram/webhook", webhookHandler.Receive)

	// All other /social routes require a valid JWT.
	social := api.Group("/social", jwtlib.AuthMiddleware(jwtService))

	tg := social.Group("/telegram")
	tg.Post("/connect", telegramHandler.Connect)
	tg.Delete("/disconnect", telegramHandler.Disconnect)
	tg.Get("/status", telegramHandler.Status)

	// /posts additionally requires an active consent record.
	tg.Get("/posts",
		middleware.RequireConsent(db, "telegram"),
		telegramHandler.Posts,
	)

	// ── MTProto (userbot) routes ────────────────────────────────────────────
	// These act as the authenticated user themselves — not as a bot.

	// Phone-number auth flow (two-step: send-code → sign-in).
	tgAuth := tg.Group("/auth")
	tgAuth.Post("/send-code", mtprotoHandler.SendCode)
	tgAuth.Post("/sign-in", mtprotoHandler.SignIn)
	tgAuth.Delete("/sign-out", mtprotoHandler.SignOut)

	// Channel management & history.
	tg.Get("/channels", mtprotoHandler.GetChannels)
	tg.Post("/sync", mtprotoHandler.Sync)

	// Posting as the user.
	tg.Post("/message", mtprotoHandler.SendMessage)
	tg.Post("/reply", mtprotoHandler.ReplyToComment)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		addr := fmt.Sprintf(":%s", cfg.Port)
		log.Printf("Social Service starting on http://localhost%s", addr)
		log.Printf("Environment: %s", cfg.Environment)
		if err := app.Listen(addr); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down gracefully")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Social Service stopped")
}
