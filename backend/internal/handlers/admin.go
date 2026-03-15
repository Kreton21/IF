package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

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
