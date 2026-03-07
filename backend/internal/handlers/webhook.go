package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

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
