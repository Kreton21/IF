package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/kreton/if-festival/internal/middleware"
	"github.com/kreton/if-festival/internal/models"
	"github.com/kreton/if-festival/internal/services"
)

type AdminHandler struct {
	adminService  *services.AdminService
	ticketService *services.TicketService
}

func NewAdminHandler(adminService *services.AdminService, ticketService *services.TicketService) *AdminHandler {
	return &AdminHandler{
		adminService:  adminService,
		ticketService: ticketService,
	}
}

// Login authentifie un admin
func (h *AdminHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Données invalides"})
		return
	}

	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Username et password requis"})
		return
	}

	resp, err := h.adminService.Login(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Identifiants invalides"})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// ChangePassword permet uniquement au super-admin de changer son mot de passe
func (h *AdminHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	rawRole := middleware.GetAdminRawRole(r.Context())
	if rawRole != "super-admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé au super-admin"})
		return
	}

	adminID := middleware.GetAdminID(r.Context())
	if adminID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Session invalide"})
		return
	}

	var req models.ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Données invalides"})
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Mot de passe actuel et nouveau requis"})
		return
	}

	err := h.adminService.ChangePassword(r.Context(), adminID, req.CurrentPassword, req.NewPassword)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Mot de passe mis à jour"})
}

// SetStaffPassword permet uniquement au super-admin de changer le mot de passe d'un staff
func (h *AdminHandler) SetStaffPassword(w http.ResponseWriter, r *http.Request) {
	rawRole := middleware.GetAdminRawRole(r.Context())
	if rawRole != "super-admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé au super-admin"})
		return
	}

	var req models.SetStaffPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Données invalides"})
		return
	}

	if req.Username == "" || req.NewPassword == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Username et nouveau mot de passe requis"})
		return
	}

	if err := h.adminService.SetStaffPassword(r.Context(), req.Username, req.NewPassword); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Mot de passe staff mis à jour"})
}

// GetStats retourne les statistiques de vente
func (h *AdminHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.adminService.GetStats(r.Context())
	if err != nil {
		log.Printf("Erreur récupération stats: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Erreur serveur"})
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

func (h *AdminHandler) ExportDatabaseCSV(w http.ResponseWriter, r *http.Request) {
	role := middleware.GetAdminRole(r.Context())
	if role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé aux administrateurs"})
		return
	}

	csvData, err := h.adminService.ExportDatabaseCSV(r.Context())
	if err != nil {
		log.Printf("Erreur export CSV: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Erreur export CSV"})
		return
	}

	filename := "database_export_" + time.Now().UTC().Format("20060102_150405") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(csvData)
}

func (h *AdminHandler) SendTestEmail(w http.ResponseWriter, r *http.Request) {
	if !h.adminService.IsTestEmailEnabled() {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "fonction non disponible"})
		return
	}

	role := middleware.GetAdminRole(r.Context())
	if role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé aux administrateurs"})
		return
	}

	var req struct {
		To string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Données invalides"})
		return
	}
	if req.To == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Adresse email destinataire requise"})
		return
	}

	adminName := middleware.GetAdminName(r.Context())
	log.Printf("📧 [ADMIN TEST EMAIL] Demande envoyée par %s vers %s", adminName, req.To)

	if err := h.adminService.SendTestEmail(r.Context(), req.To); err != nil {
		log.Printf("❌ [ADMIN TEST EMAIL] Échec envoi vers %s: %v", req.To, err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	log.Printf("✅ [ADMIN TEST EMAIL] Email de test envoyé vers %s", req.To)
	writeJSON(w, http.StatusOK, map[string]string{"message": "Email de test envoyé"})
}

// ListOrders retourne la liste paginée des commandes
func (h *AdminHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	status := r.URL.Query().Get("status")
	search := r.URL.Query().Get("search")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	params := models.OrderListParams{
		Page:     page,
		PageSize: pageSize,
		Status:   models.OrderStatus(status),
		Search:   search,
	}

	resp, err := h.adminService.ListOrders(r.Context(), params)
	if err != nil {
		log.Printf("Erreur liste commandes: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Erreur serveur"})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *AdminHandler) ResendOrderConfirmationEmail(w http.ResponseWriter, r *http.Request) {
	role := middleware.GetAdminRole(r.Context())
	if role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé aux administrateurs"})
		return
	}

	orderID := chi.URLParam(r, "id")
	if strings.TrimSpace(orderID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ID commande requis"})
		return
	}

	if err := h.ticketService.ResendOrderConfirmationEmail(r.Context(), orderID); err != nil {
		log.Printf("Erreur renvoi email commande %s: %v", orderID, err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Email de confirmation renvoyé"})
}

func (h *AdminHandler) ResendAllConfirmationEmails(w http.ResponseWriter, r *http.Request) {
	role := middleware.GetAdminRole(r.Context())
	if role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé aux administrateurs"})
		return
	}

	sent, failed, err := h.ticketService.ResendAllConfirmationEmails(r.Context())
	if err != nil {
		log.Printf("Erreur renvoi global emails: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Erreur serveur"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"sent_orders": sent, "failed_orders": failed})
}

// ValidateQR valide un QR code à l'entrée
func (h *AdminHandler) ValidateQR(w http.ResponseWriter, r *http.Request) {
	var req models.ValidateQRRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Données invalides"})
		return
	}

	if req.QRToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "QR token manquant"})
		return
	}

	adminName := middleware.GetAdminName(r.Context())

	resp, err := h.adminService.ValidateQR(r.Context(), req.QRToken, adminName)
	if err != nil {
		log.Printf("Erreur validation QR: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Erreur serveur"})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// GetTicketTypes retourne tous les types de tickets (pour admin, inclut les inactifs et masqués)
func (h *AdminHandler) GetTicketTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.ticketService.GetAllTicketTypes(r.Context())
	if err != nil {
		log.Printf("Erreur récupération ticket types: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Erreur serveur"})
		return
	}

	writeJSON(w, http.StatusOK, types)
}

// CreateTicketType crée un nouveau type de ticket
func (h *AdminHandler) CreateTicketType(w http.ResponseWriter, r *http.Request) {
	role := middleware.GetAdminRole(r.Context())
	if role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé aux administrateurs"})
		return
	}

	var req models.CreateTicketTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Données invalides"})
		return
	}

	if req.Name == "" || req.PriceCents < 0 || req.QuantityTotal < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Données incomplètes"})
		return
	}

	if req.MaxPerOrder < 1 {
		req.MaxPerOrder = 5
	}
	if req.OneTicketPerEmail {
		req.MaxPerOrder = 1
	}

	tt, err := h.ticketService.CreateTicketType(r.Context(), req)
	if err != nil {
		log.Printf("Erreur création ticket type: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Erreur lors de la création"})
		return
	}

	writeJSON(w, http.StatusCreated, tt)
}

// GetCategories retourne les catégories d'un type de ticket
func (h *AdminHandler) GetCategories(w http.ResponseWriter, r *http.Request) {
	ticketTypeID := chi.URLParam(r, "ticketTypeID")
	if ticketTypeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ID type de ticket manquant"})
		return
	}

	cats, err := h.ticketService.GetCategoriesByTicketType(r.Context(), ticketTypeID)
	if err != nil {
		log.Printf("Erreur récupération catégories: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Erreur serveur"})
		return
	}

	if cats == nil {
		cats = []models.TicketCategory{}
	}
	writeJSON(w, http.StatusOK, cats)
}

// CreateCategory crée une catégorie pour un type de ticket
func (h *AdminHandler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	role := middleware.GetAdminRole(r.Context())
	if role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé aux administrateurs"})
		return
	}

	var req models.CreateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Données invalides"})
		return
	}

	ticketTypeID := chi.URLParam(r, "ticketTypeID")
	if ticketTypeID != "" {
		req.TicketTypeID = ticketTypeID
	}

	if req.TicketTypeID == "" || req.Name == "" || req.Quantity < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Données incomplètes (ticket_type_id, name, quantity requis)"})
		return
	}

	cat, err := h.ticketService.CreateCategory(r.Context(), req)
	if err != nil {
		log.Printf("Erreur création catégorie: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, cat)
}

// ReallocateCategories réalloue des places entre catégories
func (h *AdminHandler) ReallocateCategories(w http.ResponseWriter, r *http.Request) {
	role := middleware.GetAdminRole(r.Context())
	if role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé aux administrateurs"})
		return
	}

	var req models.ReallocateCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Données invalides"})
		return
	}

	if req.SourceCategoryID == "" || req.TargetCategoryID == "" || req.Quantity < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Données incomplètes"})
		return
	}

	err := h.ticketService.ReallocateCategories(r.Context(), req)
	if err != nil {
		log.Printf("Erreur réallocation: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Réallocation effectuée"})
}

// UpdateTicketType met à jour un type de ticket existant
func (h *AdminHandler) UpdateTicketType(w http.ResponseWriter, r *http.Request) {
	role := middleware.GetAdminRole(r.Context())
	if role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé aux administrateurs"})
		return
	}

	ticketTypeID := chi.URLParam(r, "ticketTypeID")
	if ticketTypeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ID type de ticket manquant"})
		return
	}

	var req models.UpdateTicketTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Données invalides"})
		return
	}

	if req.Name == "" || req.PriceCents < 0 || req.QuantityTotal < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Données incomplètes"})
		return
	}

	tt, err := h.ticketService.UpdateTicketType(r.Context(), ticketTypeID, req)
	if err != nil {
		log.Printf("Erreur mise à jour ticket type: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, tt)
}

// ToggleTicketTypeMask masque/démasque un type de ticket
func (h *AdminHandler) ToggleTicketTypeMask(w http.ResponseWriter, r *http.Request) {
	role := middleware.GetAdminRole(r.Context())
	if role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé aux administrateurs"})
		return
	}

	ticketTypeID := chi.URLParam(r, "ticketTypeID")
	if ticketTypeID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ID type de ticket manquant"})
		return
	}

	tt, err := h.ticketService.ToggleTicketTypeMask(r.Context(), ticketTypeID)
	if err != nil {
		log.Printf("Erreur toggle mask ticket type: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, tt)
}

func (h *AdminHandler) GetBusOptions(w http.ResponseWriter, r *http.Request) {
	resp, err := h.ticketService.GetBusOptions(r.Context())
	if err != nil {
		log.Printf("Erreur récupération options bus admin: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Erreur serveur"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *AdminHandler) CreateBusStation(w http.ResponseWriter, r *http.Request) {
	role := middleware.GetAdminRole(r.Context())
	if role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé aux administrateurs"})
		return
	}

	var req models.CreateBusStationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Données invalides"})
		return
	}

	station, err := h.ticketService.CreateBusStation(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, station)
}

func (h *AdminHandler) CreateBusDeparture(w http.ResponseWriter, r *http.Request) {
	role := middleware.GetAdminRole(r.Context())
	if role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé aux administrateurs"})
		return
	}

	var req models.CreateBusDepartureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Données invalides"})
		return
	}

	dep, err := h.ticketService.CreateBusDeparture(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, dep)
}

func (h *AdminHandler) UpdateBusDeparture(w http.ResponseWriter, r *http.Request) {
	role := middleware.GetAdminRole(r.Context())
	if role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé aux administrateurs"})
		return
	}

	departureID := chi.URLParam(r, "departureID")
	if departureID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ID départ navette manquant"})
		return
	}

	var req models.UpdateBusDepartureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Données invalides"})
		return
	}

	dep, err := h.ticketService.UpdateBusDeparture(r.Context(), departureID, req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, dep)
}

func (h *AdminHandler) ToggleBusDepartureMask(w http.ResponseWriter, r *http.Request) {
	role := middleware.GetAdminRole(r.Context())
	if role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé aux administrateurs"})
		return
	}

	departureID := chi.URLParam(r, "departureID")
	if departureID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ID départ navette manquant"})
		return
	}

	dep, err := h.ticketService.ToggleBusDepartureMask(r.Context(), departureID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, dep)
}

func (h *AdminHandler) DeleteBusDeparture(w http.ResponseWriter, r *http.Request) {
	role := middleware.GetAdminRole(r.Context())
	if role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé aux administrateurs"})
		return
	}

	departureID := chi.URLParam(r, "departureID")
	if departureID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ID départ navette manquant"})
		return
	}

	if err := h.ticketService.DeleteBusDeparture(r.Context(), departureID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Départ navette supprimé"})
}

func (h *AdminHandler) ListBusTickets(w http.ResponseWriter, r *http.Request) {
	rows, err := h.ticketService.ListBusTicketsAdmin(r.Context())
	if err != nil {
		log.Printf("Erreur récupération tickets bus admin: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Erreur serveur"})
		return
	}

	writeJSON(w, http.StatusOK, rows)
}

func (h *AdminHandler) CreateReferralLink(w http.ResponseWriter, r *http.Request) {
	role := middleware.GetAdminRole(r.Context())
	if role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé aux administrateurs"})
		return
	}

	var req models.CreateReferralLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Données invalides"})
		return
	}

	baseURL := publicBaseURL(r)
	resp, err := h.adminService.CreateReferralLink(r.Context(), req, baseURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

func (h *AdminHandler) ListReferralLinks(w http.ResponseWriter, r *http.Request) {
	role := middleware.GetAdminRole(r.Context())
	if role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé aux administrateurs"})
		return
	}

	rows, err := h.adminService.ListReferralLinks(r.Context(), publicBaseURL(r))
	if err != nil {
		log.Printf("Erreur récupération liens parrainage: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Erreur serveur"})
		return
	}

	if rows == nil {
		rows = []models.ReferralLinkRow{}
	}
	writeJSON(w, http.StatusOK, rows)
}

// ToggleCategoryMask masque/démasque une catégorie
func (h *AdminHandler) ToggleCategoryMask(w http.ResponseWriter, r *http.Request) {
	role := middleware.GetAdminRole(r.Context())
	if role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé aux administrateurs"})
		return
	}

	categoryID := chi.URLParam(r, "categoryID")
	if categoryID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ID catégorie manquant"})
		return
	}

	cat, err := h.ticketService.ToggleCategoryMask(r.Context(), categoryID)
	if err != nil {
		log.Printf("Erreur toggle mask catégorie: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, cat)
}

// ToggleCategoryCheckbox active/désactive le mode "case à cocher" d'une catégorie
func (h *AdminHandler) ToggleCategoryCheckbox(w http.ResponseWriter, r *http.Request) {
	role := middleware.GetAdminRole(r.Context())
	if role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé aux administrateurs"})
		return
	}

	categoryID := chi.URLParam(r, "categoryID")
	if categoryID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ID catégorie manquant"})
		return
	}

	cat, err := h.ticketService.ToggleCategoryCheckbox(r.Context(), categoryID)
	if err != nil {
		log.Printf("Erreur toggle checkbox catégorie: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, cat)
}

// DeleteCategory supprime une catégorie vide
func (h *AdminHandler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	role := middleware.GetAdminRole(r.Context())
	if role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Accès réservé aux administrateurs"})
		return
	}

	categoryID := chi.URLParam(r, "categoryID")
	if categoryID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ID catégorie manquant"})
		return
	}

	err := h.ticketService.DeleteCategory(r.Context(), categoryID)
	if err != nil {
		log.Printf("Erreur suppression catégorie: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Catégorie supprimée"})
}

func publicBaseURL(r *http.Request) string {
	scheme := "http"
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") || r.TLS != nil {
		scheme = "https"
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		host = "localhost"
	}
	return scheme + "://" + host
}
