package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"

	"github.com/dinarasaurae/inbetwin-social-service/internal/config"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type DB struct {
	*sql.DB
	Config *config.Config
}

func NewPostgresConnection(cfg *config.Config) (*DB, error) {
	dbURL := cfg.GetDatabaseURL()

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	log.Printf("Connected to PostgreSQL database: %s", cfg.DBName)

	return &DB{DB: db, Config: cfg}, nil
}

func (db *DB) RunMigrations() error {
	src, err := iofs.New(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("failed to create migration source: %w", err)
	}

	driver, err := postgres.WithInstance(db.DB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance(
		"iofs",
		src,
		"postgres",
		driver,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	if err == migrate.ErrNoChange {
		log.Println("Database is up to date")
	} else {
		log.Println("Database migrations completed")
	}

	return nil
}

func (db *DB) Close() error {
	log.Println("Closing database connection")
	return db.DB.Close()
}

func (db *DB) HealthCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}
	return nil
}

func (db *DB) GetStats() sql.DBStats {
	return db.DB.Stats()
}
