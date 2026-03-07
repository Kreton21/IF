package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kreton/if-festival/internal/config"
	"github.com/kreton/if-festival/internal/database"
	"github.com/kreton/if-festival/internal/handlers"
	"github.com/kreton/if-festival/internal/repository"
	"github.com/kreton/if-festival/internal/router"
	"github.com/kreton/if-festival/internal/services"
)

func main() {
	log.Println("🎵 IF Festival — Démarrage du serveur...")

	// 1. Charger la configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Erreur configuration: %v", err)
	}

	// 2. Connexion PostgreSQL
	pgPool, err := database.NewPostgresPool(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Erreur PostgreSQL: %v", err)
	}
	defer pgPool.Close()
	log.Println("✅ PostgreSQL connecté")

	// 3. Connexion Redis
	redisClient, err := database.NewRedisClient(cfg.RedisURL)
	if err != nil {
		log.Fatalf("Erreur Redis: %v", err)
	}
	defer redisClient.Close()
	log.Println("✅ Redis connecté")

	// 4. Initialiser les repositories
	ticketRepo := repository.NewTicketRepository(pgPool)
	orderRepo := repository.NewOrderRepository(pgPool)
	adminRepo := repository.NewAdminRepository(pgPool)

	// 5. Initialiser les services
	var helloAssoService *services.HelloAssoService
	if cfg.PaymentProvider == "helloasso" {
		helloAssoService = services.NewHelloAssoService(cfg)
	}

	paymentProvider, err := services.NewPaymentProvider(cfg.PaymentProvider, helloAssoService, cfg.HelloAssoReturnURL)
	if err != nil {
		log.Fatalf("Erreur payment provider: %v", err)
	}

	qrService := services.NewQRCodeService(fmt.Sprintf("http://localhost:%s", cfg.Port))
	emailService := services.NewEmailService(cfg)

	ticketService := services.NewTicketService(cfg, ticketRepo, orderRepo, paymentProvider, qrService, emailService, redisClient)
	adminService := services.NewAdminService(cfg, adminRepo, orderRepo, ticketRepo, redisClient)

	// 6. Initialiser les handlers
	ticketHandler := handlers.NewTicketHandler(ticketService)
	webhookHandler := handlers.NewWebhookHandler(ticketService, adminService)
	adminHandler := handlers.NewAdminHandler(adminService, ticketService)

	// 7. Créer le routeur
	// Determine frontend directory (try relative path, then use env var or default)
	frontendDir := os.Getenv("FRONTEND_DIR")
	if frontendDir == "" {
		frontendDir = "../frontend"
	}
	r := router.NewRouter(ticketHandler, webhookHandler, adminHandler, adminService, redisClient, frontendDir)

	// 8. Créer les comptes par défaut si nécessaire
	go func() {
		ctx := context.Background()
		// Admin principal
		existing, _ := adminRepo.GetByUsername(ctx, "admin")
		if existing == nil {
			err := adminService.CreateAdmin(ctx, "admin", "admin123", "Administrateur", "admin")
			if err != nil {
				log.Printf("⚠️  Erreur création admin par défaut: %v", err)
			} else {
				log.Println("👤 Admin par défaut créé (admin / admin123)")
			}
		}
		// Compte staff (scan QR uniquement)
		existingStaff, _ := adminRepo.GetByUsername(ctx, "staff")
		if existingStaff == nil {
			err := adminService.CreateAdmin(ctx, "staff", "staff2026", "Staff", "staff")
			if err != nil {
				log.Printf("⚠️  Erreur création compte staff: %v", err)
			} else {
				log.Println("👤 Compte staff créé (staff / staff2026)")
			}
		}
	}()

	// 9. Démarrer le serveur
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Println("🛑 Arrêt en cours...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		server.Shutdown(ctx)
	}()

	log.Printf("🚀 Serveur démarré sur http://localhost:%s", cfg.Port)
	log.Printf("   📱 Client: http://localhost:%s/", cfg.Port)
	log.Printf("   🔧 Admin:  http://localhost:%s/admin/", cfg.Port)
	log.Printf("   📡 API:    http://localhost:%s/api/v1/", cfg.Port)

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Erreur serveur: %v", err)
	}

	log.Println("👋 Serveur arrêté")
}
