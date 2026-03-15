package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/kreton/if-festival/internal/config"
)

// HelloAssoService gère l'intégration avec l'API HelloAsso v5
type HelloAssoService struct {
	cfg        *config.Config
	httpClient *http.Client

	// Token OAuth2
	mu          sync.RWMutex
	accessToken string
	tokenExpiry time.Time
}

func NewHelloAssoService(cfg *config.Config) *HelloAssoService {
	return &HelloAssoService{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ============================================
// OAuth2 Token Management
// ============================================

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

func (s *HelloAssoService) getAccessToken(ctx context.Context) (string, error) {
	s.mu.RLock()
	if s.accessToken != "" && time.Now().Before(s.tokenExpiry) {
		token := s.accessToken
		s.mu.RUnlock()
		return token, nil
	}
	s.mu.RUnlock()

	return s.refreshAccessToken(ctx)
}

func (s *HelloAssoService) refreshAccessToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Double check
	if s.accessToken != "" && time.Now().Before(s.tokenExpiry) {
		return s.accessToken, nil
	}

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", s.cfg.HelloAssoClientID)
	data.Set("client_secret", s.cfg.HelloAssoClientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST",
		s.cfg.HelloAssoAPIURL+"/oauth2/token",
		strings.NewReader(data.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf("erreur création requête token: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("erreur requête token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("erreur token HelloAsso (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("erreur décodage token: %w", err)
	}

	s.accessToken = tokenResp.AccessToken
	// Renouveler 1 minute avant l'expiration
	s.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn-60) * time.Second)

	return s.accessToken, nil
}

// ============================================
// Checkout Intent
// ============================================

type CheckoutIntentRequest struct {
	TotalAmount int                `json:"totalAmount"` // En centimes
	InitialAmount int              `json:"initialAmount"`
	ItemName    string             `json:"itemName"`
	BackURL     string             `json:"backUrl"`
	ErrorURL    string             `json:"errorUrl"`
	ReturnURL   string             `json:"returnUrl"`
	ContainsDonation bool          `json:"containsDonation"`
	Payer       CheckoutPayer      `json:"payer"`
	Metadata    map[string]string  `json:"metadata,omitempty"`
}

type CheckoutPayer struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
}

type CheckoutIntentResponse struct {
	ID          int    `json:"id"`
	ExternalID  string `json:"external_id,omitempty"`
	RedirectURL string `json:"redirectUrl"`
}

// CreateCheckoutIntent crée une intention de paiement sur HelloAsso
func (s *HelloAssoService) CreateCheckoutIntent(ctx context.Context, req CheckoutIntentRequest) (*CheckoutIntentResponse, error) {
	token, err := s.getAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("erreur obtention token: %w", err)
	}

	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("erreur marshalling: %w", err)
	}

	apiURL := fmt.Sprintf("%s/v5/organizations/%s/checkout-intents",
		s.cfg.HelloAssoAPIURL, s.cfg.HelloAssoOrgSlug)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("erreur création requête: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("erreur requête checkout: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("erreur HelloAsso checkout (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var checkoutResp CheckoutIntentResponse
	if err := json.Unmarshal(body, &checkoutResp); err != nil {
		return nil, fmt.Errorf("erreur décodage response: %w", err)
	}

	return &checkoutResp, nil
}

// ============================================
// Webhook Payload
// ============================================

type WebhookPayload struct {
	EventType string          `json:"eventType"`
	Data      json.RawMessage `json:"data"`
}

type WebhookPaymentData struct {
	ID     int    `json:"id"`
	Amount int    `json:"amount"`
	State  string `json:"state"`
	Order  struct {
		ID int `json:"id"`
	} `json:"order"`
	Payer struct {
		Email     string `json:"email"`
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
	} `json:"payer"`
	Items []struct {
		Name     string `json:"name"`
		Amount   int    `json:"amount"`
		Quantity int    `json:"quantity"`
	} `json:"items"`
	Meta struct {
		CreatedAt string `json:"createdAt"`
	} `json:"meta"`
	// Metadata qu'on a envoyé dans le checkout
	Metadata map[string]string `json:"metadata"`
}

// GetCheckoutIntent récupère le statut d'un checkout intent
func (s *HelloAssoService) GetCheckoutIntent(ctx context.Context, checkoutID string) (*CheckoutIntentResponse, error) {
	token, err := s.getAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("erreur obtention token: %w", err)
	}

	apiURL := fmt.Sprintf("%s/v5/organizations/%s/checkout-intents/%s",
		s.cfg.HelloAssoAPIURL, s.cfg.HelloAssoOrgSlug, checkoutID)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("erreur HelloAsso (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result CheckoutIntentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}
