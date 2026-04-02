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

	"github.com/dinarasaurae/inbetwin-rag-service/internal/config"
	"github.com/dinarasaurae/inbetwin-rag-service/internal/database"
	"github.com/dinarasaurae/inbetwin-rag-service/internal/handlers"
	"github.com/dinarasaurae/inbetwin-rag-service/internal/middleware"
	"github.com/dinarasaurae/inbetwin-rag-service/internal/services"
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

	embSvc := services.NewEmbeddingService(cfg.OpenAIAPIKey, cfg.OpenAIEmbeddingModel)

	var pcSvc *services.PineconeService
	if cfg.PineconeAPIKey != "" && cfg.PineconeHost != "" {
		pcSvc, err = services.NewPineconeService(cfg.PineconeAPIKey, cfg.PineconeHost, cfg.PineconeIndexName)
		if err != nil {
			log.Printf("Pinecone init warning: %v (continuing without vector search)", err)
		}
	}

	nsSvc := services.NewNamespaceService(db, pcSvc)
	docSvc := services.NewDocumentService(db, embSvc, pcSvc, nsSvc, cfg.OpenAIEmbeddingModel)
	qaSvc := services.NewQAService(db, embSvc, pcSvc, nsSvc)
	searchSvc := services.NewSearchService(db, embSvc, pcSvc, nsSvc)
	sheetsSvc := services.NewSheetsService(db, embSvc, pcSvc, nsSvc)

	nsH := handlers.NewNamespaceHandler(nsSvc)
	docH := handlers.NewDocumentHandler(docSvc)
	qaH := handlers.NewQAHandler(qaSvc)
	searchH := handlers.NewSearchHandler(searchSvc)
	sheetsH := handlers.NewSheetsHandler(sheetsSvc)

	app := fiber.New(fiber.Config{
		AppName:      "inBeTwin RAG Service",
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

	rag := app.Group("/rag", middleware.WorkspaceAuth())
	rag.Post("/namespaces", nsH.Create)
	rag.Get("/namespaces", nsH.List)
	rag.Delete("/namespaces/:id", nsH.Delete)
	rag.Post("/documents", docH.Create)
	rag.Get("/documents", docH.List)
	rag.Delete("/documents/:id", docH.Delete)
	rag.Post("/qa", qaH.Create)
	rag.Get("/qa", qaH.List)
	rag.Delete("/qa/:id", qaH.Delete)
	rag.Post("/search", searchH.Search)
	rag.Post("/sheets/sync", sheetsH.Sync)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		addr := fmt.Sprintf(":%s", cfg.Port)
		log.Printf("rag-service listening on %s", addr)
		if err := app.Listen(addr); err != nil {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down rag-service...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("rag-service stopped")
}
