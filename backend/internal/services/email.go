package services

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"html/template"
	"mime"
	"net/smtp"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/kreton/if-festival/internal/config"
)

// EmailService gère l'envoi d'emails aux clients
type EmailService struct {
	cfg *config.Config
}

func NewEmailService(cfg *config.Config) *EmailService {
	return &EmailService{cfg: cfg}
}

// SendTicketEmail envoie les tickets par email avec les QR codes en pièce jointe
func (s *EmailService) SendTicketEmail(to string, customerName string, orderNumber string, tickets []TicketEmailData) error {
	return s.SendFestivalTicketEmail(to, customerName, orderNumber, tickets)
}

func (s *EmailService) SendFestivalTicketEmail(to string, customerName string, orderNumber string, tickets []TicketEmailData) error {
	return s.sendTicketEmailWithTemplate(
		to,
		customerName,
		orderNumber,
		tickets,
		s.cfg.EmailTemplatePath,
		s.cfg.EmailSubjectTemplate,
		"festival",
	)
}

func (s *EmailService) SendBusTicketEmail(to string, customerName string, orderNumber string, tickets []TicketEmailData) error {
	return s.sendTicketEmailWithTemplate(
		to,
		customerName,
		orderNumber,
		tickets,
		s.cfg.BusEmailTemplatePath,
		s.cfg.BusEmailSubjectTemplate,
		"bus",
	)
}

func (s *EmailService) sendTicketEmailWithTemplate(
	to string,
	customerName string,
	orderNumber string,
	tickets []TicketEmailData,
	templatePath string,
	subjectTemplate string,
	emailKind string,
) error {
	if s.cfg.SMTPHost == "" {
		fmt.Printf("📧 [MOCK:%s] Email envoyé à %s pour commande %s (%d tickets)\n", emailKind, to, orderNumber, len(tickets))
		return nil
	}

	for i := range tickets {
		if strings.TrimSpace(tickets[i].CID) == "" {
			tickets[i].CID = fmt.Sprintf("qr-%d", i)
		}
	}

	subject, err := s.buildSubject(orderNumber, subjectTemplate)
	if err != nil {
		return fmt.Errorf("erreur génération sujet email: %w", err)
	}

	htmlBody, err := s.buildEmailHTML(to, customerName, orderNumber, tickets, templatePath)
	if err != nil {
		return fmt.Errorf("erreur génération HTML email: %w", err)
	}
	plainBody := buildPlainTextTicketEmail(s.cfg.FestivalName, customerName, orderNumber, tickets, s.cfg.SMTPFrom)

	attachments, err := s.buildPDFTicketAttachments(customerName, orderNumber, tickets)
	if err != nil {
		return fmt.Errorf("erreur génération PDF billets: %w", err)
	}

	if err := s.sendMIMEEmail(to, subject, plainBody, htmlBody, attachments); err != nil {
		return err
	}

	fmt.Printf("📧 Email confirmation %s envoyé à %s (commande %s)\n", emailKind, to, orderNumber)
	return nil
}

type TicketEmailData struct {
	TicketTypeName string
	AttendeeName   string
	RecipientEmail string
	QRToken        string
	QRCodePNG      []byte
	CID            string
}

func (s *EmailService) buildEmailHTML(customerEmail, customerName, orderNumber string, tickets []TicketEmailData, templatePath string) (string, error) {
	templateContent := defaultTicketEmailTemplate
	if path := strings.TrimSpace(templatePath); path != "" {
		if data, err := os.ReadFile(path); err == nil {
			templateContent = string(data)
		} else {
			fmt.Printf("WARN: impossible de lire template email=%s, fallback template interne (%v)\n", path, err)
		}
	}

	t, err := template.New("ticket").Parse(templateContent)
	if err != nil {
		return "", err
	}

	bannerDataURI := s.resolveTicketBannerDataURI()
	eventDateText := formatFrenchDate(s.cfg.FestivalDate)

	var buf bytes.Buffer
	err = t.Execute(&buf, map[string]interface{}{
		"FestivalName":  s.cfg.FestivalName,
		"FestivalDate":  s.cfg.FestivalDate,
		"EventDateText": eventDateText,
		"CustomerName":  customerName,
		"CustomerEmail": customerEmail,
		"OrderNumber":   orderNumber,
		"Tickets":       tickets,
		"SupportEmail":  s.cfg.SMTPFrom,
		"VenueName":     s.cfg.VenueName,
		"VenueAddress":  s.cfg.VenueAddress,
		"BannerDataURI": bannerDataURI,
	})
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

type EmailAttachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

func (s *EmailService) sendMIMEEmail(to, subject, plainBody, htmlBody string, attachments []EmailAttachment) error {
	mixedBoundary := "==FESTIVAL_MIXED_BOUNDARY=="
	altBoundary := "==FESTIVAL_ALT_BOUNDARY=="
	now := time.Now().UTC().Format(time.RFC1123Z)
	messageID := fmt.Sprintf("<if-%d@%s>", time.Now().UnixNano(), senderDomain(s.cfg.SMTPFrom))
	decodedFromName := html.UnescapeString(strings.TrimSpace(s.cfg.SMTPFromName))
	decodedSubject := html.UnescapeString(strings.TrimSpace(subject))
	encodedFromName := decodedFromName
	encodedSubject := decodedSubject
	if decodedFromName != "" {
		encodedFromName = mime.BEncoding.Encode("UTF-8", decodedFromName)
	}
	if decodedSubject != "" {
		encodedSubject = mime.BEncoding.Encode("UTF-8", decodedSubject)
	}

	var msg strings.Builder
	if encodedFromName != "" {
		msg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", encodedFromName, s.cfg.SMTPFrom))
	} else {
		msg.WriteString(fmt.Sprintf("From: %s\r\n", s.cfg.SMTPFrom))
	}
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", encodedSubject))
	msg.WriteString(fmt.Sprintf("Date: %s\r\n", now))
	msg.WriteString(fmt.Sprintf("Message-ID: %s\r\n", messageID))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n", mixedBoundary))
	msg.WriteString("\r\n")

	// multipart/alternative part
	msg.WriteString(fmt.Sprintf("--%s\r\n", mixedBoundary))
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", altBoundary))
	msg.WriteString("\r\n")

	// Plain text version
	msg.WriteString(fmt.Sprintf("--%s\r\n", altBoundary))
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(plainBody)
	msg.WriteString("\r\n")

	// HTML version
	msg.WriteString(fmt.Sprintf("--%s\r\n", altBoundary))
	msg.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)
	msg.WriteString("\r\n")
	msg.WriteString(fmt.Sprintf("--%s--\r\n", altBoundary))

	for _, attachment := range attachments {
		if len(attachment.Data) == 0 {
			continue
		}
		contentType := strings.TrimSpace(attachment.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		filename := strings.TrimSpace(attachment.Filename)
		if filename == "" {
			filename = "attachment.bin"
		}

		msg.WriteString(fmt.Sprintf("--%s\r\n", mixedBoundary))
		msg.WriteString(fmt.Sprintf("Content-Type: %s; name=\"%s\"\r\n", contentType, filename))
		msg.WriteString("Content-Transfer-Encoding: base64\r\n")
		msg.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n", filename))
		msg.WriteString("\r\n")
		msg.WriteString(encodeBase64RFC2045(attachment.Data))
		msg.WriteString("\r\n")
	}

	msg.WriteString(fmt.Sprintf("--%s--\r\n", mixedBoundary))

	auth := smtp.PlainAuth("", s.cfg.SMTPUser, s.cfg.SMTPPassword, s.cfg.SMTPHost)
	addr := fmt.Sprintf("%s:%d", s.cfg.SMTPHost, s.cfg.SMTPPort)

	return smtp.SendMail(addr, auth, s.cfg.SMTPFrom, []string{to}, []byte(msg.String()))
}

func (s *EmailService) buildPDFTicketAttachments(customerName, orderNumber string, tickets []TicketEmailData) ([]EmailAttachment, error) {
	attachments := make([]EmailAttachment, 0, len(tickets))

	for idx, ticket := range tickets {
		htmlContent, err := s.buildTicketPDFHTML(customerName, orderNumber, ticket)
		if err != nil {
			return nil, fmt.Errorf("erreur génération HTML PDF billet: %w", err)
		}

		pdfData, err := s.renderTicketPDF(htmlContent, ticket, idx)
		if err != nil {
			return nil, err
		}

		filename := fmt.Sprintf("ticket_%s_%d.pdf", sanitizeFilename(orderNumber), idx+1)
		attachments = append(attachments, EmailAttachment{
			Filename:    filename,
			ContentType: "application/pdf",
			Data:        pdfData,
		})
	}

	return attachments, nil
}

func (s *EmailService) renderTicketPDF(htmlContent string, ticket TicketEmailData, idx int) ([]byte, error) {
	engine := strings.ToLower(strings.TrimSpace(s.cfg.TicketPDFEngine))

	switch engine {
	case "", "auto":
		pdfData, err := s.renderTicketPDFWithWKHTMLToPDF(htmlContent)
		if err == nil {
			return pdfData, nil
		}
		fmt.Printf("WARN: wkhtmltopdf indisponible, fallback fpdf (%v)\n", err)
		return s.renderTicketPDFWithFPDF(htmlContent, ticket, idx)
	case "wkhtmltopdf":
		pdfData, err := s.renderTicketPDFWithWKHTMLToPDF(htmlContent)
		if err == nil {
			return pdfData, nil
		}
		if isWKHTMLRecoverableError(err) {
			fmt.Printf("WARN: wkhtmltopdf a échoué (fallback fpdf) (%v)\n", err)
			return s.renderTicketPDFWithFPDF(htmlContent, ticket, idx)
		}
		return nil, err
	case "fpdf":
		return s.renderTicketPDFWithFPDF(htmlContent, ticket, idx)
	default:
		return nil, fmt.Errorf("moteur PDF inconnu '%s' (attendu: auto, wkhtmltopdf, fpdf)", s.cfg.TicketPDFEngine)
	}
}

func isWKHTMLRecoverableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "contentnotfounderror") ||
		strings.Contains(msg, "protocolunknownerror") ||
		strings.Contains(msg, "failed to load") ||
		strings.Contains(msg, "blocked access to file")
}

func (s *EmailService) renderTicketPDFWithWKHTMLToPDF(htmlContent string) ([]byte, error) {
	bin, err := resolveWKHTMLToPDFBinary(strings.TrimSpace(s.cfg.WKHTMLTOPDFBin))
	if err != nil {
		return nil, err
	}

	htmlFile, err := os.CreateTemp("", "if-ticket-*.html")
	if err != nil {
		return nil, fmt.Errorf("impossible de créer un fichier temporaire HTML: %w", err)
	}
	htmlPath := htmlFile.Name()
	defer os.Remove(htmlPath)

	if _, err := htmlFile.WriteString(htmlContent); err != nil {
		_ = htmlFile.Close()
		return nil, fmt.Errorf("impossible d'écrire le HTML temporaire: %w", err)
	}
	if err := htmlFile.Close(); err != nil {
		return nil, fmt.Errorf("impossible de finaliser le HTML temporaire: %w", err)
	}

	cmd := exec.Command(
		bin,
		"--encoding", "utf-8",
		"--page-size", "A4",
		"--margin-top", "12",
		"--margin-right", "12",
		"--margin-bottom", "12",
		"--margin-left", "12",
		"--enable-local-file-access",
		"--load-error-handling", "ignore",
		"--load-media-error-handling", "ignore",
		htmlPath,
		"-",
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return nil, fmt.Errorf("échec wkhtmltopdf: %s", errMsg)
	}

	if stdout.Len() == 0 {
		return nil, fmt.Errorf("wkhtmltopdf a produit un PDF vide")
	}

	return stdout.Bytes(), nil
}

func (s *EmailService) renderTicketPDFWithFPDF(htmlContent string, ticket TicketEmailData, idx int) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.AddPage()

	pdf.SetFont("Arial", "", 11)
	html := pdf.HTMLBasicNew()
	html.Write(6.0, sanitizeHTMLForFPDF(htmlContent))

	if len(ticket.QRCodePNG) > 0 {
		pdf.Ln(6)
		imageKey := fmt.Sprintf("qr-%d", idx)
		pdf.RegisterImageOptionsReader(imageKey, fpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, bytes.NewReader(ticket.QRCodePNG))
		currentY := pdf.GetY()
		pdf.ImageOptions(imageKey, 55, currentY, 100, 100, false, fpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, 0, "")
		pdf.SetY(currentY + 106)
	}

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

func resolveWKHTMLToPDFBinary(configured string) (string, error) {
	seen := map[string]struct{}{}
	candidates := make([]string, 0, 8)

	if configured != "" {
		candidates = append(candidates, configured)
	}
	candidates = append(candidates, "wkhtmltopdf")

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates,
			filepath.Join(home, "wkhtmltopdf-bin", "wkhtmltopdf"),
			filepath.Join(home, "bin", "wkhtmltopdf"),
		)
	}

	candidates = append(candidates,
		"/usr/local/bin/wkhtmltopdf",
		"/usr/bin/wkhtmltopdf",
	)

	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}

		if strings.Contains(candidate, "/") {
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
				return candidate, nil
			}
			continue
		}

		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved, nil
		}
	}

	return "", fmt.Errorf("wkhtmltopdf introuvable (renseignez WKHTMLTOPDF_BIN ou ajoutez le binaire au PATH)")
}

func sanitizeHTMLForFPDF(htmlContent string) string {
	cleaned := htmlContent
	patterns := []string{
		`(?is)<style[^>]*>.*?</style>`,
		`(?is)<script[^>]*>.*?</script>`,
		`(?is)<head[^>]*>.*?</head>`,
		`(?is)</?(html|body|doctype)[^>]*>`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		cleaned = re.ReplaceAllString(cleaned, "")
	}

	return cleaned
}

func (s *EmailService) buildTicketPDFHTML(customerName, orderNumber string, ticket TicketEmailData) (string, error) {
	templateContent := defaultTicketPDFTemplate
	if path := strings.TrimSpace(s.cfg.TicketPDFTemplatePath); path != "" {
		if data, err := os.ReadFile(path); err == nil {
			templateContent = string(data)
		} else {
			fmt.Printf("WARN: impossible de lire template PDF ticket=%s, fallback template interne (%v)\n", path, err)
		}
	}

	t, err := template.New("ticket-pdf").Parse(templateContent)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	qrDataURI := template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(ticket.QRCodePNG))
	bannerDataURI := s.resolveTicketBannerDataURI()
	err = t.Execute(&buf, map[string]interface{}{
		"FestivalName": s.cfg.FestivalName,
		"FestivalDate": s.cfg.FestivalDate,
		"CustomerName": customerName,
		"OrderNumber":  orderNumber,
		"Ticket":       ticket,
		"TicketTypeName": ticket.TicketTypeName,
		"AttendeeName":   ticket.AttendeeName,
		"RecipientEmail": ticket.RecipientEmail,
		"QRToken":        ticket.QRToken,
		"QRCodeDataURI":  qrDataURI,
		"BannerDataURI":  bannerDataURI,
		"SupportEmail": s.cfg.SMTPFrom,
	})
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (s *EmailService) resolveTicketBannerDataURI() template.URL {
	candidates := make([]string, 0, 16)

	if frontendDir := strings.TrimSpace(os.Getenv("FRONTEND_DIR")); frontendDir != "" {
		candidates = append(candidates,
			filepath.Join(frontendDir, "img", "top_ticket.jpg"),
			filepath.Join(frontendDir, "img", "top_ticket.jpeg"),
			filepath.Join(frontendDir, "img", "top_ticket.png"),
		)
	}

	candidates = append(candidates,
		filepath.Join("..", "frontend", "public", "img", "top_ticket.jpg"),
		filepath.Join("..", "frontend", "public", "img", "top_ticket.jpeg"),
		filepath.Join("..", "frontend", "public", "img", "top_ticket.png"),
		"frontend/public/img/top_ticket.jpg",
		"frontend/public/img/top_ticket.jpeg",
		"frontend/public/img/top_ticket.png",
	)

	if templatePath := strings.TrimSpace(s.cfg.TicketPDFTemplatePath); templatePath != "" {
		baseDir := filepath.Dir(templatePath)
		candidates = append(candidates,
			filepath.Join(baseDir, "img", "top_ticket.jpg"),
			filepath.Join(baseDir, "img", "top_ticket.jpeg"),
			filepath.Join(baseDir, "img", "top_ticket.png"),
			filepath.Join(baseDir, "img", "top_ticket.svg"),
		)
	}

	candidates = append(candidates,
		"mail/img/top_ticket.jpg",
		"mail/img/top_ticket.jpeg",
		"mail/img/top_ticket.png",
		"mail/img/top_ticket.svg",
		"backend/mail/img/top_ticket.jpg",
		"backend/mail/img/top_ticket.jpeg",
		"backend/mail/img/top_ticket.png",
		"backend/mail/img/top_ticket.svg",
	)

	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		path := strings.TrimSpace(candidate)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}

		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			continue
		}

		mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
		if mimeType == "" {
			mimeType = "image/png"
		}

		return template.URL("data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data))
	}

	return ""
}

func formatFrenchDate(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	months := []string{"", "janvier", "février", "mars", "avril", "mai", "juin",
		"juillet", "août", "septembre", "octobre", "novembre", "décembre"}
	return fmt.Sprintf("%d %s %d", t.Day(), months[t.Month()], t.Year())
}

func sanitizeFilename(value string) string {
	replacer := strings.NewReplacer(" ", "_", "/", "-", "\\", "-", ":", "-", "*", "", "?", "", "\"", "", "<", "", ">", "", "|", "")
	cleaned := replacer.Replace(strings.TrimSpace(value))
	if cleaned == "" {
		return "ticket"
	}
	return cleaned
}

func buildPlainTextTicketEmail(festivalName, customerName, orderNumber string, tickets []TicketEmailData, supportEmail string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s\n", festivalName))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Bonjour %s,\n", customerName))
	b.WriteString(fmt.Sprintf("Votre commande %s est confirmée.\n", orderNumber))
	b.WriteString("\nBillets :\n")
	for _, t := range tickets {
		line := fmt.Sprintf("- %s", t.TicketTypeName)
		if strings.TrimSpace(t.AttendeeName) != "" {
			line += fmt.Sprintf(" (%s)", t.AttendeeName)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\nPrésentez votre QR code à l'entrée.\n")
	if strings.TrimSpace(supportEmail) != "" {
		b.WriteString(fmt.Sprintf("Contact: %s\n", supportEmail))
	}
	return b.String()
}

func senderDomain(from string) string {
	parts := strings.Split(strings.TrimSpace(from), "@")
	if len(parts) == 2 && parts[1] != "" {
		return parts[1]
	}
	return "localhost"
}

func encodeBase64RFC2045(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	if encoded == "" {
		return ""
	}

	const lineLen = 76
	var out strings.Builder
	for i := 0; i < len(encoded); i += lineLen {
		end := i + lineLen
		if end > len(encoded) {
			end = len(encoded)
		}
		out.WriteString(encoded[i:end])
		out.WriteString("\r\n")
	}
	return out.String()
}

func (s *EmailService) buildSubject(orderNumber string, subjectTemplate string) (string, error) {
	tpl := strings.TrimSpace(subjectTemplate)
	if tpl == "" {
		tpl = "{{.FestivalName}} - Vos billets (Commande {{.OrderNumber}})"
	}

	t, err := template.New("subject").Parse(tpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, map[string]string{
		"FestivalName": s.cfg.FestivalName,
		"OrderNumber":  orderNumber,
	}); err != nil {
		return "", err
	}

	return html.UnescapeString(strings.TrimSpace(buf.String())), nil
}

const defaultTicketEmailTemplate = `<!DOCTYPE html>
<html lang="fr">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"></head>
<body style="margin:0;padding:0;background:#ffe2bf;font-family:Arial,Helvetica,sans-serif;color:#3f1a10;">
<table width="100%" cellspacing="0" cellpadding="0" border="0" style="background:linear-gradient(180deg,#7f1713 0%,#b52a1b 30%,#e14e21 62%,#ff8b2b 84%,#ffe2bf 100%);padding:24px 12px;">
<tr><td align="center">
<table width="100%" cellspacing="0" cellpadding="0" border="0" style="max-width:700px;background:#fffdfa;border-radius:24px;overflow:hidden;box-shadow:0 10px 40px rgba(127,23,19,.18);border:1px solid rgba(216,58,33,.16);">
<tr><td style="background:linear-gradient(145deg,#7f1713 0%,#d83a21 55%,#ff8b2b 100%);padding:32px 24px;text-align:center;">
<div style="font-size:11px;font-weight:800;letter-spacing:3px;text-transform:uppercase;color:rgba(255,255,255,.7);margin-bottom:10px;">Confirmation de commande</div>
<div style="font-size:34px;font-weight:900;letter-spacing:1px;text-transform:uppercase;color:#ffffff;text-shadow:0 1px 2px rgba(0,0,0,.25);margin-bottom:10px;">{{.FestivalName}}</div>
<div style="font-size:16px;font-weight:700;color:#ffe9d2;line-height:1.6;">{{.FestivalDate}}</div>
</td></tr>
<tr><td style="padding:28px 26px 12px;">
<p style="margin:0 0 12px;font-size:15px;line-height:1.75;color:#3f1a10;">Bonjour <strong>{{.CustomerName}}</strong>,</p>
<p style="margin:0 0 12px;font-size:15px;line-height:1.75;color:#3f1a10;">Votre commande <strong>{{.OrderNumber}}</strong> a bien été confirmée.</p>
<p style="margin:0 0 16px;font-size:15px;line-height:1.75;color:#3f1a10;">Vos billets sont joints à cet email en PDF.</p>
</td></tr>
<tr><td style="background:#7f1713;padding:22px 26px;text-align:center;">
<div style="font-size:24px;font-weight:900;text-transform:uppercase;color:#ff8b2b;margin-bottom:6px;">{{.FestivalName}}</div>
<div style="font-size:13px;color:#ffe9d2;margin-bottom:4px;">{{.FestivalDate}}</div>
<div style="font-size:12px;color:rgba(255,233,210,.5);">Contact : {{.SupportEmail}}</div>
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>`

const defaultTicketPDFTemplate = `
<h1>{{.FestivalName}}</h1>
<p><b>Commande :</b> {{.OrderNumber}}</p>
<p><b>Client :</b> {{.CustomerName}}</p>
<p><b>Date :</b> {{.FestivalDate}}</p>
<hr>
<h2>Billet : {{.Ticket.TicketTypeName}}</h2>
{{if .Ticket.AttendeeName}}<p><b>Participant :</b> {{.Ticket.AttendeeName}}</p>{{end}}
{{if .Ticket.RecipientEmail}}<p><b>Email de réception :</b> {{.Ticket.RecipientEmail}}</p>{{end}}
<p><b>Référence QR :</b> {{.Ticket.QRToken}}</p>
<p>Présentez ce billet (PDF) et le QR code à l'entrée. Le QR code est affiché ci-dessous.</p>
<p>Contact : {{.SupportEmail}}</p>
`

func (s *EmailService) SendAdminTestEmail(to string) error {
	if strings.TrimSpace(to) == "" {
		return fmt.Errorf("destinataire email manquant")
	}
	if s.cfg.SMTPHost == "" {
		return fmt.Errorf("SMTP non configuré")
	}

	subject := fmt.Sprintf("%s - Test SMTP", s.cfg.FestivalName)
	body := fmt.Sprintf("Bonjour,\r\n\r\nCeci est un email de test SMTP depuis l'interface admin %s.\r\n\r\nSi vous recevez ce message, la configuration email fonctionne.\r\n", s.cfg.FestivalName)

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", s.cfg.SMTPFromName, s.cfg.SMTPFrom))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	auth := smtp.PlainAuth("", s.cfg.SMTPUser, s.cfg.SMTPPassword, s.cfg.SMTPHost)
	addr := fmt.Sprintf("%s:%d", s.cfg.SMTPHost, s.cfg.SMTPPort)

	return smtp.SendMail(addr, auth, s.cfg.SMTPFrom, []string{to}, []byte(msg.String()))
}
