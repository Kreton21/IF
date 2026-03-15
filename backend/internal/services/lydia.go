package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"regexp"
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

var lydiaRefSanitizer = regexp.MustCompile(`[^A-Za-z0-9_-]`)

func (s *LydiaService) CreateCheckoutIntent(ctx context.Context, req CheckoutIntentRequest) (*CheckoutIntentResponse, error) {
	if s.cfg.LydiaVendorToken == "" {
		return nil, fmt.Errorf("LYDIA_VENDOR_TOKEN manquant")
	}

	apiURL, err := url.Parse(s.cfg.LydiaAPIURL)
	if err != nil {
		return nil, fmt.Errorf("LYDIA_API_URL invalide: %w", err)
	}
	apiURL.Path = path.Join(apiURL.Path, "/api/request/do.json")

	confirmURL := strings.TrimRight(s.cfg.BaseURL, "/") + "/api/v1/webhooks/lydia/confirm"
	cancelURL := strings.TrimRight(s.cfg.BaseURL, "/") + "/api/v1/webhooks/lydia/cancel"
	expireURL := strings.TrimRight(s.cfg.BaseURL, "/") + "/api/v1/webhooks/lydia/expire"

	orderRef := req.Metadata["order_id"]
	if orderNumber := strings.TrimSpace(req.Metadata["order_number"]); orderNumber != "" {
		orderRef = orderNumber
	}
	orderRef = sanitizeLydiaReference(orderRef)
	if orderRef == "" {
		orderRef = fmt.Sprintf("ORD-%d", time.Now().Unix())
	}

	message := sanitizeLydiaMessage(req.ItemName)

	form := url.Values{}
	form.Set("amount", fmt.Sprintf("%.2f", float64(req.TotalAmount)/100.0))
	form.Set("currency", "EUR")
	form.Set("vendor_token", s.cfg.LydiaVendorToken)
	form.Set("type", "email")
	form.Set("recipient", req.Payer.Email)
	form.Set("message", message)
	form.Set("order_ref", orderRef)
	paymentMethod := normalizeLydiaPaymentMethod(s.cfg.LydiaPaymentMethod)
	if paymentMethod != "" {
		form.Set("payment_method", paymentMethod)
	}
	form.Set("confirm_url", confirmURL)
	form.Set("cancel_url", cancelURL)
	form.Set("expire_url", expireURL)
	form.Set("browser_success_url", req.ReturnURL)
	form.Set("browser_fail_url", req.ErrorURL)
	form.Set("end_mobile_url", req.ReturnURL)
	form.Set("expire_time", "1800")

	if s.cfg.LydiaDebug {
		log.Printf("[LYDIA DEBUG] request/do payload order_ref=%s amount=%s currency=%s type=%s recipient=%s payment_method=%s",
			form.Get("order_ref"),
			form.Get("amount"),
			form.Get("currency"),
			form.Get("type"),
			maskRecipient(form.Get("recipient")),
			form.Get("payment_method"),
		)
		log.Printf("[LYDIA DEBUG] callbacks confirm=%s cancel=%s expire=%s", confirmURL, cancelURL, expireURL)
	}

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
	if s.cfg.LydiaDebug {
		log.Printf("[LYDIA DEBUG] request/do HTTP=%d body=%s", resp.StatusCode, string(body))
	}
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

func maskRecipient(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	if strings.Contains(v, "@") {
		parts := strings.SplitN(v, "@", 2)
		local := parts[0]
		domain := parts[1]
		if len(local) > 2 {
			local = local[:2] + "***"
		} else {
			local = "***"
		}
		return local + "@" + domain
	}
	if len(v) <= 4 {
		return "***"
	}
	return strings.Repeat("*", len(v)-4) + v[len(v)-4:]
}

func sanitizeLydiaReference(value string) string {
	v := strings.TrimSpace(value)
	v = lydiaRefSanitizer.ReplaceAllString(v, "-")
	v = strings.Trim(v, "-_")
	if len(v) > 64 {
		v = v[:64]
	}
	return v
}

func sanitizeLydiaMessage(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return "Paiement billetterie"
	}
	v = strings.Join(strings.Fields(v), " ")
	if len(v) > 120 {
		v = v[:120]
	}
	return v
}

func normalizeLydiaPaymentMethod(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	switch v {
	case "":
		return ""
	case "cb", "lydia":
		return v
	default:
		return ""
	}
}

func (s *LydiaService) AutoConfirms() bool {
	return false
}

func (s *LydiaService) Name() string {
	return "lydia"
}

var _ PaymentProvider = (*LydiaService)(nil)
