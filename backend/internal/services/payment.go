package services

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
)

// PaymentProvider defines the interface for payment processing.
// Implementations: HelloAssoService (production), MockPaymentProvider (testing).
type PaymentProvider interface {
	// CreateCheckoutIntent creates a payment intent and returns a redirect URL
	CreateCheckoutIntent(ctx context.Context, req CheckoutIntentRequest) (*CheckoutIntentResponse, error)

	// AutoConfirms returns true if payment is instantly confirmed (e.g. mock mode)
	AutoConfirms() bool

	// Name returns the provider name for logging
	Name() string
}

// ============================================
// Mock Payment Provider (for testing)
// ============================================

// MockPaymentProvider simulates payment without any external API.
// It auto-confirms orders so that tickets are generated immediately.
type MockPaymentProvider struct {
	returnURL string
	counter   int64
}

func NewMockPaymentProvider(returnURL string) *MockPaymentProvider {
	return &MockPaymentProvider{
		returnURL: returnURL,
	}
}

func (m *MockPaymentProvider) CreateCheckoutIntent(ctx context.Context, req CheckoutIntentRequest) (*CheckoutIntentResponse, error) {
	id := atomic.AddInt64(&m.counter, 1)

	log.Printf("🧪 [MOCK PAYMENT] Paiement simulé #%d: %s (%d centimes)", id, req.ItemName, req.TotalAmount)
	log.Printf("🧪 [MOCK PAYMENT] Payeur: %s %s <%s>", req.Payer.FirstName, req.Payer.LastName, req.Payer.Email)
	log.Printf("🧪 [MOCK PAYMENT] Metadata: %v", req.Metadata)

	// Return the return URL directly — no payment page needed
	redirectURL := req.ReturnURL
	if redirectURL == "" {
		redirectURL = m.returnURL
	}

	return &CheckoutIntentResponse{
		ID:          int(id),
		RedirectURL: redirectURL,
	}, nil
}

func (m *MockPaymentProvider) AutoConfirms() bool {
	return true
}

func (m *MockPaymentProvider) Name() string {
	return "mock"
}

// Verify interface compliance at compile time
var _ PaymentProvider = (*MockPaymentProvider)(nil)
var _ PaymentProvider = (*HelloAssoService)(nil)
var _ PaymentProvider = (*LydiaService)(nil)

// ============================================
// HelloAssoService interface methods
// ============================================

// AutoConfirms returns false — HelloAsso requires webhook confirmation
func (s *HelloAssoService) AutoConfirms() bool {
	return false
}

// Name returns the provider name
func (s *HelloAssoService) Name() string {
	return "helloasso"
}

// ============================================
// Factory
// ============================================

// NewPaymentProvider creates the appropriate payment provider based on config
func NewPaymentProvider(providerName string, helloAssoService *HelloAssoService, lydiaService *LydiaService, returnURL string) (PaymentProvider, error) {
	switch providerName {
	case "mock":
		log.Println("🧪 Mode paiement MOCK activé — les commandes seront confirmées automatiquement")
		return NewMockPaymentProvider(returnURL), nil
	case "helloasso":
		if helloAssoService == nil {
			return nil, fmt.Errorf("HelloAsso service non configuré")
		}
		log.Println("💳 Mode paiement HelloAsso activé")
		return helloAssoService, nil
	case "lydia":
		if lydiaService == nil {
			return nil, fmt.Errorf("Lydia service non configuré")
		}
		log.Println("💳 Mode paiement Lydia activé")
		return lydiaService, nil
	default:
		return nil, fmt.Errorf("payment provider inconnu: %s (valeurs acceptées: mock, helloasso, lydia)", providerName)
	}
}
