package main

import (
	"context"
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

	"github.com/dinarasaurae/inbetwin-llm-service/internal/config"
	"github.com/dinarasaurae/inbetwin-llm-service/internal/database"
	"github.com/dinarasaurae/inbetwin-llm-service/internal/handlers"
	"github.com/dinarasaurae/inbetwin-llm-service/internal/middleware"
	"github.com/dinarasaurae/inbetwin-llm-service/internal/services"
	agentsvc "github.com/dinarasaurae/inbetwin-llm-service/internal/services/agent"
	amocrmSvc "github.com/dinarasaurae/inbetwin-llm-service/internal/services/amocrm"
	calendarsvc "github.com/dinarasaurae/inbetwin-llm-service/internal/services/calendar"
	zohosvc "github.com/dinarasaurae/inbetwin-llm-service/internal/services/zoho"
	histsvc "github.com/dinarasaurae/inbetwin-llm-service/internal/services/history"
	"github.com/dinarasaurae/inbetwin-llm-service/internal/services/tools"
)

func main() {
	cfg := config.Load()

	db, err := database.NewPostgresConnection(cfg)
	if err != nil {
		log.Fatalf("DB connect: %v", err)
	}
	defer db.Close()
	if err := db.RunMigrations(); err != nil {
		log.Fatalf("Migrations: %v", err)
	}

	histService := histsvc.NewService(db)
	calService := calendarsvc.NewService(db, cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleRedirectURL)
	amoCRMService := amocrmSvc.NewService(db, cfg.AmoCRMClientID, cfg.AmoCRMClientSecret, cfg.AmoCRMRedirectURL)
	zohoService := zohosvc.NewService(db, cfg.ZohoClientID, cfg.ZohoClientSecret, cfg.ZohoRedirectURL)
	registry := tools.NewRegistry(db)
	builtinH := tools.NewBuiltinHandler(calService, amoCRMService, zohoService, db)
	dispatcher := tools.NewDispatcher(db, builtinH, registry)
	agentClient := agentsvc.NewClient(cfg.AgentServiceURL)
	llmService := services.NewLLMService(cfg, histService, registry, dispatcher, agentClient)
	toolService := services.NewToolService(db)

	chatH := handlers.NewChatHandler(llmService, histService)
	toolH := handlers.NewToolHandler(toolService)
	calH := handlers.NewCalendarHandler(calService)
	amoCRMH := handlers.NewAmoCRMHandler(amoCRMService)
	zohoH := handlers.NewZohoHandler(zohoService)
	socialH := handlers.NewSocialHandler(llmService)

	app := fiber.New(fiber.Config{
		AppName:      "inBeTwin LLM Service",
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
	})
	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format:     "[${time}] ${status} - ${method} ${path}\n",
		TimeFormat: "2006-01-02 15:04:05",
	}))
	app.Use(cors.New(cors.Config{AllowOrigins: []string{"*"}}))

	app.Get("/health", handlers.HealthCheck)
	// Google OAuth callback is public — Google redirects the browser here
	app.Get("/llm/google/callback", calH.Callback)
	// amoCRM OAuth callback is public — amoCRM redirects the browser here
	app.Get("/llm/amocrm/callback", amoCRMH.Callback)
	// Zoho CRM OAuth callback is public — Zoho redirects the browser here
	app.Get("/llm/zoho/callback", zohoH.Callback)

	llm := app.Group("/llm", middleware.WorkspaceAuth())
	llm.Post("/chat", chatH.Chat)
	llm.Get("/history/:chat_user_id", chatH.GetHistory)
	llm.Delete("/history/:chat_user_id", chatH.ClearHistory)
	llm.Get("/tools", toolH.List)
	llm.Post("/tools", toolH.Create)
	llm.Put("/tools/:id", toolH.Update)
	llm.Delete("/tools/:id", toolH.Delete)
	llm.Get("/tools/:name/executions", toolH.GetExecutions)
	llm.Get("/google/auth-url", calH.GetAuthURL)
	llm.Get("/google/status", calH.GetStatus)
	llm.Get("/google/events", calH.ListEvents)
	llm.Post("/google/events", calH.CreateEvent)
	llm.Get("/amocrm/auth-url", amoCRMH.GetAuthURL)
	llm.Get("/amocrm/status", amoCRMH.GetStatus)
	llm.Get("/amocrm/pipelines", amoCRMH.GetPipelines)
	llm.Get("/zoho/auth-url", zohoH.GetAuthURL)
	llm.Get("/zoho/status", zohoH.GetStatus)
	llm.Get("/zoho/deal-stages", zohoH.GetDealStages)
	// Social-service orchestration endpoint — not a generic chat endpoint.
	llm.Post("/social/vk/process", socialH.ProcessVK)

	// ── Startup diagnostics ──────────────────────────────────────────────────
	{
		provider := cfg.LLMProvider
		if provider == "" {
			provider = "openai"
		}
		baseURL := cfg.LLMBaseURL
		if baseURL == "" {
			switch provider {
			case "groq":
				baseURL = "https://api.groq.com/openai/v1"
			case "openrouter":
				baseURL = "https://openrouter.ai/api/v1"
			case "huggingface":
				baseURL = "https://router.huggingface.co/v1"
			default:
				baseURL = "https://api.openai.com/v1"
			}
		}
		keyHint := "(not set)"
		if cfg.LLMAPIKey != "" {
			n := len(cfg.LLMAPIKey)
			if n > 8 {
				keyHint = cfg.LLMAPIKey[:4] + "…" + cfg.LLMAPIKey[n-4:]
			} else {
				keyHint = "(set)"
			}
		}
		log.Printf("llm-service provider : %s", provider)
		log.Printf("llm-service base URL : %s", baseURL)
		log.Printf("llm-service model    : %s", cfg.LLMModel)
		log.Printf("llm-service api key  : %s", keyHint)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		addr := fmt.Sprintf(":%s", cfg.Port)
		log.Printf("llm-service listening on %s", addr)
		if err := app.Listen(addr); err != nil {
			log.Fatalf("listen: %v", err)
		}
	}()
	<-quit
	log.Println("Shutting down llm-service...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = app.ShutdownWithContext(ctx)
	log.Println("llm-service stopped")
}
