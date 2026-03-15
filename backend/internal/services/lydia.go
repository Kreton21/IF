package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/kreton/if-festival/internal/config"
)

type LydiaService struct {
	cfg        *config.Config
	httpClient *http.Client
}

func NewLydiaService(cfg *config.Config) *LydiaService {
	return &LydiaService{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type lydiaRequestDoResponse struct {
	Error      string `json:"error"`
	Message    string `json:"message"`
	RequestID  string `json:"request_id"`
	RequestUUID string `json:"request_uuid"`
	MobileURL  string `json:"mobile_url"`
}

func (s *LydiaService) CreateCheckoutIntent(ctx context.Context, req CheckoutIntentRequest) (*CheckoutIntentResponse, error) {
	if s.cfg.LydiaVendorToken == "" {
		return nil, fmt.Errorf("LYDIA_VENDOR_TOKEN manquant")
	}

	apiURL, err := url.Parse(s.cfg.LydiaAPIURL)
	if err != nil {
		return nil, fmt.Errorf("LYDIA_API_URL invalide: %w", err)
	}
	apiURL.Path = path.Join(apiURL.Path, "/api/request/do.json")

	confirmURL := strings.TrimRight(s.cfg.BaseURL, "/") + "/api/v1/webhooks/lydia?event=confirm"
	cancelURL := strings.TrimRight(s.cfg.BaseURL, "/") + "/api/v1/webhooks/lydia?event=cancel"
	expireURL := strings.TrimRight(s.cfg.BaseURL, "/") + "/api/v1/webhooks/lydia?event=expire"

	form := url.Values{}
	form.Set("amount", fmt.Sprintf("%.2f", float64(req.TotalAmount)/100.0))
	form.Set("currency", "EUR")
	form.Set("vendor_token", s.cfg.LydiaVendorToken)
	form.Set("type", "email")
	form.Set("recipient", req.Payer.Email)
	form.Set("message", req.ItemName)
	form.Set("order_ref", req.Metadata["order_id"])
	form.Set("payment_method", s.cfg.LydiaPaymentMethod)
	form.Set("confirm_url", confirmURL)
	form.Set("cancel_url", cancelURL)
	form.Set("expire_url", expireURL)
	form.Set("browser_success_url", req.ReturnURL)
	form.Set("browser_fail_url", req.ErrorURL)
	form.Set("end_mobile_url", req.ReturnURL)
	form.Set("expire_time", "1800")

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("erreur création requête Lydia: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("erreur appel Lydia: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("erreur Lydia (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var out lydiaRequestDoResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("erreur décodage réponse Lydia: %w", err)
	}

	if out.Error != "0" {
		if out.Message == "" {
			out.Message = "erreur Lydia"
		}
		return nil, fmt.Errorf("Lydia erreur %s: %s", out.Error, out.Message)
	}

	if out.MobileURL == "" {
		return nil, fmt.Errorf("Lydia n'a pas retourné d'URL de paiement")
	}

	externalID := out.RequestUUID
	if externalID == "" {
		externalID = out.RequestID
	}

	return &CheckoutIntentResponse{
		ID:         0,
		ExternalID: externalID,
		RedirectURL: out.MobileURL,
	}, nil
}

func (s *LydiaService) AutoConfirms() bool {
	return false
}

func (s *LydiaService) Name() string {
	return "lydia"
}

var _ PaymentProvider = (*LydiaService)(nil)
