package router

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/redis/go-redis/v9"

	"github.com/kreton/if-festival/internal/handlers"
	"github.com/kreton/if-festival/internal/middleware"
	"github.com/kreton/if-festival/internal/services"
)

func NewRouter(
	ticketHandler *handlers.TicketHandler,
	webhookHandler *handlers.WebhookHandler,
	adminHandler *handlers.AdminHandler,
	analyticsHandler *handlers.AnalyticsHandler,
	adminService *services.AdminService,
	redisClient *redis.Client,
	frontendDir string,
) *chi.Mux {
	r := chi.NewRouter()

	// Middleware globaux
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Timeout(30 * time.Second))
	r.Use(chimiddleware.Compress(5))

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"X-RateLimit-Limit", "X-RateLimit-Remaining"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	rateLimitDisabled := strings.EqualFold(os.Getenv("DISABLE_RATE_LIMIT"), "1") ||
		strings.EqualFold(os.Getenv("DISABLE_RATE_LIMIT"), "true")

	// Rate limiting global (100 req/min)
	if !rateLimitDisabled {
		r.Use(middleware.RateLimit(redisClient, 100, 1*time.Minute))
	}

	// ==========================================
	// API Routes
	// ==========================================
	r.Route("/api/v1", func(r chi.Router) {

		// --- Public : Tickets ---
		r.Route("/tickets", func(r chi.Router) {
			r.Get("/types", ticketHandler.GetTicketTypes)
			if rateLimitDisabled {
				r.Post("/checkout", ticketHandler.CreateCheckout)
			} else {
				r.With(middleware.StrictRateLimit(redisClient)).Post("/checkout", ticketHandler.CreateCheckout)
			}
		})

		r.Route("/bus", func(r chi.Router) {
			r.Get("/options", ticketHandler.GetBusOptions)
			if rateLimitDisabled {
				r.Post("/checkout", ticketHandler.CreateBusCheckout)
			} else {
				r.With(middleware.StrictRateLimit(redisClient)).Post("/checkout", ticketHandler.CreateBusCheckout)
			}
		})

		r.Post("/camping/claim", ticketHandler.ClaimCampingByEmail)

		// --- Public : Commandes ---
		r.Get("/orders/{id}/status", ticketHandler.GetOrderStatus)

		// --- Public : QR Code image ---
		r.Get("/tickets/{qrToken}/qr", ticketHandler.GetTicketQRCode)

		// --- Public : Analytics ingest ---
		r.Post("/analytics/events", analyticsHandler.Ingest)

		// --- Webhooks HelloAsso ---
		r.Post("/webhooks/helloasso", webhookHandler.HandleHelloAssoWebhook)
		r.Post("/webhooks/lydia", webhookHandler.HandleLydiaWebhook)
		r.Post("/webhooks/lydia/{event}", webhookHandler.HandleLydiaWebhook)
		r.Get("/webhooks/lydia", webhookHandler.HandleLydiaWebhook)
		r.Get("/webhooks/lydia/{event}", webhookHandler.HandleLydiaWebhook)

		// --- Admin (JWT requis) ---
		r.Post("/admin/login", adminHandler.Login)

		r.Route("/admin", func(r chi.Router) {
			r.Use(middleware.JWTAuth(adminService))
			r.Get("/stats", adminHandler.GetStats)
			r.Get("/analytics/kpi", analyticsHandler.GetKPI)

			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireNotRawRole("comm"))

				r.Post("/change-password", adminHandler.ChangePassword)
				r.Post("/staff/change-password", adminHandler.SetStaffPassword)
				r.Post("/test-email", adminHandler.SendTestEmail)
				r.Get("/stats/export-csv", adminHandler.ExportDatabaseCSV)
				r.Get("/orders", adminHandler.ListOrders)
				r.Put("/orders/{id}", adminHandler.UpdateSuccessfulOrder)
				r.Post("/orders/{id}/resend-email", adminHandler.ResendOrderConfirmationEmail)
				r.Post("/orders/resend-confirmations", adminHandler.ResendAllConfirmationEmails)
				r.Post("/validate-qr", adminHandler.ValidateQR)
				r.Get("/ticket-types", adminHandler.GetTicketTypes)
				r.Post("/ticket-types", adminHandler.CreateTicketType)
				r.Put("/ticket-types/{ticketTypeID}", adminHandler.UpdateTicketType)
				r.Post("/ticket-types/{ticketTypeID}/mask", adminHandler.ToggleTicketTypeMask)
				r.Get("/ticket-types/{ticketTypeID}/categories", adminHandler.GetCategories)
				r.Post("/ticket-types/{ticketTypeID}/categories", adminHandler.CreateCategory)
				r.Post("/categories/{categoryID}/mask", adminHandler.ToggleCategoryMask)
				r.Post("/categories/{categoryID}/checkbox", adminHandler.ToggleCategoryCheckbox)
				r.Post("/categories/reallocate", adminHandler.ReallocateCategories)
				r.Delete("/categories/{categoryID}", adminHandler.DeleteCategory)
				r.Get("/bus/options", adminHandler.GetBusOptions)
				r.Post("/bus/stations", adminHandler.CreateBusStation)
				r.Post("/bus/departures", adminHandler.CreateBusDeparture)
				r.Put("/bus/departures/{departureID}", adminHandler.UpdateBusDeparture)
				r.Post("/bus/departures/{departureID}/mask", adminHandler.ToggleBusDepartureMask)
				r.Delete("/bus/departures/{departureID}", adminHandler.DeleteBusDeparture)
				r.Get("/bus/tickets", adminHandler.ListBusTickets)
				r.Get("/referrals", adminHandler.ListReferralLinks)
				r.Post("/referrals", adminHandler.CreateReferralLink)
			})
		})
	})

	r.Get("/r/{code}", ticketHandler.HandleReferralRedirect)

	// ==========================================
	// Frontend static files (no-cache for dev)
	// ==========================================
	noCache := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
			next.ServeHTTP(w, r)
		})
	}

	// Redirect /admin to /admin/
	r.Get("/admin", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/admin/", http.StatusMovedPermanently)
	})

	// Admin interface
	adminFS := http.FileServer(http.Dir(frontendDir + "/admin"))
	r.Handle("/admin/*", noCache(http.StripPrefix("/admin/", adminFS)))

	// Client site with custom 404
	publicDir := frontendDir + "/public"
	r.Handle("/*", noCache(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Try to serve the requested file
		reqPath := req.URL.Path
		if reqPath == "/" {
			reqPath = "/index.html"
		}

		fullPath := filepath.Join(publicDir, filepath.Clean(reqPath))

		// Check if file exists (or dir with index.html)
		info, err := os.Stat(fullPath)
		if err == nil {
			if info.IsDir() {
				indexPath := filepath.Join(fullPath, "index.html")
				if _, err := os.Stat(indexPath); err != nil {
					serve404(w, publicDir)
					return
				}
			}
			// File exists — serve it normally
			http.FileServer(http.Dir(publicDir)).ServeHTTP(w, req)
			return
		}

		// File not found — serve 404 page
		serve404(w, publicDir)
	})))

	return r
}

func serve404(w http.ResponseWriter, publicDir string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusNotFound)
	page, err := os.ReadFile(filepath.Join(publicDir, "404.html"))
	if err != nil {
		w.Write([]byte("<h1>404 — Page introuvable</h1>"))
		return
	}
	w.Write(page)
}
