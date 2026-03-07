package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPostgresPool(databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("erreur parsing database URL: %w", err)
	}

	// Configuration du pool optimisée pour un flux important
	config.MaxConns = 50
	config.MinConns = 5
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute
	config.HealthCheckPeriod = 30 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("erreur connexion PostgreSQL: %w", err)
	}

	// Test de connexion
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("erreur ping PostgreSQL: %w", err)
	}

	return pool, nil
}

// RunMigrations exécute les fichiers de migration SQL
func RunMigrations(pool *pgxpool.Pool, migrationsDir string) error {
	ctx := context.Background()

	// Lire et exécuter le fichier de migration
	// En production, utiliser un outil de migration comme golang-migrate
	migrationSQL := `
	-- Vérifier si la table existe déjà
	SELECT EXISTS (
		SELECT FROM information_schema.tables 
		WHERE table_name = 'orders'
	);`

	var exists bool
	err := pool.QueryRow(ctx, migrationSQL).Scan(&exists)
	if err != nil {
		return fmt.Errorf("erreur vérification migration: %w", err)
	}

	if exists {
		fmt.Println("📦 Base de données déjà initialisée")
		return nil
	}

	fmt.Println("🔧 Exécution des migrations...")
	// Les migrations sont gérées via le fichier SQL externe
	// Exécuter: psql -f migrations/001_init.sql
	fmt.Println("⚠️  Exécutez manuellement: psql $DATABASE_URL -f migrations/001_init.sql")

	return nil
}
