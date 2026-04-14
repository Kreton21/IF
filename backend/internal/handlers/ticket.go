package handlers

import (
	"crypto/rand"
	"encoding/hex"
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

	resp := make([]models.TicketTypeForEmail, 0, len(types))
	for _, tt := range types {
		effectiveMaxPerOrder := tt.MaxPerOrder
		if !tt.OneTicketPerEmail && effectiveMaxPerOrder < 2 {
			effectiveMaxPerOrder = 10
		}
		remaining := tt.QuantityTotal - tt.QuantitySold
		if remaining < 0 {
			remaining = 0
		}
		maxSelectable := effectiveMaxPerOrder
		if remaining < maxSelectable {
			maxSelectable = remaining
		}

		resp = append(resp, models.TicketTypeForEmail{
			ID:                tt.ID,
			Name:              tt.Name,
			Description:       tt.Description,
			PriceCents:        tt.PriceCents,
			MaxPerOrder:       effectiveMaxPerOrder,
			MaxSelectable:     maxSelectable,
			OneTicketPerEmail: tt.OneTicketPerEmail,
			SaleStart:         tt.SaleStart,
			SaleEnd:           tt.SaleEnd,
			IsActive:          tt.IsActive,
			IsAvailable:       remaining > 0,
			Categories:        []models.CategoryForEmail{},
		})
	}

	writeJSON(w, http.StatusOK, resp)
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

	if refCookie, err := r.Cookie("if_ref_code"); err == nil && refCookie != nil {
		req.ReferralCode = strings.TrimSpace(refCookie.Value)
	}
	if visitorCookie, err := r.Cookie("if_ref_vid"); err == nil && visitorCookie != nil {
		req.ReferralVisitorID = strings.TrimSpace(visitorCookie.Value)
	}

	// Récupérer l'IP du client
	ip := extractClientIP(r)

	resp, err := h.ticketService.CreateCheckout(r.Context(), req, ip, r.UserAgent())
	if err != nil {
		log.Printf("Erreur création checkout: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

func (h *TicketHandler) HandleReferralRedirect(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(strings.ToLower(chi.URLParam(r, "code")))
	if code == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	link, err := h.ticketService.GetReferralByCode(r.Context(), code)
	if err != nil {
		log.Printf("Erreur lookup lien parrainage %s: %v", code, err)
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if link == nil || !link.IsActive {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	visitorID := getOrCreateVisitorID(r)
	ip := extractClientIP(r)

	if err := h.ticketService.TrackReferralClick(r.Context(), link.ID, visitorID, ip, r.UserAgent()); err != nil {
		log.Printf("Erreur tracking clic parrainage %s: %v", link.Code, err)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "if_ref_code",
		Value:    link.Code,
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 30,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "if_ref_vid",
		Value:    visitorID,
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 180,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *TicketHandler) GetBusOptions(w http.ResponseWriter, r *http.Request) {
	resp, err := h.ticketService.GetBusOptions(r.Context())
	if err != nil {
		log.Printf("Erreur récupération options bus: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Erreur serveur"})
		return
	}

	outbound := make([]models.PublicBusDeparture, 0, len(resp.OutboundDepartures))
	for _, dep := range resp.OutboundDepartures {
		outbound = append(outbound, models.PublicBusDeparture{
			ID:            dep.ID,
			StationID:     dep.StationID,
			Direction:     dep.Direction,
			DepartureTime: dep.DepartureTime,
			PriceCents:    dep.PriceCents,
			IsActive:      dep.IsActive,
		})
	}

	returns := make([]models.PublicBusDeparture, 0, len(resp.ReturnDepartures))
	for _, dep := range resp.ReturnDepartures {
		returns = append(returns, models.PublicBusDeparture{
			ID:            dep.ID,
			StationID:     dep.StationID,
			Direction:     dep.Direction,
			DepartureTime: dep.DepartureTime,
			PriceCents:    dep.PriceCents,
			IsActive:      dep.IsActive,
		})
	}

	writeJSON(w, http.StatusOK, models.PublicBusOptionsResponse{
		Stations:           resp.Stations,
		OutboundDepartures: outbound,
		ReturnDepartures:   returns,
	})
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

func extractClientIP(r *http.Request) string {
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
	if idx := strings.Index(ip, ","); idx != -1 {
		ip = strings.TrimSpace(ip[:idx])
	}
	return ip
}

func getOrCreateVisitorID(r *http.Request) string {
	if cookie, err := r.Cookie("if_ref_vid"); err == nil && cookie != nil && strings.TrimSpace(cookie.Value) != "" {
		return strings.TrimSpace(cookie.Value)
	}

	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "visitor_fallback"
	}
	return hex.EncodeToString(b)
}
