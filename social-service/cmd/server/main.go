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

	vkSvc := services.NewVKService(db, enc, cfg)
	go vkSvc.StartAllWorkers(context.Background())

	vkHandler := handlers.NewVKHandler(vkSvc, cfg.FrontendURL)

	if cfg.TelegramBotToken != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := telegramSvc.Init(ctx); err != nil {
			log.Printf("WARNING: telegram init failed: %v", err)
		}
		cancel()
	} else {
		log.Println("WARNING: TELEGRAM_BOT_TOKEN not set — Telegram integration disabled")
	}

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

	// Gateway strips /api/v1 before proxying — social-service sees paths without that prefix.
	app.Post("/social/telegram/webhook", webhookHandler.Receive)

	// VK web OAuth callbacks — browser is redirected here by VK after authorisation.
	// No JWT: user is identified via the state nonce stored in VKService.
	app.Get("/social/vk/oauth/user/callback", vkHandler.UserOAuthCallback)
	app.Get("/social/vk/oauth/callback", vkHandler.OAuthCallback)

	// ── Protected routes (JWT required) ─────────────────────────────────────
	social := app.Group("/social", jwtlib.AuthMiddleware(jwtService))

	tg := social.Group("/telegram")
	tg.Post("/connect", telegramHandler.Connect)
	tg.Delete("/disconnect", telegramHandler.Disconnect)
	tg.Get("/status", telegramHandler.Status)
	tg.Get("/posts",
		middleware.RequireConsent(db, "telegram"),
		telegramHandler.Posts,
	)

	tgAuth := tg.Group("/auth")
	tgAuth.Post("/send-code", mtprotoHandler.SendCode)
	tgAuth.Post("/sign-in", mtprotoHandler.SignIn)
	tgAuth.Delete("/sign-out", mtprotoHandler.SignOut)

	tg.Get("/channels", mtprotoHandler.GetChannels)
	tg.Post("/sync", mtprotoHandler.Sync)
	tg.Post("/message", mtprotoHandler.SendMessage)
	tg.Post("/reply", mtprotoHandler.ReplyToComment)

	// ── VK routes ────────────────────────────────────────────────────────────
	vk := social.Group("/vk")

	// Step 1 — user OAuth (list admin groups, fetch profile/subscriptions)
	vk.Get("/oauth/user/start", vkHandler.UserOAuthStart)
	vk.Post("/oauth/user/exchange", vkHandler.UserOAuthExchange) // mobile only

	// Step 2 — group OAuth (community token)
	vk.Get("/oauth/start", vkHandler.OAuthStart)
	vk.Post("/oauth/exchange", vkHandler.OAuthExchange) // mobile only

	// User-level data
	vk.Get("/groups/admin", vkHandler.GetAdminGroups)
	vk.Get("/user/profile", vkHandler.GetUserProfile)
	vk.Get("/user/subscriptions", vkHandler.GetUserSubscriptions)
	vk.Get("/user/posts", vkHandler.GetUserPosts)

	// Connected groups management
	vk.Get("/groups", vkHandler.ListIntegrations)
	vk.Delete("/disconnect", vkHandler.Disconnect)
	vk.Post("/sync", vkHandler.SyncPosts)
	vk.Post("/message", vkHandler.SendMessage)
	vk.Get("/lead/:vk_user_id", vkHandler.EnrichLead)

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
