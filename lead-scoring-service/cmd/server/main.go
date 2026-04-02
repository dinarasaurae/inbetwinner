package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/dinarasaurae/inbetwin-lead-scoring-service/internal/config"
	"github.com/dinarasaurae/inbetwin-lead-scoring-service/internal/database"
	"github.com/dinarasaurae/inbetwin-lead-scoring-service/internal/handlers"
	"github.com/dinarasaurae/inbetwin-lead-scoring-service/internal/middleware"
	"github.com/dinarasaurae/inbetwin-lead-scoring-service/internal/services"
)

func main() {
	cfg := config.Load()

	db, err := database.NewPostgresConnection(cfg)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	if err := db.RunMigrations(); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	scorer := services.NewScorerService(db, cfg.HotLeadThreshold, cfg.NotificationsURL)
	scoringHandler := handlers.NewScoringHandler(scorer)

	app := fiber.New(fiber.Config{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	})

	app.Get("/health", handlers.HealthCheck)

	api := app.Group("/scoring", middleware.WorkspaceAuth)
	api.Post("/score", scoringHandler.Score)
	api.Get("/leads", scoringHandler.ListScores)
	api.Get("/leads/:leadID", scoringHandler.GetScore)
	api.Get("/leads/:leadID/signals", scoringHandler.GetSignals)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := app.Listen(":" + cfg.Port); err != nil {
			log.Printf("server stopped: %v", err)
		}
	}()

	log.Printf("lead-scoring-service started on :%s", cfg.Port)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Println("lead-scoring-service stopped")
}
