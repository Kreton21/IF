package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/kreton/if-festival/internal/models"
	"github.com/kreton/if-festival/internal/services"
)

type TicketHandler struct {
	ticketService *services.TicketService
}

func NewTicketHandler(ticketService *services.TicketService) *TicketHandler {
	return &TicketHandler{ticketService: ticketService}
}

// GetTicketTypes retourne les types de tickets disponibles
// Si ?email= est fourni, filtre par domaine email et renvoie les catégories
func (h *TicketHandler) GetTicketTypes(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")

	if email != "" {
		// Filtrer par email → retourne les types avec catégories accessibles
		types, err := h.ticketService.GetTicketTypesForEmail(r.Context(), email)
		if err != nil {
			log.Printf("Erreur récupération tickets pour email: %v", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, types)
		return
	}

	// Sans email → retourne tous les types actifs (ancienne API)
	types, err := h.ticketService.GetAvailableTicketTypes(r.Context())
	if err != nil {
		log.Printf("Erreur récupération ticket types: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Erreur serveur"})
		return
	}

	writeJSON(w, http.StatusOK, types)
}

// CreateCheckout crée un checkout HelloAsso
func (h *TicketHandler) CreateCheckout(w http.ResponseWriter, r *http.Request) {
	var req models.CheckoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Données invalides"})
		return
	}

	// Validation basique
	if req.CustomerEmail == "" || req.CustomerFirstName == "" || req.CustomerLastName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Email, prénom et nom sont requis"})
		return
	}
	if strings.TrimSpace(req.DateOfBirth) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Date de naissance requise"})
		return
	}
	if len(req.Items) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Aucun ticket sélectionné"})
		return
	}
	if !isValidEmail(req.CustomerEmail) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Email invalide"})
		return
	}

	// Récupérer l'IP du client
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		// Handle IPv6 [::1]:port format
		addr := r.RemoteAddr
		if strings.HasPrefix(addr, "[") {
			// IPv6: [::1]:port
			if idx := strings.LastIndex(addr, "]"); idx != -1 {
				ip = addr[1:idx]
			} else {
				ip = "127.0.0.1"
			}
		} else if idx := strings.LastIndex(addr, ":"); idx != -1 {
			ip = addr[:idx]
		} else {
			ip = addr
		}
	}
	// Fallback to localhost if empty
	if ip == "" {
		ip = "127.0.0.1"
	}

	resp, err := h.ticketService.CreateCheckout(r.Context(), req, ip, r.UserAgent())
	if err != nil {
		log.Printf("Erreur création checkout: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

func (h *TicketHandler) GetBusOptions(w http.ResponseWriter, r *http.Request) {
	resp, err := h.ticketService.GetBusOptions(r.Context())
	if err != nil {
		log.Printf("Erreur récupération options bus: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Erreur serveur"})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *TicketHandler) CreateBusCheckout(w http.ResponseWriter, r *http.Request) {
	var req models.BusCheckoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Données invalides"})
		return
	}

	if req.CustomerEmail == "" || req.CustomerFirstName == "" || req.CustomerLastName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Email, prénom et nom sont requis"})
		return
	}
	if !isValidEmail(req.CustomerEmail) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Email invalide"})
		return
	}

	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		addr := r.RemoteAddr
		if strings.HasPrefix(addr, "[") {
			if idx := strings.LastIndex(addr, "]"); idx != -1 {
				ip = addr[1:idx]
			} else {
				ip = "127.0.0.1"
			}
		} else if idx := strings.LastIndex(addr, ":"); idx != -1 {
			ip = addr[:idx]
		} else {
			ip = addr
		}
	}
	if ip == "" {
		ip = "127.0.0.1"
	}

	resp, err := h.ticketService.CreateBusCheckout(r.Context(), req, ip, r.UserAgent())
	if err != nil {
		log.Printf("Erreur création checkout bus: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

func (h *TicketHandler) ClaimCampingByEmail(w http.ResponseWriter, r *http.Request) {
	var req models.CampingClaimRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Données invalides"})
		return
	}

	resp, err := h.ticketService.ClaimCampingByEmail(r.Context(), req.Email)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetOrderStatus retourne le statut d'une commande
func (h *TicketHandler) GetOrderStatus(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "id")
	if orderID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ID commande manquant"})
		return
	}

	order, err := h.ticketService.GetOrderStatus(r.Context(), orderID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Commande introuvable"})
		return
	}

	writeJSON(w, http.StatusOK, order)
}

// GetTicketQRCode returns the QR code PNG image for a ticket
func (h *TicketHandler) GetTicketQRCode(w http.ResponseWriter, r *http.Request) {
	qrToken := chi.URLParam(r, "qrToken")
	if qrToken == "" {
		http.Error(w, "Token manquant", http.StatusBadRequest)
		return
	}

	data, err := h.ticketService.GetQRCodeImage(r.Context(), qrToken)
	if err != nil || len(data) == 0 {
		http.Error(w, "QR code introuvable", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data)
}

// Helpers

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func isValidEmail(email string) bool {
	return strings.Contains(email, "@") && strings.Contains(email, ".")
}
