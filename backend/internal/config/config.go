package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port string
	Env  string
	BaseURL string

	// PostgreSQL
	DatabaseURL string

	// Redis
	RedisURL string

	// Payment provider: "mock" or "helloasso"
	PaymentProvider string

	// HelloAsso (required only when PaymentProvider = "helloasso")
	HelloAssoClientID     string
	HelloAssoClientSecret string
	HelloAssoOrgSlug      string
	HelloAssoAPIURL       string
	HelloAssoReturnURL    string
	HelloAssoErrorURL     string

	// Lydia (required only when PaymentProvider = "lydia")
	LydiaAPIURL             string
	LydiaVendorToken        string
	LydiaVendorPrivateToken string
	LydiaPaymentMethod      string
	LydiaDebug              bool

	// JWT
	JWTSecret string

	// SMTP
	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string
	SMTPFromName string
	EnableAdminTestEmail bool
	EmailTemplatePath    string
	TicketPDFTemplatePath string
	TicketPDFEngine       string
	WKHTMLTOPDFBin        string
	EmailSubjectTemplate string
	BusEmailTemplatePath    string
	BusEmailSubjectTemplate string

	// Festival
	FestivalName string
	FestivalDate string
}

func Load() (*Config, error) {
	// Charger .env si présent (ignoré en production)
	_ = godotenv.Load()

	smtpPort, _ := strconv.Atoi(getEnv("SMTP_PORT", "587"))
	enableAdminTestEmail, _ := strconv.ParseBool(getEnv("ENABLE_ADMIN_TEST_EMAIL", "false"))

	paymentProvider := getEnv("PAYMENT_PROVIDER", "mock")
	lydiaDebug, _ := strconv.ParseBool(getEnv("LYDIA_DEBUG", "false"))

	cfg := &Config{
		Port: getEnv("PORT", "8080"),
		Env:  getEnv("ENV", "development"),
		BaseURL: getEnv("BASE_URL", "http://localhost:8080"),

		DatabaseURL: mustGetEnv("DATABASE_URL"),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379/0"),

		PaymentProvider: paymentProvider,

		// HelloAsso — only required when PAYMENT_PROVIDER=helloasso
		HelloAssoClientID:     getEnv("HELLOASSO_CLIENT_ID", ""),
		HelloAssoClientSecret: getEnv("HELLOASSO_CLIENT_SECRET", ""),
		HelloAssoOrgSlug:      getEnv("HELLOASSO_ORG_SLUG", ""),
		HelloAssoAPIURL:       getEnv("HELLOASSO_API_URL", "https://api.helloasso.com"),
		HelloAssoReturnURL:    getEnv("HELLOASSO_RETURN_URL", getEnv("BASE_URL", "http://localhost:8080")+"/"),
		HelloAssoErrorURL:     getEnv("HELLOASSO_ERROR_URL", getEnv("BASE_URL", "http://localhost:8080")+"/"),

		LydiaAPIURL:             getEnv("LYDIA_API_URL", "https://homologation.lydia-app.com"),
		LydiaVendorToken:        getEnv("LYDIA_VENDOR_TOKEN", ""),
		LydiaVendorPrivateToken: getEnv("LYDIA_VENDOR_PRIVATE_TOKEN", ""),
		LydiaPaymentMethod:      getEnv("LYDIA_PAYMENT_METHOD", ""),
		LydiaDebug:              lydiaDebug,

		JWTSecret: mustGetEnv("JWT_SECRET"),

		SMTPHost:     getEnv("SMTP_HOST", ""),
		SMTPPort:     smtpPort,
		SMTPUser:     getEnv("SMTP_USER", ""),
		SMTPPassword: getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:     getEnv("SMTP_FROM", ""),
		SMTPFromName: getEnv("SMTP_FROM_NAME", "IF Festival"),
		EnableAdminTestEmail: enableAdminTestEmail,
		EmailTemplatePath:    getEnv("EMAIL_TEMPLATE_PATH", "mail/ticket_email.html"),
		TicketPDFTemplatePath: getEnv("TICKET_PDF_TEMPLATE_PATH", "mail/ticket_pdf.html"),
		TicketPDFEngine:       getEnv("TICKET_PDF_ENGINE", "auto"),
		WKHTMLTOPDFBin:        getEnv("WKHTMLTOPDF_BIN", "wkhtmltopdf"),
		EmailSubjectTemplate: getEnv("EMAIL_SUBJECT_TEMPLATE", "{{.FestivalName}} - Vos billets (Commande {{.OrderNumber}})"),
		BusEmailTemplatePath:    getEnv("BUS_EMAIL_TEMPLATE_PATH", "templates/bus_ticket_confirmation.html"),
		BusEmailSubjectTemplate: getEnv("BUS_EMAIL_SUBJECT_TEMPLATE", "{{.FestivalName}} - Votre ticket navette (Commande {{.OrderNumber}})"),

		FestivalName: getEnv("FESTIVAL_NAME", "IF Festival"),
		FestivalDate: getEnv("FESTIVAL_DATE", "2026-07-15"),
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func mustGetEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		fmt.Printf("WARNING: variable d'environnement %s non définie\n", key)
	}
	return val
}
