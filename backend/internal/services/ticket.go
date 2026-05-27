package services

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kreton/if-festival/internal/config"
	"github.com/kreton/if-festival/internal/models"
	"github.com/kreton/if-festival/internal/repository"
	"github.com/redis/go-redis/v9"
)

// TicketService orchestre la logique métier d'achat de tickets
type TicketService struct {
	cfg             *config.Config
	ticketRepo      *repository.TicketRepository
	orderRepo       *repository.OrderRepository
	couponRepo      *repository.CouponRepository
	paymentProvider PaymentProvider
	qrService       *QRCodeService
	emailService    *EmailService
	redis           *redis.Client
}

func NewTicketService(
	cfg *config.Config,
	ticketRepo *repository.TicketRepository,
	orderRepo *repository.OrderRepository,
	couponRepo *repository.CouponRepository,
	paymentProvider PaymentProvider,
	qrService *QRCodeService,
	emailService *EmailService,
	redis *redis.Client,
) *TicketService {
	return &TicketService{
		cfg:             cfg,
		ticketRepo:      ticketRepo,
		orderRepo:       orderRepo,
		couponRepo:      couponRepo,
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

// ToggleCategoryCheckbox toggles checkbox mode for a category (one per ticket type max)
func (s *TicketService) ToggleCategoryCheckbox(ctx context.Context, categoryID string) (*models.TicketCategory, error) {
	cat, err := s.ticketRepo.ToggleCategoryCheckbox(ctx, categoryID)
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
	normalizedEmail := normalizeEmail(email)
	if normalizedEmail == "" {
		return nil, fmt.Errorf("email invalide")
	}

	allTypes, err := s.GetAvailableTicketTypes(ctx)
	if err != nil {
		return nil, err
	}

	var result []models.TicketTypeForEmail
	for _, tt := range allTypes {
		if !emailAllowedByRules(normalizedEmail, tt.AllowedDomains) {
			continue
		}

		// Get categories for this ticket type
		cats, err := s.ticketRepo.GetCategoriesByTicketType(ctx, tt.ID)
		if err != nil {
			log.Printf("Erreur chargement catégories pour %s: %v", tt.ID, err)
			continue
		}

		// Filter categories by email domain and masked status
		var filteredCats []models.CategoryForEmail
		totalCategoryRemaining := 0
		for _, cat := range cats {
			if cat.IsMasked {
				continue
			}
			if !emailAllowedByRules(normalizedEmail, cat.AllowedDomains) {
				continue
			}
			remaining := cat.QuantityAllocated - cat.QuantitySold
			if remaining < 0 {
				remaining = 0
			}
			totalCategoryRemaining += remaining
			filteredCats = append(filteredCats, models.CategoryForEmail{
				ID:          cat.ID,
				Name:        cat.Name,
				IsCheckbox:  cat.IsCheckbox,
				IsAvailable: remaining > 0,
			})
		}

		// Only include ticket type if it has matching categories or no categories at all
		if len(cats) == 0 || len(filteredCats) > 0 {
			effectiveMaxPerOrder := tt.MaxPerOrder
			if !tt.OneTicketPerEmail && effectiveMaxPerOrder < 2 {
				effectiveMaxPerOrder = 10
			}

			remaining := tt.QuantityTotal - tt.QuantitySold
			if len(cats) > 0 {
				remaining = totalCategoryRemaining
			}
			if remaining < 0 {
				remaining = 0
			}
			maxSelectable := effectiveMaxPerOrder
			if remaining < maxSelectable {
				maxSelectable = remaining
			}
			isAvailable := remaining > 0

			ttForEmail := models.TicketTypeForEmail{
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
				IsAvailable:       isAvailable,
				Categories:        filteredCats,
			}
			result = append(result, ttForEmail)
		}
	}

	return result, nil
}

func (s *TicketService) PreviewCoupon(ctx context.Context, req models.CouponPreviewRequest) (*models.CouponPreviewResponse, error) {
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	if code == "" {
		return &models.CouponPreviewResponse{Valid: false, Message: "Code requis"}, nil
	}
	if len(req.Items) == 0 {
		return &models.CouponPreviewResponse{Valid: false, Message: "Aucun ticket sélectionné"}, nil
	}

	coupon, err := s.couponRepo.GetCouponByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if coupon == nil {
		return &models.CouponPreviewResponse{Valid: false, Message: "Coupon introuvable"}, nil
	}
	if !coupon.IsActive {
		return &models.CouponPreviewResponse{Valid: false, Message: "Coupon désactivé"}, nil
	}

	remaining := coupon.MaxUses - coupon.UsedCount
	if remaining <= 0 {
		return &models.CouponPreviewResponse{Valid: false, Message: "Coupon épuisé"}, nil
	}

	eligibleQty := 0
	for _, item := range req.Items {
		if item.TicketTypeID == coupon.TicketTypeID && item.Quantity > 0 {
			eligibleQty += item.Quantity
		}
	}
	if eligibleQty <= 0 {
		return &models.CouponPreviewResponse{Valid: false, Message: "Coupon non applicable à ces tickets"}, nil
	}

	applied := eligibleQty
	if remaining < applied {
		applied = remaining
	}

	discount := applied * coupon.DiscountCents
	msg := "Coupon appliqué"
	if applied < eligibleQty {
		msg = fmt.Sprintf("Coupon appliqué sur %d billet(s)", applied)
	}

	return &models.CouponPreviewResponse{
		Valid:         true,
		Message:       msg,
		Code:          coupon.Code,
		TicketTypeID:  coupon.TicketTypeID,
		AppliedUses:   applied,
		RemainingUses: remaining - applied,
		DiscountCents: discount,
	}, nil
}

func (s *TicketService) applyCouponInTx(ctx context.Context, tx pgx.Tx, code string, items []models.CheckoutItem) (*models.Coupon, int, int, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil, 0, 0, nil
	}

	coupon, err := s.couponRepo.GetCouponByCodeForUpdate(ctx, tx, code)
	if err != nil {
		return nil, 0, 0, err
	}
	if coupon == nil {
		return nil, 0, 0, fmt.Errorf("coupon introuvable")
	}
	if !coupon.IsActive {
		return nil, 0, 0, fmt.Errorf("coupon désactivé")
	}

	remaining := coupon.MaxUses - coupon.UsedCount
	if remaining <= 0 {
		return nil, 0, 0, fmt.Errorf("coupon épuisé")
	}

	eligibleQty := 0
	for _, item := range items {
		if item.TicketTypeID == coupon.TicketTypeID && item.Quantity > 0 {
			eligibleQty += item.Quantity
		}
	}
	if eligibleQty <= 0 {
		return nil, 0, 0, fmt.Errorf("coupon non applicable à ces tickets")
	}

	applied := eligibleQty
	if remaining < applied {
		applied = remaining
	}

	discount := applied * coupon.DiscountCents
	return coupon, applied, discount, nil
}

// CreateCheckout crée une commande et redirige vers HelloAsso
func (s *TicketService) CreateCheckout(ctx context.Context, req models.CheckoutRequest, ipAddress, userAgent string) (*models.CheckoutResponse, error) {
	if !isAdultFromDate(req.DateOfBirth) {
		return nil, fmt.Errorf("réservé aux personnes de 18 ans et plus")
	}

	var referral *models.ReferralPublicInfo
	if strings.TrimSpace(req.ReferralCode) != "" {
		link, err := s.orderRepo.GetReferralLinkByCode(ctx, req.ReferralCode)
		if err != nil {
			log.Printf("WARN: erreur lookup parrainage %s: %v", req.ReferralCode, err)
		} else if link != nil && link.IsActive {
			referral = link
		}
	}

	// 1. Valider les items et calculer le total
	ticketTotalCents := 0
	var validatedItems []struct {
		ticketType *models.TicketType
		quantity   int
	}

	normalizedEmail := normalizeEmail(req.CustomerEmail)
	for idx := range req.Items {
		item := req.Items[idx]
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
		if !emailAllowedByRules(normalizedEmail, tt.AllowedDomains) {
			return nil, fmt.Errorf("ce ticket n'est pas accessible pour cette adresse email")
		}
		if tt.OneTicketPerEmail && item.Quantity != 1 {
			return nil, fmt.Errorf("le ticket '%s' est limité à 1 billet par email", tt.Name)
		}
		effectiveMaxPerOrder := tt.MaxPerOrder
		if !tt.OneTicketPerEmail && effectiveMaxPerOrder < 2 {
			effectiveMaxPerOrder = 10
		}
		if item.Quantity > effectiveMaxPerOrder {
			return nil, fmt.Errorf("maximum %d tickets '%s' par commande", effectiveMaxPerOrder, tt.Name)
		}

		normalizedAttendees := make([]models.CheckoutAttendee, 0, item.Quantity)
		if len(item.Attendees) > 0 {
			if len(item.Attendees) != item.Quantity {
				return nil, fmt.Errorf("les informations nominatives sont incomplètes pour le ticket '%s'", tt.Name)
			}
			for _, attendee := range item.Attendees {
				firstName := strings.TrimSpace(attendee.FirstName)
				lastName := strings.TrimSpace(attendee.LastName)
				email := strings.ToLower(strings.TrimSpace(attendee.Email))

				if firstName == "" {
					firstName = strings.TrimSpace(req.CustomerFirstName)
				}
				if lastName == "" {
					lastName = strings.TrimSpace(req.CustomerLastName)
				}
				if email == "" {
					email = strings.ToLower(strings.TrimSpace(req.CustomerEmail))
				}
				if !strings.Contains(email, "@") {
					return nil, fmt.Errorf("email participant invalide pour le ticket '%s'", tt.Name)
				}

				normalizedAttendees = append(normalizedAttendees, models.CheckoutAttendee{
					FirstName: firstName,
					LastName:  lastName,
					Email:     email,
				})
			}
		}

		for len(normalizedAttendees) < item.Quantity {
			normalizedAttendees = append(normalizedAttendees, models.CheckoutAttendee{
				FirstName: strings.TrimSpace(req.CustomerFirstName),
				LastName:  strings.TrimSpace(req.CustomerLastName),
				Email:     strings.ToLower(strings.TrimSpace(req.CustomerEmail)),
			})
		}

		if tt.OneTicketPerEmail {
			attendeeEmail := normalizedAttendees[0].Email
			alreadyOwned, err := s.orderRepo.CountFestivalTicketsByTypeAndEmail(ctx, item.TicketTypeID, attendeeEmail)
			if err != nil {
				return nil, err
			}
			if alreadyOwned > 0 {
				return nil, fmt.Errorf("le ticket '%s' est déjà acheté pour l'email %s", tt.Name, attendeeEmail)
			}
		}

		req.Items[idx].Attendees = normalizedAttendees

		ticketTotalCents += tt.PriceCents * item.Quantity
		validatedItems = append(validatedItems, struct {
			ticketType *models.TicketType
			quantity   int
		}{tt, item.Quantity})
	}

	insuranceCents := 0
	if req.WantsRefundInsurance {
		insuranceCents = 100
	}

	totalCents := ticketTotalCents + insuranceCents

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

	var coupon *models.Coupon
	var couponAppliedUses int
	var couponDiscountCents int

	if strings.TrimSpace(req.CouponCode) != "" {
		var discount int
		coupon, couponAppliedUses, discount, err = s.applyCouponInTx(ctx, tx, req.CouponCode, req.Items)
		if err != nil {
			s.ticketRepo.ReleaseTickets(ctx, req.Items)
			return nil, err
		}
		couponDiscountCents = discount
		if couponDiscountCents > ticketTotalCents {
			couponDiscountCents = ticketTotalCents
		}
		totalCents = ticketTotalCents + insuranceCents - couponDiscountCents
		if totalCents < 0 {
			totalCents = 0
		}
	}

	order := &models.Order{
		CustomerEmail:        req.CustomerEmail,
		CustomerFirstName:    req.CustomerFirstName,
		CustomerLastName:     req.CustomerLastName,
		CustomerPhone:        req.CustomerPhone,
		DateOfBirth:          req.DateOfBirth,
		WantsCamping:         req.WantsCamping,
		WantsRefundInsurance: req.WantsRefundInsurance,
		TotalCents:           totalCents,
		Status:               models.OrderStatusPending,
		IPAddress:            ipAddress,
		UserAgent:            userAgent,
		CouponID:             "",
		CouponCode:           "",
		CouponDiscountCents:  couponDiscountCents,
		CouponUsesApplied:    couponAppliedUses,
	}
	if coupon != nil {
		order.CouponID = coupon.ID
		order.CouponCode = coupon.Code
	}

	if err := s.orderRepo.CreateOrder(ctx, tx, order); err != nil {
		s.ticketRepo.ReleaseTickets(ctx, req.Items)
		return nil, fmt.Errorf("erreur création commande: %w", err)
	}

	if err := s.orderRepo.SaveOrderItems(ctx, tx, order.ID, req.Items); err != nil {
		s.ticketRepo.ReleaseTickets(ctx, req.Items)
		return nil, fmt.Errorf("erreur sauvegarde items commande: %w", err)
	}

	if coupon != nil && couponAppliedUses > 0 {
		if err := s.couponRepo.IncrementCouponUsage(ctx, tx, coupon.ID, couponAppliedUses); err != nil {
			s.ticketRepo.ReleaseTickets(ctx, req.Items)
			return nil, err
		}
		if err := s.couponRepo.InsertCouponRedemption(ctx, tx, coupon.ID, order.ID, couponAppliedUses); err != nil {
			s.ticketRepo.ReleaseTickets(ctx, req.Items)
			return nil, err
		}
	}

	if referral != nil {
		if err := s.orderRepo.AttachReferralConversion(ctx, tx, order.ID, referral.ID, req.ReferralVisitorID); err != nil {
			s.ticketRepo.ReleaseTickets(ctx, req.Items)
			return nil, fmt.Errorf("erreur association parrainage: %w", err)
		}
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

func (s *TicketService) GetReferralByCode(ctx context.Context, code string) (*models.ReferralPublicInfo, error) {
	if strings.TrimSpace(code) == "" {
		return nil, nil
	}
	return s.orderRepo.GetReferralLinkByCode(ctx, code)
}

func (s *TicketService) TrackReferralClick(ctx context.Context, referralLinkID, visitorID, ipAddress, userAgent string) error {
	return s.orderRepo.RecordReferralClick(ctx, referralLinkID, visitorID, ipAddress, userAgent)
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

	tripType := strings.TrimSpace(strings.ToLower(req.TripType))
	if tripType == "" {
		if req.RoundTrip {
			tripType = "round_trip"
		} else if req.OutboundDepartureID != "" {
			tripType = "outbound"
		} else if req.ReturnDepartureID != "" {
			tripType = "return"
		}
	}
	if tripType != "outbound" && tripType != "return" && tripType != "round_trip" {
		return nil, fmt.Errorf("type de trajet invalide")
	}

	requireOutbound := tripType == "outbound" || tripType == "round_trip"
	requireReturn := tripType == "return" || tripType == "round_trip"

	if requireOutbound && req.OutboundDepartureID == "" {
		return nil, fmt.Errorf("horaire aller requis")
	}
	if requireReturn && req.ReturnDepartureID == "" {
		return nil, fmt.Errorf("horaire retour requis")
	}
	if requireReturn && req.ReturnStationID == "" {
		return nil, fmt.Errorf("station de retour requise")
	}

	stations, err := s.ticketRepo.GetBusStations(ctx)
	if err != nil {
		return nil, fmt.Errorf("erreur chargement stations: %w", err)
	}
	stationByID := make(map[string]models.BusStation, len(stations))
	for _, st := range stations {
		stationByID[st.ID] = st
	}

	totalCents := 0

	var outbound *models.BusDeparture
	var outboundStation models.BusStation
	if requireOutbound {
		dep, depErr := s.ticketRepo.GetBusDepartureByID(ctx, req.OutboundDepartureID)
		if depErr != nil {
			return nil, fmt.Errorf("erreur récupération horaire aller: %w", depErr)
		}
		if dep == nil || dep.Direction != "to_festival" || !dep.IsActive {
			return nil, fmt.Errorf("horaire aller invalide")
		}
		st, ok := stationByID[dep.StationID]
		if !ok || !st.IsActive {
			return nil, fmt.Errorf("station de départ invalide")
		}
		if req.FromStationID != "" && req.FromStationID != dep.StationID {
			return nil, fmt.Errorf("l'horaire aller ne correspond pas à la station sélectionnée")
		}
		outbound = dep
		outboundStation = st
		totalCents += dep.PriceCents
	}

	var returnDeparture *models.BusDeparture
	var returnStation models.BusStation
	if requireReturn {
		dep, depErr := s.ticketRepo.GetBusDepartureByID(ctx, req.ReturnDepartureID)
		if depErr != nil {
			return nil, fmt.Errorf("erreur récupération horaire retour: %w", depErr)
		}
		if dep == nil || dep.Direction != "from_festival" || !dep.IsActive {
			return nil, fmt.Errorf("horaire retour invalide")
		}
		st, ok := stationByID[dep.StationID]
		if !ok || !st.IsActive {
			return nil, fmt.Errorf("station de retour invalide")
		}
		if req.ReturnStationID != dep.StationID {
			return nil, fmt.Errorf("l'horaire retour ne correspond pas à la station sélectionnée")
		}
		returnDeparture = dep
		returnStation = st
		totalCents += dep.PriceCents
	}

	tx, err := s.orderRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("erreur transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if outbound != nil {
		if err := s.ticketRepo.ReserveBusDepartureSeat(ctx, tx, outbound.ID); err != nil {
			return nil, err
		}
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

	if outbound != nil {
		if err := s.ticketRepo.SaveBusOrderRide(ctx, tx, order.ID, outbound.ID, "outbound", outboundStation.Name, "Festival"); err != nil {
			return nil, err
		}
	}

	if returnDeparture != nil {
		if err := s.ticketRepo.SaveBusOrderRide(ctx, tx, order.ID, returnDeparture.ID, "return", "Festival", returnStation.Name); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("erreur commit commande bus: %w", err)
	}

	rideLabel := "Navette"
	switch tripType {
	case "outbound":
		rideLabel = fmt.Sprintf("Navette %s → Festival", outboundStation.Name)
	case "return":
		rideLabel = fmt.Sprintf("Navette Festival → %s", returnStation.Name)
	case "round_trip":
		rideLabel = fmt.Sprintf("Navette %s → Festival + Retour Festival → %s", outboundStation.Name, returnStation.Name)
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
			"trip_type":    tripType,
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

func (s *TicketService) UpdateBusDeparture(ctx context.Context, id string, req models.UpdateBusDepartureRequest) (*models.BusDeparture, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("id départ navette manquant")
	}
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
	return s.ticketRepo.UpdateBusDeparture(ctx, id, req)
}

func (s *TicketService) ToggleBusDepartureMask(ctx context.Context, id string) (*models.BusDeparture, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("id départ navette manquant")
	}
	return s.ticketRepo.ToggleBusDepartureMask(ctx, id)
}

func (s *TicketService) DeleteBusDeparture(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("id départ navette manquant")
	}
	return s.ticketRepo.DeleteBusDeparture(ctx, id)
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

	// 2. Enregistrer l'identifiant de paiement
	if paymentID != "" {
		if err := s.orderRepo.SetHelloAssoPaymentID(ctx, order.ID, paymentID); err != nil {
			log.Printf("WARN: erreur enregistrement payment ID: %v", err)
		}
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

	// 4. Mettre à jour le statut (festival uniquement)
	if err := s.orderRepo.UpdateOrderStatus(ctx, order.ID, models.OrderStatusPaid); err != nil {
		return fmt.Errorf("erreur mise à jour statut: %w", err)
	}

	// 5. Générer les tickets avec QR codes
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

		attendees := item.Attendees
		if len(attendees) != item.Quantity {
			attendees = make([]models.CheckoutAttendee, 0, item.Quantity)
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

			attendee := models.CheckoutAttendee{}
			if i < len(attendees) {
				attendee = attendees[i]
			}

			firstName := strings.TrimSpace(attendee.FirstName)
			if firstName == "" {
				firstName = strings.TrimSpace(order.CustomerFirstName)
			}

			lastName := strings.TrimSpace(attendee.LastName)
			if lastName == "" {
				lastName = strings.TrimSpace(order.CustomerLastName)
			}

			attendeeEmail := strings.ToLower(strings.TrimSpace(attendee.Email))
			if attendeeEmail == "" {
				attendeeEmail = strings.ToLower(strings.TrimSpace(order.CustomerEmail))
			}

			attendeeName := strings.TrimSpace(fmt.Sprintf("%s %s", firstName, lastName))

			ticket := &models.Ticket{
				OrderID:           order.ID,
				TicketTypeID:      item.TicketTypeID,
				CategoryID:        item.CategoryID,
				QRToken:           qrToken,
				QRCodeData:        qrPNG,
				IsCamping:         order.WantsCamping,
				AttendeeFirstName: firstName,
				AttendeeLastName:  lastName,
				AttendeeEmail:     attendeeEmail,
			}

			if err := s.ticketRepo.CreateTicket(ctx, tx, ticket); err != nil {
				return fmt.Errorf("erreur création ticket: %w", err)
			}

			s.redis.Set(ctx, fmt.Sprintf("qr:%s", qrToken), order.ID, 0)

			emailTickets = append(emailTickets, TicketEmailData{
				TicketTypeName: tt.Name,
				AttendeeName:   attendeeName,
				DateOfBirth:    order.DateOfBirth,
				RecipientEmail: attendeeEmail,
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

	if err := s.dispatchFestivalTicketEmails(order, emailTickets); err != nil {
		log.Printf("ERROR: erreur envoi email: %v", err)
	}

	s.redis.Del(ctx, "ticket_types:active")

	log.Printf("✅ Commande %s confirmée — %d tickets générés", order.OrderNumber, len(emailTickets))
	return nil
}

func (s *TicketService) dispatchFestivalTicketEmails(order *models.Order, emailTickets []TicketEmailData) error {
	if len(emailTickets) == 0 {
		return nil
	}

	type recipientBatch struct {
		name    string
		tickets []TicketEmailData
	}

	batches := map[string]*recipientBatch{}
	for _, ticket := range emailTickets {
		email := strings.ToLower(strings.TrimSpace(ticket.RecipientEmail))
		if email == "" {
			email = strings.ToLower(strings.TrimSpace(order.CustomerEmail))
		}
		if email == "" {
			continue
		}

		batch, exists := batches[email]
		if !exists {
			name := strings.TrimSpace(ticket.AttendeeName)
			if name == "" {
				name = strings.TrimSpace(fmt.Sprintf("%s %s", order.CustomerFirstName, order.CustomerLastName))
			}
			batch = &recipientBatch{name: name}
			batches[email] = batch
		}

		batch.tickets = append(batch.tickets, ticket)
	}

	var firstErr error
	for email, batch := range batches {
		if err := s.emailService.SendTicketEmail(email, batch.name, order.OrderNumber, batch.tickets); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			log.Printf("WARN: échec envoi email ticket à %s: %v", email, err)
		}
	}

	return firstErr
}

func (s *TicketService) formatBusTime(t time.Time) string {
	busLocation, locErr := time.LoadLocation("Europe/Paris")
	if locErr != nil {
		busLocation = time.Local
	}
	return t.In(busLocation).Format("02/01 15:04")
}

func (s *TicketService) buildFestivalEmailTicketsForOrder(ctx context.Context, order *models.Order) ([]TicketEmailData, error) {
	tickets, err := s.ticketRepo.GetTicketsByOrderID(ctx, order.ID)
	if err != nil {
		return nil, fmt.Errorf("erreur récupération tickets commande: %w", err)
	}
	if len(tickets) == 0 {
		return nil, fmt.Errorf("aucun ticket pour la commande %s", order.OrderNumber)
	}

	emailTickets := make([]TicketEmailData, 0, len(tickets))
	for _, ticket := range tickets {
		qrPNG, qrErr := s.ticketRepo.GetQRCodeDataByToken(ctx, ticket.QRToken)
		if qrErr != nil || len(qrPNG) == 0 {
			qrPNG, qrErr = s.qrService.GenerateQRCode(ticket.QRToken)
			if qrErr != nil {
				log.Printf("WARN: QR indisponible pour ticket %s (%s): %v", ticket.ID, ticket.QRToken, qrErr)
				continue
			}
		}

		recipientEmail := strings.TrimSpace(ticket.AttendeeEmail)
		if recipientEmail == "" {
			recipientEmail = strings.TrimSpace(order.CustomerEmail)
		}

		emailTickets = append(emailTickets, TicketEmailData{
			TicketTypeName: ticket.TicketTypeName,
			AttendeeName:   strings.TrimSpace(strings.TrimSpace(ticket.AttendeeFirstName) + " " + strings.TrimSpace(ticket.AttendeeLastName)),
			DateOfBirth:    order.DateOfBirth,
			RecipientEmail: recipientEmail,
			QRToken:        ticket.QRToken,
			QRCodePNG:      qrPNG,
		})
	}

	if len(emailTickets) == 0 {
		return nil, fmt.Errorf("aucun ticket envoyable pour la commande %s", order.OrderNumber)
	}

	return emailTickets, nil
}

func (s *TicketService) buildBusEmailTicketsForOrder(ctx context.Context, order *models.Order) ([]TicketEmailData, error) {
	rows, err := s.ticketRepo.ListBusTicketsByOrderID(ctx, order.ID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("aucun ticket bus pour la commande %s", order.OrderNumber)
	}

	emailTickets := make([]TicketEmailData, 0, len(rows))
	for _, row := range rows {
		qrPNG, qrErr := s.ticketRepo.GetQRCodeDataByToken(ctx, row.QRToken)
		if qrErr != nil || len(qrPNG) == 0 {
			qrPNG, qrErr = s.qrService.GenerateQRCode(row.QRToken)
			if qrErr != nil {
				log.Printf("WARN: QR indisponible pour ticket bus (%s): %v", row.QRToken, qrErr)
				continue
			}
		}

		details := fmt.Sprintf("%s → %s", row.FromStation, row.ToStation)
		if row.IsRoundTrip {
			details += " · Aller-retour"
		}

		departureInfo := ""
		if !row.DepartureTime.IsZero() {
			departureInfo = fmt.Sprintf("Aller : %s", s.formatBusTime(row.DepartureTime))
		}
		if row.ReturnDepartureTime != nil {
			retInfo := fmt.Sprintf("Retour : %s", s.formatBusTime(*row.ReturnDepartureTime))
			if departureInfo != "" {
				departureInfo = departureInfo + " • " + retInfo
			} else {
				departureInfo = retInfo
			}
		}

		emailTickets = append(emailTickets, TicketEmailData{
			TicketTypeName: row.TicketTypeName,
			AttendeeName:   details,
			DepartureInfo:  departureInfo,
			QRToken:        row.QRToken,
			QRCodePNG:      qrPNG,
			IsBus:          true,
		})
	}

	if len(emailTickets) == 0 {
		return nil, fmt.Errorf("aucun ticket bus envoyable pour la commande %s", order.OrderNumber)
	}

	return emailTickets, nil
}

func (s *TicketService) ResendOrderConfirmationEmail(ctx context.Context, orderID string) error {
	order, err := s.orderRepo.GetOrderByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("erreur récupération commande %s: %w", orderID, err)
	}
	if order == nil {
		return fmt.Errorf("commande introuvable: %s", orderID)
	}
	if order.Status != models.OrderStatusPaid && order.Status != models.OrderStatusConfirmed {
		return fmt.Errorf("commande %s ignorée (statut %s)", order.OrderNumber, order.Status)
	}

	totalTickets, err := s.ticketRepo.CountTicketsByOrder(ctx, order.ID)
	if err != nil {
		return err
	}
	if totalTickets == 0 {
		return fmt.Errorf("aucun ticket pour la commande %s", order.OrderNumber)
	}

	busTickets, err := s.ticketRepo.CountBusTicketsByOrder(ctx, order.ID)
	if err != nil {
		return err
	}
	if busTickets > 0 && busTickets == totalTickets {
		emailTickets, err := s.buildBusEmailTicketsForOrder(ctx, order)
		if err != nil {
			return err
		}

		customerName := strings.TrimSpace(fmt.Sprintf("%s %s", order.CustomerFirstName, order.CustomerLastName))
		if err := s.emailService.SendBusTicketEmail(order.CustomerEmail, customerName, order.OrderNumber, emailTickets); err != nil {
			return fmt.Errorf("échec renvoi email bus commande %s: %w", order.OrderNumber, err)
		}

		return nil
	}

	emailTickets, err := s.buildFestivalEmailTicketsForOrder(ctx, order)
	if err != nil {
		return err
	}

	if err := s.dispatchFestivalTicketEmails(order, emailTickets); err != nil {
		return fmt.Errorf("échec renvoi emails commande %s: %w", order.OrderNumber, err)
	}

	return nil
}

func (s *TicketService) ResendAllConfirmationEmails(ctx context.Context) (int, int, error) {
	orderIDs, err := s.orderRepo.ListPaidConfirmedOrderIDsWithTickets(ctx)
	if err != nil {
		return 0, 0, err
	}

	sent := 0
	failed := 0
	for _, orderID := range orderIDs {
		if err := s.ResendOrderConfirmationEmail(ctx, orderID); err != nil {
			failed++
			log.Printf("WARN: renvoi email commande %s échoué: %v", orderID, err)
			continue
		}
		sent++
	}

	return sent, failed, nil
}

func (s *TicketService) ListOrderTickets(ctx context.Context, orderID string) ([]models.OrderTicketAdminRow, error) {
	if strings.TrimSpace(orderID) == "" {
		return nil, fmt.Errorf("id commande manquant")
	}
	return s.ticketRepo.ListOrderTickets(ctx, orderID)
}

func (s *TicketService) RefundSingleTicket(ctx context.Context, orderID, ticketID string) error {
	if strings.TrimSpace(ticketID) == "" {
		return fmt.Errorf("id ticket manquant")
	}

	info, err := s.ticketRepo.GetRefundTicketInfo(ctx, ticketID)
	if err != nil {
		return err
	}
	if info == nil {
		return fmt.Errorf("ticket introuvable")
	}
	if strings.TrimSpace(orderID) != "" && info.OrderID != orderID {
		return fmt.Errorf("ticket n'appartient pas à la commande")
	}
	if info.OrderStatus != models.OrderStatusPaid && info.OrderStatus != models.OrderStatusConfirmed {
		return fmt.Errorf("seules les commandes payées/confirmées sont remboursables")
	}
	if info.IsBus {
		return fmt.Errorf("remboursement par ticket non disponible pour les navettes")
	}
	if info.IsValidated {
		return fmt.Errorf("ticket déjà validé")
	}
	if info.IsRefunded {
		return fmt.Errorf("ticket déjà remboursé")
	}

	if s.paymentProvider == nil || s.paymentProvider.Name() != "lydia" {
		return fmt.Errorf("remboursement automatique disponible uniquement pour Lydia")
	}
	lydiaProvider, ok := s.paymentProvider.(*LydiaService)
	if !ok {
		return fmt.Errorf("provider Lydia indisponible")
	}

	refundCents := info.PriceCents - 100
	if refundCents <= 0 {
		return fmt.Errorf("montant à rembourser invalide (prix %d centimes)", info.PriceCents)
	}

	if err := lydiaProvider.RefundTransaction(ctx, LydiaRefundRequest{
		TransactionIdentifier: strings.TrimSpace(info.PaymentID),
		OrderRef:              info.OrderNumber,
		AmountCents:           refundCents,
		NotifyPayer:           true,
		NotifyCollecter:       true,
	}); err != nil {
		return fmt.Errorf("échec remboursement Lydia: %w", err)
	}

	if err := s.ticketRepo.MarkTicketRefunded(ctx, ticketID); err != nil {
		return err
	}

	s.redis.Del(ctx, "ticket_types:active")

	return nil
}

func (s *TicketService) CreateCompedOrder(ctx context.Context, req models.CreateCompedOrderRequest, ipAddress, userAgent string) (*models.Order, error) {
	qty := req.Quantity
	if qty < 1 {
		qty = 1
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		return nil, fmt.Errorf("email requis")
	}
	firstName := strings.TrimSpace(req.FirstName)
	lastName := strings.TrimSpace(req.LastName)
	if firstName == "" || lastName == "" {
		return nil, fmt.Errorf("prénom et nom requis")
	}

	tt, err := s.ticketRepo.GetTicketTypeByID(ctx, req.TicketTypeID)
	if err != nil {
		return nil, fmt.Errorf("erreur récupération type ticket: %w", err)
	}
	if tt == nil {
		return nil, fmt.Errorf("type de ticket introuvable")
	}

	if tt.OneTicketPerEmail && qty != 1 {
		return nil, fmt.Errorf("le ticket '%s' est limité à 1 billet par email", tt.Name)
	}
	effectiveMaxPerOrder := tt.MaxPerOrder
	if !tt.OneTicketPerEmail && effectiveMaxPerOrder < 2 {
		effectiveMaxPerOrder = 10
	}
	if qty > effectiveMaxPerOrder {
		return nil, fmt.Errorf("maximum %d tickets '%s' par commande", effectiveMaxPerOrder, tt.Name)
	}

	if strings.TrimSpace(req.CategoryID) != "" {
		cats, err := s.ticketRepo.GetCategoriesByTicketType(ctx, tt.ID)
		if err != nil {
			return nil, fmt.Errorf("erreur récupération catégories: %w", err)
		}
		var matched *models.TicketCategory
		for idx := range cats {
			if cats[idx].ID == req.CategoryID {
				matched = &cats[idx]
				break
			}
		}
		if matched == nil {
			return nil, fmt.Errorf("catégorie invalide pour ce ticket")
		}
		remaining := matched.QuantityAllocated - matched.QuantitySold
		if remaining < qty {
			return nil, fmt.Errorf("stock insuffisant dans cette catégorie")
		}
	}

	attendees := make([]models.CheckoutAttendee, 0, qty)
	for i := 0; i < qty; i++ {
		attendees = append(attendees, models.CheckoutAttendee{
			FirstName: firstName,
			LastName:  lastName,
			Email:     email,
		})
	}

	items := []models.CheckoutItem{
		{
			TicketTypeID: tt.ID,
			CategoryID:   strings.TrimSpace(req.CategoryID),
			Quantity:     qty,
			Attendees:    attendees,
		},
	}

	if err := s.ticketRepo.ReserveTickets(ctx, items); err != nil {
		return nil, fmt.Errorf("erreur réservation: %w", err)
	}

	tx, err := s.orderRepo.BeginTx(ctx)
	if err != nil {
		s.ticketRepo.ReleaseTickets(ctx, items)
		return nil, fmt.Errorf("erreur transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	order := &models.Order{
		CustomerEmail:     email,
		CustomerFirstName: firstName,
		CustomerLastName:  lastName,
		TotalCents:        0,
		Status:            models.OrderStatusConfirmed,
		IPAddress:         ipAddress,
		UserAgent:         userAgent,
	}

	if err := s.orderRepo.CreateOrder(ctx, tx, order); err != nil {
		s.ticketRepo.ReleaseTickets(ctx, items)
		return nil, fmt.Errorf("erreur création commande: %w", err)
	}

	if err := s.orderRepo.SaveOrderItems(ctx, tx, order.ID, items); err != nil {
		s.ticketRepo.ReleaseTickets(ctx, items)
		return nil, fmt.Errorf("erreur sauvegarde items: %w", err)
	}

	var emailTickets []TicketEmailData
	for _, item := range items {
		for i := 0; i < item.Quantity; i++ {
			qrToken, err := s.qrService.GenerateToken()
			if err != nil {
				s.ticketRepo.ReleaseTickets(ctx, items)
				return nil, fmt.Errorf("erreur génération token QR: %w", err)
			}
			qrPNG, err := s.qrService.GenerateQRCode(qrToken)
			if err != nil {
				s.ticketRepo.ReleaseTickets(ctx, items)
				return nil, fmt.Errorf("erreur génération QR code: %w", err)
			}

			attendee := item.Attendees[i]
			attendeeName := strings.TrimSpace(fmt.Sprintf("%s %s", attendee.FirstName, attendee.LastName))

			ticket := &models.Ticket{
				OrderID:           order.ID,
				TicketTypeID:      item.TicketTypeID,
				CategoryID:        item.CategoryID,
				QRToken:           qrToken,
				QRCodeData:        qrPNG,
				IsCamping:         false,
				AttendeeFirstName: attendee.FirstName,
				AttendeeLastName:  attendee.LastName,
				AttendeeEmail:     attendee.Email,
			}

			if err := s.ticketRepo.CreateTicket(ctx, tx, ticket); err != nil {
				s.ticketRepo.ReleaseTickets(ctx, items)
				return nil, fmt.Errorf("erreur création ticket: %w", err)
			}

			s.redis.Set(ctx, fmt.Sprintf("qr:%s", qrToken), order.ID, 0)

			emailTickets = append(emailTickets, TicketEmailData{
				TicketTypeName: tt.Name,
				AttendeeName:   attendeeName,
				DateOfBirth:    order.DateOfBirth,
				RecipientEmail: attendee.Email,
				QRToken:        qrToken,
				QRCodePNG:      qrPNG,
			})
		}
	}

	if err := tx.Commit(ctx); err != nil {
		s.ticketRepo.ReleaseTickets(ctx, items)
		return nil, fmt.Errorf("erreur commit tickets: %w", err)
	}

	if err := s.orderRepo.UpdateOrderStatus(ctx, order.ID, models.OrderStatusConfirmed); err != nil {
		log.Printf("WARN: erreur mise à jour statut confirmed: %v", err)
	}

	if err := s.dispatchFestivalTicketEmails(order, emailTickets); err != nil {
		log.Printf("ERROR: erreur envoi email: %v", err)
	}

	s.redis.Del(ctx, "ticket_types:active")

	return order, nil
}

func (s *TicketService) RefundOrderTotal(ctx context.Context, orderID string) error {
	order, err := s.orderRepo.GetOrderByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("erreur récupération commande %s: %w", orderID, err)
	}
	if order == nil {
		return fmt.Errorf("commande introuvable")
	}

	if order.Status != models.OrderStatusPaid && order.Status != models.OrderStatusConfirmed {
		return fmt.Errorf("seules les commandes payées/confirmées sont remboursables")
	}

	if s.paymentProvider == nil || s.paymentProvider.Name() != "lydia" {
		return fmt.Errorf("remboursement automatique disponible uniquement pour Lydia")
	}

	lydiaProvider, ok := s.paymentProvider.(*LydiaService)
	if !ok {
		return fmt.Errorf("provider Lydia indisponible")
	}

	items, err := s.orderRepo.GetOrderItems(ctx, orderID)
	if err != nil {
		return fmt.Errorf("erreur récupération items: %w", err)
	}
	if len(items) == 0 {
		return fmt.Errorf("navettes non remboursables")
	}

	ticketCount := 0
	for _, item := range items {
		ticketCount += item.Quantity
	}
	if ticketCount == 0 {
		count, err := s.ticketRepo.CountTicketsByOrder(ctx, orderID)
		if err != nil {
			return fmt.Errorf("erreur comptage tickets: %w", err)
		}
		ticketCount = count
	}
	if ticketCount <= 0 {
		return fmt.Errorf("aucun ticket à rembourser")
	}

	refundCents := order.TotalCents - (ticketCount * 100)
	if refundCents <= 0 {
		return fmt.Errorf("montant à rembourser invalide (total %d centimes)", order.TotalCents)
	}

	transactionIdentifier := strings.TrimSpace(order.HelloAssoPaymentID)
	if err := lydiaProvider.RefundTransaction(ctx, LydiaRefundRequest{
		TransactionIdentifier: transactionIdentifier,
		OrderRef:              order.OrderNumber,
		AmountCents:           refundCents,
		NotifyPayer:           true,
		NotifyCollecter:       true,
	}); err != nil {
		return fmt.Errorf("échec remboursement Lydia: %w", err)
	}

	if len(items) > 0 {
		if err := s.ticketRepo.ReleaseTickets(ctx, items); err != nil {
			return fmt.Errorf("remboursement effectué mais erreur libération stock: %w", err)
		}
	}

	if len(items) == 0 {
		if err := s.ticketRepo.ReleaseBusOrderRides(ctx, orderID); err != nil {
			return fmt.Errorf("remboursement effectué mais erreur libération places navette: %w", err)
		}
	}

	if err := s.orderRepo.UpdateOrderStatus(ctx, order.ID, models.OrderStatusRefunded); err != nil {
		return fmt.Errorf("remboursement effectué mais statut non mis à jour: %w", err)
	}

	s.redis.Del(ctx, fmt.Sprintf("order:%s:items", orderID))
	s.redis.Del(ctx, "ticket_types:active")

	return nil
}

// RemoveOrderLocal marks an order as refunded locally without contacting the payment provider.
func (s *TicketService) RemoveOrderLocal(ctx context.Context, orderID string) error {
	order, err := s.orderRepo.GetOrderByID(ctx, orderID)
	if err != nil {
		return fmt.Errorf("erreur récupération commande %s: %w", orderID, err)
	}
	if order == nil {
		return fmt.Errorf("commande introuvable")
	}

	if order.Status != models.OrderStatusPaid && order.Status != models.OrderStatusConfirmed {
		return fmt.Errorf("seules les commandes payées/confirmées sont remboursables")
	}

	items, err := s.orderRepo.GetOrderItems(ctx, orderID)
	if err != nil {
		return fmt.Errorf("erreur récupération items: %w", err)
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

	if err := s.orderRepo.UpdateOrderStatus(ctx, order.ID, models.OrderStatusRefunded); err != nil {
		return fmt.Errorf("statut non mis à jour: %w", err)
	}

	s.redis.Del(ctx, fmt.Sprintf("order:%s:items", orderID))
	s.redis.Del(ctx, "ticket_types:active")

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
		orderRef = strings.TrimSpace(form.Get("order_id"))
	}
	if orderRef == "" {
		orderRef = strings.TrimSpace(form.Get("order_number"))
	}

	var (
		order *models.Order
		err   error
	)

	if orderRef != "" {
		order, err = s.orderRepo.GetOrderByReference(ctx, orderRef)
		if err != nil {
			return fmt.Errorf("erreur résolution order_ref: %w", err)
		}
	}

	if order == nil {
		checkoutRef := strings.TrimSpace(form.Get("request_uuid"))
		if checkoutRef == "" {
			checkoutRef = strings.TrimSpace(form.Get("request_id"))
		}
		if checkoutRef == "" {
			checkoutRef = strings.TrimSpace(form.Get("transaction_identifier"))
		}
		if checkoutRef != "" {
			order, err = s.orderRepo.GetOrderByCheckoutID(ctx, checkoutRef)
			if err != nil {
				return fmt.Errorf("erreur résolution checkout Lydia: %w", err)
			}
		}
	}

	if order == nil {
		return fmt.Errorf("commande introuvable (order_ref=%s request_uuid=%s request_id=%s)",
			strings.TrimSpace(form.Get("order_ref")),
			strings.TrimSpace(form.Get("request_uuid")),
			strings.TrimSpace(form.Get("request_id")),
		)
	}
	orderID := order.ID

	if s.cfg.LydiaVendorPrivateToken != "" {
		sig := strings.TrimSpace(form.Get("sig"))
		if sig != "" && !s.verifyLydiaSignature(form, sig) {
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
// VerifyOrderForCIC confirms an order exists, belongs to that email, and is paid/confirmed.
func (s *TicketService) VerifyOrderForCIC(ctx context.Context, orderNumber, email string) (*models.Order, error) {
	return s.orderRepo.VerifyOrderByNumberAndEmail(ctx, orderNumber, email)
}

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

func (s *TicketService) ClaimCampingByEmail(ctx context.Context, email string) (*models.CampingClaimResponse, error) {
	trimmedEmail := strings.TrimSpace(strings.ToLower(email))
	if trimmedEmail == "" || !strings.Contains(trimmedEmail, "@") {
		return nil, fmt.Errorf("email invalide")
	}

	totalTickets, alreadyCamping, updatable, err := s.ticketRepo.GetCampingClaimStats(ctx, trimmedEmail)
	if err != nil {
		return nil, err
	}

	if totalTickets == 0 {
		return &models.CampingClaimResponse{
			UpdatedTickets: 0,
			Message:        "Si des billets éligibles existent, l'option camping sera activée.",
		}, nil
	}

	if updatable == 0 && alreadyCamping > 0 {
		return &models.CampingClaimResponse{
			UpdatedTickets: 0,
			Message:        "Si des billets éligibles existent, l'option camping sera activée.",
		}, nil
	}

	updated, err := s.ticketRepo.ClaimCampingByEmail(ctx, trimmedEmail)
	if err != nil {
		return nil, err
	}

	if updated == 0 {
		return &models.CampingClaimResponse{
			UpdatedTickets: 0,
			Message:        "Si des billets éligibles existent, l'option camping sera activée.",
		}, nil
	}

	return &models.CampingClaimResponse{
		UpdatedTickets: updated,
		Message:        "Si des billets éligibles existent, l'option camping sera activée.",
	}, nil
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

func normalizeEmail(email string) string {
	trimmed := strings.ToLower(strings.TrimSpace(email))
	parts := strings.SplitN(trimmed, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return trimmed
}

func emailAllowedByRules(email string, rules []string) bool {
	if len(rules) == 0 {
		return true
	}

	normalizedEmail := normalizeEmail(email)
	if normalizedEmail == "" {
		return false
	}
	parts := strings.SplitN(normalizedEmail, "@", 2)
	localPart := parts[0]
	domain := parts[1]

	for _, rawRule := range rules {
		rule := strings.ToLower(strings.TrimSpace(rawRule))
		if rule == "" {
			continue
		}

		if strings.HasPrefix(rule, "@") {
			ruleDomain := strings.TrimPrefix(rule, "@")
			if ruleDomain != "" && ruleDomain == domain {
				return true
			}
			continue
		}

		if strings.Contains(rule, "@") {
			ruleParts := strings.SplitN(rule, "@", 2)
			if len(ruleParts) == 2 {
				ruleLocal := strings.TrimSpace(ruleParts[0])
				ruleDomain := strings.TrimSpace(ruleParts[1])
				if ruleDomain == "" {
					continue
				}
				if ruleLocal == "" {
					if ruleDomain == domain {
						return true
					}
					continue
				}
				if ruleLocal == localPart && ruleDomain == domain {
					return true
				}
			}
			continue
		}

		if rule == domain {
			return true
		}
	}

	return false
}

func isAdultFromDate(dateOfBirth string) bool {
	trimmed := strings.TrimSpace(dateOfBirth)
	if trimmed == "" {
		return false
	}

	var dob time.Time
	var err error

	if strings.Contains(trimmed, "/") {
		dob, err = time.Parse("02/01/2006", trimmed)
	} else {
		dob, err = time.Parse("2006-01-02", trimmed)
	}

	if err != nil {
		return false
	}

	now := time.Now()
	if dob.After(now) {
		return false
	}

	age := now.Year() - dob.Year()
	birthdayThisYear := time.Date(now.Year(), dob.Month(), dob.Day(), 0, 0, 0, 0, now.Location())
	if now.Before(birthdayThisYear) {
		age--
	}

	return age >= 18
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
	hasOutbound := false
	hasReturn := false
	fromStation := ""
	toStation := ""
	isRoundTrip := false
	var returnDepartureID *string

	for _, ride := range rides {
		if ride["ride_kind"] == "outbound" {
			outboundDepartureID = ride["departure_id"]
			hasOutbound = true
			fromStation = ride["from_station"]
			toStation = ride["to_station"]
		} else if ride["ride_kind"] == "return" {
			hasReturn = true
			id := ride["departure_id"]
			if hasOutbound {
				returnDepartureID = &id
				isRoundTrip = true
				toStation = ride["to_station"]
			} else {
				outboundDepartureID = id
				fromStation = ride["from_station"]
				toStation = ride["to_station"]
			}
		}
	}

	if outboundDepartureID == "" {
		return fmt.Errorf("trajet navette introuvable")
	}

	departureInfo := ""
	if dep, depErr := s.ticketRepo.GetBusDepartureByID(ctx, outboundDepartureID); depErr == nil && dep != nil {
		departureInfo = fmt.Sprintf("Aller : %s", s.formatBusTime(dep.DepartureTime))
	}
	if returnDepartureID != nil {
		if ret, retErr := s.ticketRepo.GetBusDepartureByID(ctx, *returnDepartureID); retErr == nil && ret != nil {
			retInfo := fmt.Sprintf("Retour : %s", s.formatBusTime(ret.DepartureTime))
			if departureInfo != "" {
				departureInfo = departureInfo + " • " + retInfo
			} else {
				departureInfo = retInfo
			}
		}
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
	} else if hasReturn && !hasOutbound {
		busLabel = "Navette Retour"
	} else if hasOutbound {
		busLabel = "Navette Aller"
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

	if toStation == "" {
		toStation = "Festival"
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
	details := fmt.Sprintf("%s → %s", fromStation, toStation)
	if isRoundTrip {
		details += " · Aller-retour"
	}

	if err := s.emailService.SendBusTicketEmail(order.CustomerEmail, customerName, order.OrderNumber, []TicketEmailData{{
		TicketTypeName: busLabel,
		AttendeeName:   details,
		DepartureInfo:  departureInfo,
		QRToken:        qrToken,
		QRCodePNG:      qrPNG,
		IsBus:          true,
	}}); err != nil {
		log.Printf("ERROR: erreur envoi email bus: %v", err)
	}

	log.Printf("✅ Commande bus %s confirmée — ticket généré", order.OrderNumber)
	return nil
}
