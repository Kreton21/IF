package services

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/url"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/kreton/if-festival/internal/config"
	"github.com/kreton/if-festival/internal/models"
	"github.com/kreton/if-festival/internal/repository"
)

// TicketService orchestre la logique métier d'achat de tickets
type TicketService struct {
	cfg             *config.Config
	ticketRepo      *repository.TicketRepository
	orderRepo       *repository.OrderRepository
	paymentProvider PaymentProvider
	qrService       *QRCodeService
	emailService    *EmailService
	redis           *redis.Client
}

func NewTicketService(
	cfg *config.Config,
	ticketRepo *repository.TicketRepository,
	orderRepo *repository.OrderRepository,
	paymentProvider PaymentProvider,
	qrService *QRCodeService,
	emailService *EmailService,
	redis *redis.Client,
) *TicketService {
	return &TicketService{
		cfg:             cfg,
		ticketRepo:      ticketRepo,
		orderRepo:       orderRepo,
		paymentProvider: paymentProvider,
		qrService:       qrService,
		emailService:    emailService,
		redis:           redis,
	}
}

// GetAvailableTicketTypes retourne les types de tickets en vente
func (s *TicketService) GetAvailableTicketTypes(ctx context.Context) ([]models.TicketType, error) {
	// Tenter le cache Redis d'abord
	cacheKey := "ticket_types:active"
	cached, err := s.redis.Get(ctx, cacheKey).Bytes()
	if err == nil {
		var types []models.TicketType
		if json.Unmarshal(cached, &types) == nil {
			return types, nil
		}
	}

	types, err := s.ticketRepo.GetActiveTicketTypes(ctx)
	if err != nil {
		return nil, err
	}

	// Cache 30 secondes (courte durée car le stock change)
	if data, err := json.Marshal(types); err == nil {
		s.redis.Set(ctx, cacheKey, data, 30*time.Second)
	}

	return types, nil
}

// CreateTicketType crée un nouveau type de ticket
func (s *TicketService) CreateTicketType(ctx context.Context, req models.CreateTicketTypeRequest) (*models.TicketType, error) {
	tt, err := s.ticketRepo.CreateTicketType(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("erreur création type ticket: %w", err)
	}

	// Invalider le cache
	s.redis.Del(ctx, "ticket_types:active")

	return tt, nil
}

// UpdateTicketType updates an existing ticket type
func (s *TicketService) UpdateTicketType(ctx context.Context, id string, req models.UpdateTicketTypeRequest) (*models.TicketType, error) {
	tt, err := s.ticketRepo.UpdateTicketType(ctx, id, req)
	if err != nil {
		return nil, fmt.Errorf("erreur mise à jour type ticket: %w", err)
	}
	s.redis.Del(ctx, "ticket_types:active")
	return tt, nil
}

// GetAllTicketTypes returns all ticket types for admin (including inactive and masked)
func (s *TicketService) GetAllTicketTypes(ctx context.Context) ([]models.TicketType, error) {
	return s.ticketRepo.GetAllTicketTypes(ctx)
}

// ToggleTicketTypeMask toggles the masked status of a ticket type
func (s *TicketService) ToggleTicketTypeMask(ctx context.Context, id string) (*models.TicketType, error) {
	tt, err := s.ticketRepo.ToggleTicketTypeMask(ctx, id)
	if err != nil {
		return nil, err
	}
	s.redis.Del(ctx, "ticket_types:active")
	return tt, nil
}

// ToggleCategoryMask toggles the masked status of a category
func (s *TicketService) ToggleCategoryMask(ctx context.Context, categoryID string) (*models.TicketCategory, error) {
	cat, err := s.ticketRepo.ToggleCategoryMask(ctx, categoryID)
	if err != nil {
		return nil, err
	}
	s.redis.Del(ctx, "ticket_types:active")
	return cat, nil
}

// ============================================
// Catégories
// ============================================

func (s *TicketService) GetCategoriesByTicketType(ctx context.Context, ticketTypeID string) ([]models.TicketCategory, error) {
	return s.ticketRepo.GetCategoriesByTicketType(ctx, ticketTypeID)
}

func (s *TicketService) CreateCategory(ctx context.Context, req models.CreateCategoryRequest) (*models.TicketCategory, error) {
	cat, err := s.ticketRepo.CreateCategory(ctx, req)
	if err != nil {
		return nil, err
	}
	s.redis.Del(ctx, "ticket_types:active")
	return cat, nil
}

func (s *TicketService) ReallocateCategories(ctx context.Context, req models.ReallocateCategoryRequest) error {
	err := s.ticketRepo.ReallocateCategories(ctx, req)
	if err != nil {
		return err
	}
	s.redis.Del(ctx, "ticket_types:active")
	return nil
}

func (s *TicketService) DeleteCategory(ctx context.Context, categoryID string) error {
	err := s.ticketRepo.DeleteCategory(ctx, categoryID)
	if err != nil {
		return err
	}
	s.redis.Del(ctx, "ticket_types:active")
	return nil
}

// GetTicketTypesForEmail returns ticket types and their categories filtered for a given email domain
func (s *TicketService) GetTicketTypesForEmail(ctx context.Context, email string) ([]models.TicketTypeForEmail, error) {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("email invalide")
	}
	domain := strings.ToLower(parts[1])

	allTypes, err := s.GetAvailableTicketTypes(ctx)
	if err != nil {
		return nil, err
	}

	var result []models.TicketTypeForEmail
	for _, tt := range allTypes {
		// Check if user's domain has access to this ticket type
		if len(tt.AllowedDomains) > 0 {
			domainMatch := false
			for _, d := range tt.AllowedDomains {
				if strings.ToLower(d) == domain {
					domainMatch = true
					break
				}
			}
			if !domainMatch {
				continue
			}
		}

		// Get categories for this ticket type
		cats, err := s.ticketRepo.GetCategoriesByTicketType(ctx, tt.ID)
		if err != nil {
			log.Printf("Erreur chargement catégories pour %s: %v", tt.ID, err)
			continue
		}

		// Filter categories by email domain and masked status
		var filteredCats []models.CategoryForEmail
		for _, cat := range cats {
			if cat.IsMasked {
				continue
			}
			if len(cat.AllowedDomains) > 0 {
				catMatch := false
				for _, d := range cat.AllowedDomains {
					if strings.ToLower(d) == domain {
						catMatch = true
						break
					}
				}
				if !catMatch {
					continue
				}
			}
			filteredCats = append(filteredCats, models.CategoryForEmail{
				ID:                cat.ID,
				Name:              cat.Name,
				QuantityAllocated: cat.QuantityAllocated,
				QuantitySold:      cat.QuantitySold,
			})
		}

		// Only include ticket type if it has matching categories or no categories at all
		if len(cats) == 0 || len(filteredCats) > 0 {
			ttForEmail := models.TicketTypeForEmail{
				ID:            tt.ID,
				Name:          tt.Name,
				Description:   tt.Description,
				PriceCents:    tt.PriceCents,
				QuantityTotal: tt.QuantityTotal,
				QuantitySold:  tt.QuantitySold,
				MaxPerOrder:   tt.MaxPerOrder,
				SaleStart:     tt.SaleStart,
				SaleEnd:       tt.SaleEnd,
				IsActive:      tt.IsActive,
				Categories:    filteredCats,
			}
			result = append(result, ttForEmail)
		}
	}

	return result, nil
}

// CreateCheckout crée une commande et redirige vers HelloAsso
func (s *TicketService) CreateCheckout(ctx context.Context, req models.CheckoutRequest, ipAddress, userAgent string) (*models.CheckoutResponse, error) {
	// 1. Valider les items et calculer le total
	totalCents := 0
	var validatedItems []struct {
		ticketType *models.TicketType
		quantity   int
	}

	for _, item := range req.Items {
		if item.Quantity < 1 {
			return nil, fmt.Errorf("quantité invalide pour %s", item.TicketTypeID)
		}

		tt, err := s.ticketRepo.GetTicketTypeByID(ctx, item.TicketTypeID)
		if err != nil {
			return nil, fmt.Errorf("erreur récupération type ticket: %w", err)
		}
		if tt == nil {
			return nil, fmt.Errorf("type de ticket %s introuvable", item.TicketTypeID)
		}
		if !tt.IsOnSale() {
			return nil, fmt.Errorf("le ticket '%s' n'est plus en vente", tt.Name)
		}
		if item.Quantity > tt.MaxPerOrder {
			return nil, fmt.Errorf("maximum %d tickets '%s' par commande", tt.MaxPerOrder, tt.Name)
		}

		totalCents += tt.PriceCents * item.Quantity
		validatedItems = append(validatedItems, struct {
			ticketType *models.TicketType
			quantity   int
		}{tt, item.Quantity})
	}

	// 2. Réserver les tickets (avec lock pessimiste)
	err := s.ticketRepo.ReserveTickets(ctx, req.Items)
	if err != nil {
		return nil, fmt.Errorf("erreur réservation: %w", err)
	}

	// 3. Créer la commande en DB (dans une transaction)
	tx, err := s.orderRepo.BeginTx(ctx)
	if err != nil {
		s.ticketRepo.ReleaseTickets(ctx, req.Items)
		return nil, fmt.Errorf("erreur transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	order := &models.Order{
		CustomerEmail:     req.CustomerEmail,
		CustomerFirstName: req.CustomerFirstName,
		CustomerLastName:  req.CustomerLastName,
		CustomerPhone:     req.CustomerPhone,
		TotalCents:        totalCents,
		Status:            models.OrderStatusPending,
		IPAddress:         ipAddress,
		UserAgent:         userAgent,
	}

	if err := s.orderRepo.CreateOrder(ctx, tx, order); err != nil {
		s.ticketRepo.ReleaseTickets(ctx, req.Items)
		return nil, fmt.Errorf("erreur création commande: %w", err)
	}

	if err := s.orderRepo.SaveOrderItems(ctx, tx, order.ID, req.Items); err != nil {
		s.ticketRepo.ReleaseTickets(ctx, req.Items)
		return nil, fmt.Errorf("erreur sauvegarde items commande: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		s.ticketRepo.ReleaseTickets(ctx, req.Items)
		return nil, fmt.Errorf("erreur commit: %w", err)
	}

	// 4. Créer le checkout HelloAsso
	itemNames := make([]string, 0)
	for _, vi := range validatedItems {
		itemNames = append(itemNames, fmt.Sprintf("%dx %s", vi.quantity, vi.ticketType.Name))
	}

	haReq := CheckoutIntentRequest{
		TotalAmount:      totalCents,
		InitialAmount:    totalCents,
		ItemName:         fmt.Sprintf("%s - %s", s.cfg.FestivalName, joinStrings(itemNames)),
		BackURL:          s.cfg.HelloAssoErrorURL,
		ErrorURL:         s.cfg.HelloAssoErrorURL,
		ReturnURL:        fmt.Sprintf("%s?order_id=%s", s.cfg.HelloAssoReturnURL, order.ID),
		ContainsDonation: false,
		Payer: CheckoutPayer{
			FirstName: req.CustomerFirstName,
			LastName:  req.CustomerLastName,
			Email:     req.CustomerEmail,
		},
		Metadata: map[string]string{
			"order_id":     order.ID,
			"order_number": order.OrderNumber,
		},
	}

	payResp, err := s.paymentProvider.CreateCheckoutIntent(ctx, haReq)
	if err != nil {
		// Annuler la réservation
		s.ticketRepo.ReleaseTickets(ctx, req.Items)
		s.orderRepo.UpdateOrderStatus(ctx, order.ID, models.OrderStatusCancelled)
		return nil, fmt.Errorf("erreur %s checkout: %w", s.paymentProvider.Name(), err)
	}

	// 5. Mettre à jour la commande avec les infos checkout
	checkoutID := payResp.ExternalID
	if checkoutID == "" {
		checkoutID = fmt.Sprintf("%d", payResp.ID)
	}
	if err := s.orderRepo.UpdateOrderHelloAsso(ctx, order.ID, checkoutID, payResp.RedirectURL); err != nil {
		log.Printf("WARN: erreur mise à jour checkout info: %v", err)
	}

	// 6. Stocker les items en Redis pour le traitement webhook
	orderItems, _ := json.Marshal(req.Items)
	s.redis.Set(ctx, fmt.Sprintf("order:%s:items", order.ID), orderItems, 24*time.Hour)

	// Invalider le cache des ticket types
	s.redis.Del(ctx, "ticket_types:active")

	// 7. Si le provider auto-confirme (mock), traiter immédiatement
	if s.paymentProvider.AutoConfirms() {
		log.Printf("🧪 [MOCK] Auto-confirmation commande %s", order.OrderNumber)
		mockPayload := WebhookPaymentData{
			ID:       payResp.ID,
			Amount:   totalCents,
			State:    "Authorized",
			Metadata: map[string]string{"order_id": order.ID, "order_number": order.OrderNumber},
		}
		if err := s.ProcessPaymentWebhook(ctx, mockPayload, order.ID); err != nil {
			log.Printf("ERROR: auto-confirmation échouée: %v", err)
			// On ne retourne pas l'erreur — la commande est créée, on peut retenter
		}
	}

	return &models.CheckoutResponse{
		OrderID:     order.ID,
		OrderNumber: order.OrderNumber,
		CheckoutURL: payResp.RedirectURL,
		TotalCents:  totalCents,
	}, nil
}

func (s *TicketService) GetBusOptions(ctx context.Context) (*models.BusOptionsResponse, error) {
	stations, err := s.ticketRepo.GetBusStations(ctx)
	if err != nil {
		return nil, err
	}

	outbound, err := s.ticketRepo.GetBusDepartures(ctx, "to_festival")
	if err != nil {
		return nil, err
	}

	retour, err := s.ticketRepo.GetBusDepartures(ctx, "from_festival")
	if err != nil {
		return nil, err
	}

	return &models.BusOptionsResponse{
		Stations:           stations,
		OutboundDepartures: outbound,
		ReturnDepartures:   retour,
	}, nil
}

func (s *TicketService) CreateBusCheckout(ctx context.Context, req models.BusCheckoutRequest, ipAddress, userAgent string) (*models.CheckoutResponse, error) {
	if req.CustomerEmail == "" || req.CustomerFirstName == "" || req.CustomerLastName == "" {
		return nil, fmt.Errorf("email, prénom et nom sont requis")
	}
	if req.FromStationID == "" || req.OutboundDepartureID == "" {
		return nil, fmt.Errorf("station de départ et horaire aller requis")
	}

	stations, err := s.ticketRepo.GetBusStations(ctx)
	if err != nil {
		return nil, fmt.Errorf("erreur chargement stations: %w", err)
	}
	stationByID := make(map[string]models.BusStation, len(stations))
	for _, st := range stations {
		stationByID[st.ID] = st
	}

	fromStation, ok := stationByID[req.FromStationID]
	if !ok || !fromStation.IsActive {
		return nil, fmt.Errorf("station de départ invalide")
	}

	outbound, err := s.ticketRepo.GetBusDepartureByID(ctx, req.OutboundDepartureID)
	if err != nil {
		return nil, fmt.Errorf("erreur récupération horaire aller: %w", err)
	}
	if outbound == nil || outbound.Direction != "to_festival" || !outbound.IsActive {
		return nil, fmt.Errorf("horaire aller invalide")
	}
	if outbound.StationID != req.FromStationID {
		return nil, fmt.Errorf("l'horaire aller ne correspond pas à la station sélectionnée")
	}

	totalCents := outbound.PriceCents

	var returnDeparture *models.BusDeparture
	var returnStation models.BusStation
	if req.RoundTrip {
		if req.ReturnDepartureID == "" || req.ReturnStationID == "" {
			return nil, fmt.Errorf("horaire retour et station de dépose requis")
		}
		ret, err := s.ticketRepo.GetBusDepartureByID(ctx, req.ReturnDepartureID)
		if err != nil {
			return nil, fmt.Errorf("erreur récupération horaire retour: %w", err)
		}
		if ret == nil || ret.Direction != "from_festival" || !ret.IsActive {
			return nil, fmt.Errorf("horaire retour invalide")
		}
		returnDeparture = ret

		st, ok := stationByID[req.ReturnStationID]
		if !ok || !st.IsActive {
			return nil, fmt.Errorf("station de dépose invalide")
		}
		returnStation = st

		totalCents += returnDeparture.PriceCents
	}

	tx, err := s.orderRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("erreur transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.ticketRepo.ReserveBusDepartureSeat(ctx, tx, outbound.ID); err != nil {
		return nil, err
	}

	if returnDeparture != nil {
		if err := s.ticketRepo.ReserveBusDepartureSeat(ctx, tx, returnDeparture.ID); err != nil {
			return nil, err
		}
	}

	order := &models.Order{
		CustomerEmail:     req.CustomerEmail,
		CustomerFirstName: req.CustomerFirstName,
		CustomerLastName:  req.CustomerLastName,
		CustomerPhone:     req.CustomerPhone,
		TotalCents:        totalCents,
		Status:            models.OrderStatusPending,
		IPAddress:         ipAddress,
		UserAgent:         userAgent,
	}

	if err := s.orderRepo.CreateOrder(ctx, tx, order); err != nil {
		return nil, fmt.Errorf("erreur création commande bus: %w", err)
	}

	if err := s.ticketRepo.SaveBusOrderRide(ctx, tx, order.ID, outbound.ID, "outbound", fromStation.Name, "Festival"); err != nil {
		return nil, err
	}

	if returnDeparture != nil {
		if err := s.ticketRepo.SaveBusOrderRide(ctx, tx, order.ID, returnDeparture.ID, "return", "Festival", returnStation.Name); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("erreur commit commande bus: %w", err)
	}

	rideLabel := fmt.Sprintf("Navette %s → Festival", fromStation.Name)
	if returnDeparture != nil {
		rideLabel += fmt.Sprintf(" + Retour Festival → %s", returnStation.Name)
	}

	payReq := CheckoutIntentRequest{
		TotalAmount:      totalCents,
		InitialAmount:    totalCents,
		ItemName:         rideLabel,
		BackURL:          s.cfg.HelloAssoErrorURL,
		ErrorURL:         s.cfg.HelloAssoErrorURL,
		ReturnURL:        fmt.Sprintf("%s?order_id=%s", s.cfg.HelloAssoReturnURL, order.ID),
		ContainsDonation: false,
		Payer: CheckoutPayer{
			FirstName: req.CustomerFirstName,
			LastName:  req.CustomerLastName,
			Email:     req.CustomerEmail,
		},
		Metadata: map[string]string{
			"order_id":     order.ID,
			"order_number": order.OrderNumber,
			"order_kind":   "bus",
			"payer_phone":  strings.TrimSpace(req.CustomerPhone),
		},
	}

	payResp, err := s.paymentProvider.CreateCheckoutIntent(ctx, payReq)
	if err != nil {
		_ = s.ticketRepo.ReleaseBusOrderRides(ctx, order.ID)
		_ = s.orderRepo.UpdateOrderStatus(ctx, order.ID, models.OrderStatusCancelled)
		return nil, fmt.Errorf("erreur %s checkout bus: %w", s.paymentProvider.Name(), err)
	}

	checkoutID := payResp.ExternalID
	if checkoutID == "" {
		checkoutID = fmt.Sprintf("%d", payResp.ID)
	}
	if err := s.orderRepo.UpdateOrderHelloAsso(ctx, order.ID, checkoutID, payResp.RedirectURL); err != nil {
		log.Printf("WARN: erreur mise à jour checkout bus: %v", err)
	}

	if s.paymentProvider.AutoConfirms() {
		mockPayload := WebhookPaymentData{ID: payResp.ID, Amount: totalCents, State: "Authorized"}
		if err := s.ProcessPaymentWebhook(ctx, mockPayload, order.ID); err != nil {
			log.Printf("ERROR: auto-confirmation bus échouée: %v", err)
		}
	}

	return &models.CheckoutResponse{
		OrderID:     order.ID,
		OrderNumber: order.OrderNumber,
		CheckoutURL: payResp.RedirectURL,
		TotalCents:  totalCents,
	}, nil
}

func (s *TicketService) CreateBusStation(ctx context.Context, req models.CreateBusStationRequest) (*models.BusStation, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("nom de station requis")
	}
	return s.ticketRepo.CreateBusStation(ctx, name)
}

func (s *TicketService) CreateBusDeparture(ctx context.Context, req models.CreateBusDepartureRequest) (*models.BusDeparture, error) {
	if req.StationID == "" {
		return nil, fmt.Errorf("station requise")
	}
	if req.Direction != "to_festival" && req.Direction != "from_festival" {
		return nil, fmt.Errorf("direction invalide")
	}
	if req.Capacity < 1 {
		return nil, fmt.Errorf("capacité invalide")
	}
	if req.PriceCents < 0 {
		return nil, fmt.Errorf("prix invalide")
	}
	return s.ticketRepo.CreateBusDeparture(ctx, req)
}

func (s *TicketService) ListBusTicketsAdmin(ctx context.Context) ([]models.BusTicketAdminRow, error) {
	return s.ticketRepo.ListBusTickets(ctx, 300)
}

// ProcessPaymentWebhook traite le webhook de confirmation de paiement HelloAsso
func (s *TicketService) ProcessPaymentWebhook(ctx context.Context, payload WebhookPaymentData, orderID string) error {
	paymentID := fmt.Sprintf("%d", payload.ID)
	return s.ProcessOrderPaymentConfirmed(ctx, orderID, paymentID)
}

func (s *TicketService) ProcessOrderPaymentConfirmed(ctx context.Context, orderID string, paymentID string) error {
	// 1. Récupérer la commande
	order, err := s.orderRepo.GetOrderByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("erreur récupération commande: %w", err)
	}
	if order == nil {
		return fmt.Errorf("commande %s introuvable", orderID)
	}

	if order.Status != models.OrderStatusPending {
		log.Printf("Commande %s déjà traitée (statut: %s)", orderID, order.Status)
		return nil
	}

	// 2. Mettre à jour le statut
	if paymentID != "" {
		if err := s.orderRepo.SetHelloAssoPaymentID(ctx, order.ID, paymentID); err != nil {
			log.Printf("WARN: erreur enregistrement payment ID: %v", err)
		}
	}

	if err := s.orderRepo.UpdateOrderStatus(ctx, order.ID, models.OrderStatusPaid); err != nil {
		return fmt.Errorf("erreur mise à jour statut: %w", err)
	}

	// 3. Récupérer les items de la commande (Redis cache puis fallback DB)
	var items []models.CheckoutItem
	itemsData, err := s.redis.Get(ctx, fmt.Sprintf("order:%s:items", order.ID)).Bytes()
	if err == nil {
		if unmarshalErr := json.Unmarshal(itemsData, &items); unmarshalErr != nil {
			items = nil
		}
	}

	if len(items) == 0 {
		items, err = s.orderRepo.GetOrderItems(ctx, order.ID)
		if err != nil {
			return fmt.Errorf("erreur récupération items depuis DB: %w", err)
		}
	}

	if len(items) == 0 {
		return s.processBusOrderPaymentConfirmed(ctx, order, paymentID)
	}

	// 4. Générer les tickets avec QR codes
	tx, err := s.ticketRepo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("erreur transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var emailTickets []TicketEmailData

	for _, item := range items {
		tt, err := s.ticketRepo.GetTicketTypeByID(ctx, item.TicketTypeID)
		if err != nil {
			return fmt.Errorf("erreur récupération type ticket: %w", err)
		}

		for i := 0; i < item.Quantity; i++ {
			qrToken, err := s.qrService.GenerateToken()
			if err != nil {
				return fmt.Errorf("erreur génération token QR: %w", err)
			}

			qrPNG, err := s.qrService.GenerateQRCode(qrToken)
			if err != nil {
				return fmt.Errorf("erreur génération QR code: %w", err)
			}

			ticket := &models.Ticket{
				OrderID:           order.ID,
				TicketTypeID:      item.TicketTypeID,
				QRToken:           qrToken,
				QRCodeData:        qrPNG,
				AttendeeFirstName: order.CustomerFirstName,
				AttendeeLastName:  order.CustomerLastName,
			}

			if err := s.ticketRepo.CreateTicket(ctx, tx, ticket); err != nil {
				return fmt.Errorf("erreur création ticket: %w", err)
			}

			s.redis.Set(ctx, fmt.Sprintf("qr:%s", qrToken), order.ID, 0)

			emailTickets = append(emailTickets, TicketEmailData{
				TicketTypeName: tt.Name,
				AttendeeName:   fmt.Sprintf("%s %s", order.CustomerFirstName, order.CustomerLastName),
				QRToken:        qrToken,
				QRCodePNG:      qrPNG,
			})
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("erreur commit tickets: %w", err)
	}

	if err := s.orderRepo.UpdateOrderStatus(ctx, order.ID, models.OrderStatusConfirmed); err != nil {
		log.Printf("WARN: erreur mise à jour statut confirmed: %v", err)
	}

	customerName := fmt.Sprintf("%s %s", order.CustomerFirstName, order.CustomerLastName)
	if err := s.emailService.SendTicketEmail(order.CustomerEmail, customerName, order.OrderNumber, emailTickets); err != nil {
		log.Printf("ERROR: erreur envoi email: %v", err)
	}

	s.redis.Del(ctx, "ticket_types:active")

	log.Printf("✅ Commande %s confirmée — %d tickets générés", order.OrderNumber, len(emailTickets))
	return nil
}

func (s *TicketService) CancelPendingOrder(ctx context.Context, orderID string) error {
	order, err := s.orderRepo.GetOrderByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("erreur récupération commande: %w", err)
	}
	if order == nil {
		return fmt.Errorf("commande introuvable")
	}
	if order.Status != models.OrderStatusPending {
		return nil
	}

	items, err := s.orderRepo.GetOrderItems(ctx, orderID)
	if err != nil {
		return fmt.Errorf("erreur récupération items commande: %w", err)
	}

	if len(items) > 0 {
		if err := s.ticketRepo.ReleaseTickets(ctx, items); err != nil {
			return fmt.Errorf("erreur libération stock: %w", err)
		}
	}

	if len(items) == 0 {
		if err := s.ticketRepo.ReleaseBusOrderRides(ctx, orderID); err != nil {
			return fmt.Errorf("erreur libération places navette: %w", err)
		}
	}

	if err := s.orderRepo.UpdateOrderStatus(ctx, orderID, models.OrderStatusCancelled); err != nil {
		return fmt.Errorf("erreur annulation commande: %w", err)
	}

	s.redis.Del(ctx, fmt.Sprintf("order:%s:items", orderID))
	s.redis.Del(ctx, "ticket_types:active")
	return nil
}

func (s *TicketService) HandleLydiaWebhook(ctx context.Context, event string, form url.Values) error {
	orderRef := strings.TrimSpace(form.Get("order_ref"))
	if orderRef == "" {
		return fmt.Errorf("order_ref manquant")
	}

	order, err := s.orderRepo.GetOrderByReference(ctx, orderRef)
	if err != nil {
		return fmt.Errorf("erreur résolution order_ref: %w", err)
	}
	if order == nil {
		return fmt.Errorf("commande introuvable pour order_ref=%s", orderRef)
	}
	orderID := order.ID

	if sig := form.Get("sig"); sig != "" && s.cfg.LydiaVendorPrivateToken != "" {
		if !s.verifyLydiaSignature(form, sig) {
			return fmt.Errorf("signature Lydia invalide")
		}
	}

	switch event {
	case "confirm":
		paymentID := form.Get("transaction_identifier")
		if paymentID == "" {
			paymentID = form.Get("request_uuid")
		}
		if paymentID == "" {
			paymentID = form.Get("request_id")
		}
		return s.ProcessOrderPaymentConfirmed(ctx, orderID, paymentID)
	case "cancel", "expire":
		return s.CancelPendingOrder(ctx, orderID)
	default:
		return nil
	}
}

func (s *TicketService) verifyLydiaSignature(form url.Values, providedSig string) bool {
	keys := make([]string, 0, len(form))
	for key := range form {
		if key == "sig" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys)+1)
	for _, key := range keys {
		parts = append(parts, key+"="+form.Get(key))
	}
	parts = append(parts, s.cfg.LydiaVendorPrivateToken)

	raw := strings.Join(parts[:len(parts)-1], "&") + "&" + parts[len(parts)-1]
	expected := fmt.Sprintf("%x", md5.Sum([]byte(raw)))

	return strings.EqualFold(expected, providedSig)
}

// GetOrderStatus retourne le statut d'une commande
func (s *TicketService) GetOrderStatus(ctx context.Context, orderID string) (*models.Order, error) {
	order, err := s.orderRepo.GetOrderByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, fmt.Errorf("commande introuvable")
	}

	// Charger les tickets si la commande est confirmée
	if order.Status == models.OrderStatusConfirmed || order.Status == models.OrderStatusPaid {
		tickets, err := s.ticketRepo.GetTicketsByOrderID(ctx, order.ID)
		if err != nil {
			log.Printf("WARN: erreur chargement tickets: %v", err)
		} else {
			order.Tickets = tickets
		}
	}

	return order, nil
}

// GetQRCodeImage returns the QR code PNG data for a given token
func (s *TicketService) GetQRCodeImage(ctx context.Context, qrToken string) ([]byte, error) {
	return s.ticketRepo.GetQRCodeDataByToken(ctx, qrToken)
}

// CleanupExpiredPendingOrders annule les commandes pending trop anciennes
// et libère leur stock réservé.
func (s *TicketService) CleanupExpiredPendingOrders(ctx context.Context, pendingTTL time.Duration) (int, error) {
	cutoff := time.Now().Add(-pendingTTL)
	orderIDs, err := s.orderRepo.GetExpiredPendingOrderIDs(ctx, cutoff)
	if err != nil {
		return 0, err
	}

	cancelled := 0
	for _, orderID := range orderIDs {
		order, err := s.orderRepo.GetOrderByID(ctx, orderID)
		if err != nil || order == nil || order.Status != models.OrderStatusPending {
			continue
		}

		items, err := s.orderRepo.GetOrderItems(ctx, orderID)
		if err != nil {
			log.Printf("WARN: impossible de récupérer les items de %s: %v", orderID, err)
			continue
		}

		if len(items) > 0 {
			if err := s.ticketRepo.ReleaseTickets(ctx, items); err != nil {
				log.Printf("WARN: impossible de libérer le stock pour %s: %v", orderID, err)
				continue
			}
		} else {
			if err := s.ticketRepo.ReleaseBusOrderRides(ctx, orderID); err != nil {
				log.Printf("WARN: impossible de libérer les places navette pour %s: %v", orderID, err)
				continue
			}
		}

		if err := s.orderRepo.UpdateOrderStatus(ctx, orderID, models.OrderStatusCancelled); err != nil {
			log.Printf("WARN: impossible d'annuler %s: %v", orderID, err)
			continue
		}

		s.redis.Del(ctx, fmt.Sprintf("order:%s:items", orderID))
		cancelled++
	}

	if cancelled > 0 {
		s.redis.Del(ctx, "ticket_types:active")
	}

	return cancelled, nil
}

func joinStrings(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}

func (s *TicketService) processBusOrderPaymentConfirmed(ctx context.Context, order *models.Order, paymentID string) error {
	rides, err := s.ticketRepo.GetBusOrderRides(ctx, order.ID)
	if err != nil {
		return fmt.Errorf("erreur récupération rides bus: %w", err)
	}
	if len(rides) == 0 {
		return fmt.Errorf("aucun item trouvé pour la commande")
	}

	outboundDepartureID := ""
	fromStation := ""
	toStation := "Festival"
	isRoundTrip := false
	var returnDepartureID *string
	returnTo := ""

	for _, ride := range rides {
		if ride["ride_kind"] == "outbound" {
			outboundDepartureID = ride["departure_id"]
			fromStation = ride["from_station"]
		} else if ride["ride_kind"] == "return" {
			id := ride["departure_id"]
			returnDepartureID = &id
			isRoundTrip = true
			returnTo = ride["to_station"]
		}
	}

	if outboundDepartureID == "" {
		return fmt.Errorf("ride aller introuvable")
	}

	tx, err := s.ticketRepo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("erreur transaction ticket bus: %w", err)
	}
	defer tx.Rollback(ctx)

	qrToken, err := s.qrService.GenerateToken()
	if err != nil {
		return fmt.Errorf("erreur génération token QR: %w", err)
	}

	qrPNG, err := s.qrService.GenerateQRCode(qrToken)
	if err != nil {
		return fmt.Errorf("erreur génération QR code: %w", err)
	}

	busLabel := "Navette"
	if isRoundTrip {
		busLabel = "Navette Aller-Retour"
	}

	busType, err := s.ticketRepo.EnsureBusTicketType(ctx, tx, busLabel)
	if err != nil {
		return fmt.Errorf("erreur type ticket bus: %w", err)
	}

	ticket := &models.Ticket{
		OrderID:           order.ID,
		TicketTypeID:      busType.ID,
		QRToken:           qrToken,
		QRCodeData:        qrPNG,
		AttendeeFirstName: order.CustomerFirstName,
		AttendeeLastName:  order.CustomerLastName,
	}

	if err := s.ticketRepo.CreateTicket(ctx, tx, ticket); err != nil {
		return fmt.Errorf("erreur création ticket bus: %w", err)
	}

	if isRoundTrip {
		toStation = returnTo
	}
	if err := s.ticketRepo.SaveBusTicketDetails(ctx, tx, ticket.ID, outboundDepartureID, returnDepartureID, fromStation, toStation, isRoundTrip); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("erreur commit ticket bus: %w", err)
	}

	if paymentID != "" {
		if err := s.orderRepo.SetHelloAssoPaymentID(ctx, order.ID, paymentID); err != nil {
			log.Printf("WARN: erreur enregistrement payment ID bus: %v", err)
		}
	}

	if err := s.orderRepo.UpdateOrderStatus(ctx, order.ID, models.OrderStatusConfirmed); err != nil {
		log.Printf("WARN: erreur mise à jour statut confirmed bus: %v", err)
	}

	customerName := fmt.Sprintf("%s %s", order.CustomerFirstName, order.CustomerLastName)
	details := fmt.Sprintf("%s → Festival", fromStation)
	if isRoundTrip {
		details += fmt.Sprintf(" · Retour Festival → %s", returnTo)
	}

	if err := s.emailService.SendTicketEmail(order.CustomerEmail, customerName, order.OrderNumber, []TicketEmailData{{
		TicketTypeName: busLabel,
		AttendeeName:   details,
		QRToken:        qrToken,
		QRCodePNG:      qrPNG,
	}}); err != nil {
		log.Printf("ERROR: erreur envoi email bus: %v", err)
	}

	log.Printf("✅ Commande bus %s confirmée — ticket généré", order.OrderNumber)
	return nil
}
