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
	agentsvc "github.com/dinarasaurae/inbetwin-llm-service/internal/services/agent"
	calendarsvc "github.com/dinarasaurae/inbetwin-llm-service/internal/services/calendar"
	histsvc "github.com/dinarasaurae/inbetwin-llm-service/internal/services/history"
	"github.com/dinarasaurae/inbetwin-llm-service/internal/services"
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
	registry := tools.NewRegistry(db)
	builtinH := tools.NewBuiltinHandler(calService, db)
	dispatcher := tools.NewDispatcher(db, builtinH, registry)
	agentClient := agentsvc.NewClient(cfg.AgentServiceURL)
	llmService := services.NewLLMService(cfg, histService, registry, dispatcher, agentClient)
	toolService := services.NewToolService(db)

	chatH := handlers.NewChatHandler(llmService, histService)
	toolH := handlers.NewToolHandler(toolService)
	calH := handlers.NewCalendarHandler(calService)
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
	// Social-service orchestration endpoint — not a generic chat endpoint.
	llm.Post("/social/vk/process", socialH.ProcessVK)

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
