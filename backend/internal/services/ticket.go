package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	checkoutID := fmt.Sprintf("%d", payResp.ID)
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

// ProcessPaymentWebhook traite le webhook de confirmation de paiement HelloAsso
func (s *TicketService) ProcessPaymentWebhook(ctx context.Context, payload WebhookPaymentData, orderID string) error {
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
	paymentID := fmt.Sprintf("%d", payload.ID)
	if err := s.orderRepo.SetHelloAssoPaymentID(ctx, order.ID, paymentID); err != nil {
		log.Printf("WARN: erreur enregistrement payment ID: %v", err)
	}

	if err := s.orderRepo.UpdateOrderStatus(ctx, order.ID, models.OrderStatusPaid); err != nil {
		return fmt.Errorf("erreur mise à jour statut: %w", err)
	}

	// 3. Récupérer les items de la commande depuis Redis
	var items []models.CheckoutItem
	itemsData, err := s.redis.Get(ctx, fmt.Sprintf("order:%s:items", order.ID)).Bytes()
	if err != nil {
		return fmt.Errorf("erreur récupération items: %w", err)
	}
	if err := json.Unmarshal(itemsData, &items); err != nil {
		return fmt.Errorf("erreur décodage items: %w", err)
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
			// Générer token QR unique
			qrToken, err := s.qrService.GenerateToken()
			if err != nil {
				return fmt.Errorf("erreur génération token QR: %w", err)
			}

			// Générer image QR
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

			// Stocker le QR token dans Redis pour validation rapide
			s.redis.Set(ctx, fmt.Sprintf("qr:%s", qrToken), order.ID, 0) // Pas d'expiration

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

	// 5. Mettre à jour le statut en "confirmed"
	if err := s.orderRepo.UpdateOrderStatus(ctx, order.ID, models.OrderStatusConfirmed); err != nil {
		log.Printf("WARN: erreur mise à jour statut confirmed: %v", err)
	}

	// 6. Envoyer l'email avec les tickets
	customerName := fmt.Sprintf("%s %s", order.CustomerFirstName, order.CustomerLastName)
	if err := s.emailService.SendTicketEmail(order.CustomerEmail, customerName, order.OrderNumber, emailTickets); err != nil {
		log.Printf("ERROR: erreur envoi email: %v", err)
		// Ne pas retourner l'erreur — le paiement est validé, on peut renvoyer l'email plus tard
	}

	// Invalider le cache
	s.redis.Del(ctx, "ticket_types:active")

	log.Printf("✅ Commande %s confirmée — %d tickets générés", order.OrderNumber, len(emailTickets))
	return nil
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
