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

	"github.com/dinarasaurae/inbetwin-agent-service/internal/config"
	"github.com/dinarasaurae/inbetwin-agent-service/internal/database"
	"github.com/dinarasaurae/inbetwin-agent-service/internal/handlers"
	"github.com/dinarasaurae/inbetwin-agent-service/internal/middleware"
	"github.com/dinarasaurae/inbetwin-agent-service/internal/services"
)

func main() {
	cfg := config.Load()

	db, err := database.NewPostgresConnection(cfg)
	if err != nil {
		log.Fatalf("DB: %v", err)
	}
	defer db.Close()
	if err := db.RunMigrations(); err != nil {
		log.Fatalf("Migrations: %v", err)
	}

	agentSvc := services.NewAgentService(db)
	agentH := handlers.NewAgentHandler(agentSvc)

	app := fiber.New(fiber.Config{
		AppName:      "inBeTwin Agent Service",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	})
	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format:     "[${time}] ${status} - ${method} ${path}\n",
		TimeFormat: "2006-01-02 15:04:05",
	}))
	app.Use(cors.New(cors.Config{AllowOrigins: []string{"*"}}))

	app.Get("/health", handlers.HealthCheck)

	agent := app.Group("/agent", middleware.WorkspaceAuth())
	agent.Get("/agents", agentH.List)
	agent.Post("/agents", agentH.Create)
	agent.Get("/agents/:id", agentH.Get)
	agent.Put("/agents/:id", agentH.Update)
	agent.Delete("/agents/:id", agentH.Delete)
	agent.Get("/agents/:id/tools", agentH.GetTools)
	agent.Post("/agents/:id/tools", agentH.AssignTool)
	agent.Delete("/agents/:id/tools/:toolId", agentH.UnassignTool)
	agent.Get("/builtin-tools", handlers.ListBuiltinTools)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		addr := fmt.Sprintf(":%s", cfg.Port)
		log.Printf("agent-service listening on %s", addr)
		if err := app.Listen(addr); err != nil {
			log.Fatalf("listen: %v", err)
		}
	}()
	<-quit
	log.Println("Shutting down agent-service...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = app.ShutdownWithContext(ctx)
	log.Println("agent-service stopped")
}
