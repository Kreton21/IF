package services

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
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

type LydiaRefundRequest struct {
	TransactionIdentifier string
	OrderRef              string
	AmountCents           int
	NotifyPayer           bool
	NotifyCollecter       bool
}

type lydiaRefundResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

var lydiaRefSanitizer = regexp.MustCompile(`[^A-Za-z0-9_-]`)
var lydiaPhoneSanitizer = regexp.MustCompile(`[^0-9+]`)

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
	orderID := strings.TrimSpace(req.Metadata["order_id"])
	orderNumber := strings.TrimSpace(req.Metadata["order_number"])
	confirmURL = appendLydiaCallbackRef(confirmURL, orderID, orderNumber)
	cancelURL = appendLydiaCallbackRef(cancelURL, orderID, orderNumber)
	expireURL = appendLydiaCallbackRef(expireURL, orderID, orderNumber)

	orderRef = sanitizeLydiaReference(orderRef)
	if orderRef == "" {
		orderRef = fmt.Sprintf("ORD-%d", time.Now().Unix())
	}

	message := sanitizeLydiaMessage(req.ItemName)
	recipientCandidates := buildLydiaRecipientCandidates(req)
	if len(recipientCandidates) == 0 {
		return nil, fmt.Errorf("destinataire Lydia invalide (email/téléphone manquant)")
	}

	var out lydiaRequestDoResponse
	var lastErr error

	for idx, cand := range recipientCandidates {
		out, lastErr = s.callLydiaRequestDo(ctx, apiURL.String(), lydiaRequestInput{
			amountCents:    req.TotalAmount,
			vendorToken:    s.cfg.LydiaVendorToken,
			recipientType:  cand.recipientType,
			recipient:      cand.recipient,
			message:        message,
			orderRef:       orderRef,
			paymentMethod:  normalizeLydiaPaymentMethod(s.cfg.LydiaPaymentMethod),
			confirmURL:     confirmURL,
			cancelURL:      cancelURL,
			expireURL:      expireURL,
			browserSuccess: req.ReturnURL,
			browserFail:    req.ErrorURL,
		})
		if lastErr == nil {
			break
		}
		if idx < len(recipientCandidates)-1 {
			if !isLydiaRecipientTypeError(lastErr) {
				break
			}
			if s.cfg.LydiaDebug {
				log.Printf("[LYDIA DEBUG] fallback recipient type after error: %v", lastErr)
			}
		}
	}

	if lastErr != nil {
		return nil, lastErr
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

type lydiaRecipientCandidate struct {
	recipientType string
	recipient     string
}

type lydiaRequestInput struct {
	amountCents    int
	vendorToken    string
	recipientType  string
	recipient      string
	message        string
	orderRef       string
	paymentMethod  string
	confirmURL     string
	cancelURL      string
	expireURL      string
	browserSuccess string
	browserFail    string
}

func buildLydiaRecipientCandidates(req CheckoutIntentRequest) []lydiaRecipientCandidate {
	candidates := make([]lydiaRecipientCandidate, 0, 2)

	email := strings.ToLower(strings.TrimSpace(req.Payer.Email))
	if looksLikeEmail(email) {
		candidates = append(candidates, lydiaRecipientCandidate{recipientType: "email", recipient: email})
	}

	phone := normalizeLydiaPhone(req.Metadata["payer_phone"])
	if phone != "" {
		candidates = append(candidates, lydiaRecipientCandidate{recipientType: "phone", recipient: phone})
	}

	return candidates
}

func normalizeLydiaPhone(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	v = lydiaPhoneSanitizer.ReplaceAllString(v, "")
	if strings.HasPrefix(v, "00") {
		v = "+" + strings.TrimPrefix(v, "00")
	}
	if strings.HasPrefix(v, "0") {
		v = "+33" + strings.TrimPrefix(v, "0")
	}
	if !strings.HasPrefix(v, "+") {
		return ""
	}
	if len(v) < 10 {
		return ""
	}
	return v
}

func looksLikeEmail(value string) bool {
	v := strings.TrimSpace(value)
	if v == "" {
		return false
	}
	parts := strings.Split(v, "@")
	if len(parts) != 2 {
		return false
	}
	if parts[0] == "" || parts[1] == "" {
		return false
	}
	return strings.Contains(parts[1], ".")
}

func isLydiaRecipientTypeError(err error) bool {
	if err == nil {
		return false
	}
	v := strings.ToLower(err.Error())
	if strings.Contains(v, "lydia erreur 3") {
		return true
	}
	if strings.Contains(v, "type de destinataire") {
		return true
	}
	if strings.Contains(v, "recipient") && strings.Contains(v, "type") {
		return true
	}
	return false
}

func (s *LydiaService) callLydiaRequestDo(ctx context.Context, apiURL string, in lydiaRequestInput) (lydiaRequestDoResponse, error) {
	form := url.Values{}
	form.Set("amount", fmt.Sprintf("%.2f", float64(in.amountCents)/100.0))
	form.Set("currency", "EUR")
	form.Set("vendor_token", in.vendorToken)
	form.Set("type", in.recipientType)
	form.Set("recipient", in.recipient)
	form.Set("message", in.message)
	form.Set("order_ref", in.orderRef)
	if in.paymentMethod != "" {
		form.Set("payment_method", in.paymentMethod)
	}
	form.Set("confirm_url", in.confirmURL)
	form.Set("cancel_url", in.cancelURL)
	form.Set("expire_url", in.expireURL)
	form.Set("browser_success_url", in.browserSuccess)
	form.Set("browser_fail_url", in.browserFail)
	form.Set("end_mobile_url", in.browserSuccess)
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
		log.Printf("[LYDIA DEBUG] callbacks confirm=%s cancel=%s expire=%s", in.confirmURL, in.cancelURL, in.expireURL)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return lydiaRequestDoResponse{}, fmt.Errorf("erreur création requête Lydia: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return lydiaRequestDoResponse{}, fmt.Errorf("erreur appel Lydia: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if s.cfg.LydiaDebug {
		log.Printf("[LYDIA DEBUG] request/do HTTP=%d body=%s", resp.StatusCode, string(body))
	}
	if resp.StatusCode != http.StatusOK {
		return lydiaRequestDoResponse{}, fmt.Errorf("erreur Lydia (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var out lydiaRequestDoResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return lydiaRequestDoResponse{}, fmt.Errorf("erreur décodage réponse Lydia: %w", err)
	}

	if out.Error != "0" {
		msg := out.Message
		if msg == "" {
			msg = "erreur Lydia"
		}
		return out, fmt.Errorf("Lydia erreur %s: %s", out.Error, msg)
	}

	return out, nil
}

func (s *LydiaService) AutoConfirms() bool {
	return false
}

func (s *LydiaService) Name() string {
	return "lydia"
}

func (s *LydiaService) RefundTransaction(ctx context.Context, req LydiaRefundRequest) error {
	if s.cfg.LydiaVendorToken == "" {
		return fmt.Errorf("LYDIA_VENDOR_TOKEN manquant")
	}
	if s.cfg.LydiaVendorPrivateToken == "" {
		return fmt.Errorf("LYDIA_VENDOR_PRIVATE_TOKEN manquant")
	}
	if req.AmountCents <= 0 {
		return fmt.Errorf("montant de remboursement invalide")
	}

	transactionIdentifier := strings.TrimSpace(req.TransactionIdentifier)
	orderRef := sanitizeLydiaReference(req.OrderRef)
	if transactionIdentifier == "" && orderRef == "" {
		return fmt.Errorf("transaction_identifier ou order_ref requis pour Lydia")
	}

	apiURL, err := url.Parse(s.cfg.LydiaAPIURL)
	if err != nil {
		return fmt.Errorf("LYDIA_API_URL invalide: %w", err)
	}
	apiURL.Path = path.Join(apiURL.Path, "/api/transaction/refund.json")

	targets := make([]url.Values, 0, 2)
	buildForm := func() url.Values {
		form := url.Values{}
		form.Set("vendor_token", s.cfg.LydiaVendorToken)
		form.Set("amount", fmt.Sprintf("%.2f", float64(req.AmountCents)/100.0))
		if req.NotifyPayer {
			form.Set("notify_payer", "yes")
		} else {
			form.Set("notify_payer", "no")
		}
		if req.NotifyCollecter {
			form.Set("notify_collecter", "yes")
		} else {
			form.Set("notify_collecter", "no")
		}
		return form
	}

	if transactionIdentifier != "" {
		form := buildForm()
		form.Set("transaction_identifier", transactionIdentifier)
		form.Set("signature", s.buildLydiaSignature(form, []string{"transaction_identifier", "order_ref", "amount"}))
		targets = append(targets, form)
	}
	if orderRef != "" {
		form := buildForm()
		form.Set("order_ref", orderRef)
		form.Set("signature", s.buildLydiaSignature(form, []string{"transaction_identifier", "order_ref", "amount"}))
		targets = append(targets, form)
	}

	var lastErr error
	for idx, form := range targets {
		if err := s.callLydiaRefund(ctx, apiURL.String(), form); err == nil {
			return nil
		} else {
			lastErr = err
			if s.cfg.LydiaDebug && idx < len(targets)-1 {
				log.Printf("[LYDIA DEBUG] refund fallback vers autre identifiant (%v)", err)
			}
		}
	}

	if lastErr != nil {
		return lastErr
	}

	return fmt.Errorf("aucune tentative de remboursement Lydia possible")
}

func (s *LydiaService) callLydiaRefund(ctx context.Context, apiURL string, form url.Values) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("erreur création requête Lydia refund: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("erreur appel Lydia refund: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if s.cfg.LydiaDebug {
		log.Printf("[LYDIA DEBUG] refund payload id=%s order_ref=%s amount=%s", form.Get("transaction_identifier"), form.Get("order_ref"), form.Get("amount"))
		log.Printf("[LYDIA DEBUG] refund HTTP=%d body=%s", resp.StatusCode, string(body))
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("erreur Lydia refund (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var out lydiaRefundResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("erreur décodage réponse Lydia refund: %w", err)
	}

	if out.Error != "" && out.Error != "0" {
		if out.Message == "" {
			out.Message = "erreur Lydia refund"
		}
		return fmt.Errorf("Lydia erreur %s: %s", out.Error, out.Message)
	}

	return nil
}

func (s *LydiaService) buildLydiaSignature(form url.Values, keysForSignature []string) string {
	allowed := make(map[string]struct{}, len(keysForSignature))
	for _, key := range keysForSignature {
		allowed[key] = struct{}{}
	}

	keys := make([]string, 0, len(keysForSignature))
	for key := range form {
		if _, ok := allowed[key]; !ok {
			continue
		}
		if strings.TrimSpace(form.Get(key)) == "" {
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
	return fmt.Sprintf("%x", md5.Sum([]byte(raw)))
}

func appendLydiaCallbackRef(baseURL, orderID, orderNumber string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}

	q := u.Query()
	if orderID != "" {
		q.Set("order_id", orderID)
	}
	if orderNumber != "" {
		q.Set("order_number", orderNumber)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

var _ PaymentProvider = (*LydiaService)(nil)
