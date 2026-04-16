package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/kreton/if-festival/internal/services"
)

type WebhookHandler struct {
	ticketService *services.TicketService
	adminService  *services.AdminService
}

func NewWebhookHandler(ticketService *services.TicketService, adminService *services.AdminService) *WebhookHandler {
	return &WebhookHandler{
		ticketService: ticketService,
		adminService:  adminService,
	}
}

// HandleHelloAssoWebhook traite les webhooks de HelloAsso
func (h *WebhookHandler) HandleHelloAssoWebhook(w http.ResponseWriter, r *http.Request) {
	if !isHelloAssoWebhookAuthorized(r) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Erreur lecture webhook body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Enregistrer le webhook pour audit
	var payload services.WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("Erreur décodage webhook: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	logID, err := h.adminService.SaveWebhookLog(r.Context(), payload.EventType, body)
	if err != nil {
		log.Printf("Erreur enregistrement webhook log: %v", err)
	}

	log.Printf("📬 Webhook reçu: type=%s", payload.EventType)

	// Traiter selon le type d'événement
	switch payload.EventType {
	case "Payment", "Order":
		var paymentData services.WebhookPaymentData
		if err := json.Unmarshal(payload.Data, &paymentData); err != nil {
			log.Printf("Erreur décodage payment data: %v", err)
			h.adminService.MarkWebhookProcessed(r.Context(), logID, err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Récupérer l'order_id depuis les metadata
		orderID := paymentData.Metadata["order_id"]
		if orderID == "" {
			log.Printf("WARN: webhook sans order_id dans metadata")
			h.adminService.MarkWebhookProcessed(r.Context(), logID, "order_id manquant")
			w.WriteHeader(http.StatusOK) // On ne veut pas que HelloAsso retry
			return
		}

		// Vérifier le statut du paiement
		if paymentData.State == "Authorized" {
			if err := h.ticketService.ProcessPaymentWebhook(r.Context(), paymentData, orderID); err != nil {
				log.Printf("Erreur traitement webhook: %v", err)
				h.adminService.MarkWebhookProcessed(r.Context(), logID, err.Error())
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		} else {
			log.Printf("Paiement non autorisé (state=%s) pour commande %s", paymentData.State, orderID)
		}

		h.adminService.MarkWebhookProcessed(r.Context(), logID, "")

	default:
		log.Printf("Type de webhook ignoré: %s", payload.EventType)
		h.adminService.MarkWebhookProcessed(r.Context(), logID, "type ignoré")
	}

	w.WriteHeader(http.StatusOK)
}

// HandleLydiaWebhook traite les callbacks Lydia (confirm/cancel/expire)
func (h *WebhookHandler) HandleLydiaWebhook(w http.ResponseWriter, r *http.Request) {
	event := chi.URLParam(r, "event")
	if event == "" {
		event = r.URL.Query().Get("event")
	}
	if event == "" {
		event = "confirm"
	}

	if err := r.ParseForm(); err != nil {
		log.Printf("Erreur parse form Lydia: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	payload := r.Form

	if isLydiaDebugEnabled() {
		log.Printf("[LYDIA DEBUG] webhook event=%s payload=%v", event, redactFormForLog(payload))
	}

	formJSON, _ := json.Marshal(payload)
	logID, err := h.adminService.SaveWebhookLog(r.Context(), "lydia:"+event, formJSON)
	if err != nil {
		log.Printf("Erreur enregistrement webhook Lydia: %v", err)
	}

	if err := h.ticketService.HandleLydiaWebhook(r.Context(), event, payload); err != nil {
		log.Printf("Erreur traitement webhook Lydia (%s): %v", event, err)
		h.adminService.MarkWebhookProcessed(r.Context(), logID, err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	h.adminService.MarkWebhookProcessed(r.Context(), logID, "")
	w.WriteHeader(http.StatusOK)
}

func isLydiaDebugEnabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("LYDIA_DEBUG")))
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

func isHelloAssoWebhookAuthorized(r *http.Request) bool {
	secret := strings.TrimSpace(os.Getenv("HELLOASSO_WEBHOOK_SECRET"))
	if secret == "" {
		return true
	}

	provided := strings.TrimSpace(r.Header.Get("X-Webhook-Secret"))
	if provided == "" {
		provided = strings.TrimSpace(r.Header.Get("X-HelloAsso-Webhook-Secret"))
	}
	if provided == "" {
		provided = strings.TrimSpace(r.Header.Get("X-HelloAsso-Secret"))
	}
	if provided == "" {
		provided = strings.TrimSpace(r.URL.Query().Get("secret"))
	}
	if provided == "" {
		log.Printf("WARN: webhook HelloAsso sans secret (validation relâchée)")
		return true
	}

	return subtle.ConstantTimeCompare([]byte(provided), []byte(secret)) == 1
}

func redactFormForLog(form map[string][]string) map[string]string {
	result := make(map[string]string, len(form))
	for key, values := range form {
		if len(values) == 0 {
			continue
		}
		value := values[0]
		switch strings.ToLower(key) {
		case "sig":
			result[key] = "***"
		default:
			result[key] = value
		}
	}
	return result
}
