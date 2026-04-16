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

	// MinIO / S3 storage — optional
	ctx := context.Background()
	storageSvc, err := services.NewStorageService(
		cfg.MinioEndpoint, cfg.MinioAccessKey, cfg.MinioSecretKey,
		cfg.MinioBucket, cfg.MinioUseSSL,
	)
	if err != nil {
		log.Printf("MinIO init warning: %v (continuing without file storage)", err)
		storageSvc = nil
	}
	if storageSvc != nil {
		if err := storageSvc.EnsureBucket(ctx); err != nil {
			log.Printf("MinIO bucket ensure warning: %v", err)
		} else {
			log.Printf("MinIO connected (bucket: %s)", cfg.MinioBucket)
		}
	}

	// Google OAuth (optional — requires GOOGLE_CLIENT_ID + GOOGLE_CLIENT_SECRET)
	googleRedirectURI := cfg.GoogleRedirectURI
	if googleRedirectURI == "" {
		googleRedirectURI = cfg.PublicBaseURL + "/rag/google/oauth/callback"
	}
	googleOAuthSvc := services.NewGoogleOAuthService(db, cfg.GoogleClientID, cfg.GoogleClientSecret, googleRedirectURI)
	if googleOAuthSvc.IsConfigured() {
		log.Printf("Google OAuth configured (redirect: %s)", googleRedirectURI)
	}

	nsSvc := services.NewNamespaceService(db, pcSvc)
	docSvc := services.NewDocumentService(db, embSvc, pcSvc, nsSvc, storageSvc, cfg.OpenAIEmbeddingModel)
	qaSvc := services.NewQAService(db, embSvc, pcSvc, nsSvc)
	searchSvc := services.NewSearchService(db, embSvc, pcSvc, nsSvc)
	sheetsSvc := services.NewSheetsService(db, embSvc, pcSvc, nsSvc, googleOAuthSvc)
	webSvc := services.NewWebService(db, embSvc, pcSvc, nsSvc)

	nsH := handlers.NewNamespaceHandler(nsSvc)
	docH := handlers.NewDocumentHandler(docSvc)
	qaH := handlers.NewQAHandler(qaSvc)
	searchH := handlers.NewSearchHandler(searchSvc)
	sheetsH := handlers.NewSheetsHandler(sheetsSvc)
	webH := handlers.NewWebHandler(webSvc)
	googleOAuthH := handlers.NewGoogleOAuthHandler(googleOAuthSvc, googleRedirectURI)

	app := fiber.New(fiber.Config{
		AppName:      "inBeTwin RAG Service",
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 30 * time.Second,
		BodyLimit:    52 << 20,
	})
	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format:     "[${time}] ${status} - ${method} ${path}\n",
		TimeFormat: "2006-01-02 15:04:05",
	}))
	app.Use(cors.New(cors.Config{AllowOrigins: []string{"*"}}))

	app.Get("/health", handlers.HealthCheck)

	// Google OAuth callback — NOT behind workspace auth (browser redirect)
	app.Get("/rag/google/oauth/callback", googleOAuthH.Callback)

	rag := app.Group("/rag", middleware.WorkspaceAuth())

	// ── Namespaces ────────────────────────────────────────────────────────────
	rag.Post("/namespaces", nsH.Create)
	rag.Get("/namespaces", nsH.List)
	rag.Delete("/namespaces/:id", nsH.Delete)

	// ── Documents (text ingestion + file upload) ──────────────────────────────
	rag.Post("/documents", docH.Create)
	rag.Post("/documents/upload", docH.Upload)
	rag.Get("/documents", docH.List)
	rag.Delete("/documents/:id", docH.Delete)
	rag.Post("/documents/:id/reindex", docH.Reindex)

	// ── QA pairs ─────────────────────────────────────────────────────────────
	rag.Post("/qa", qaH.Create)
	rag.Get("/qa", qaH.List)
	rag.Delete("/qa/:id", qaH.Delete)

	// ── Google Sheets ─────────────────────────────────────────────────────────
	rag.Post("/sheets/sync", sheetsH.Sync)
	rag.Get("/sheets", sheetsH.List)
	rag.Delete("/sheets/:id", sheetsH.Delete)

	// ── Web / URL sources ─────────────────────────────────────────────────────
	rag.Post("/web", webH.Add)
	rag.Get("/web", webH.List)
	rag.Delete("/web/:id", webH.Delete)

	// ── Google OAuth (requires workspace auth for start/status/disconnect) ────
	rag.Get("/google/oauth/start", googleOAuthH.Start)
	rag.Get("/google/oauth/status", googleOAuthH.Status)
	rag.Delete("/google/oauth", googleOAuthH.Disconnect)

	// ── Semantic search ───────────────────────────────────────────────────────
	rag.Post("/search", searchH.Search)

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
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(shutCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("rag-service stopped")
}
