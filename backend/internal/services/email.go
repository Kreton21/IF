package services

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/smtp"
	"os"
	"strings"
	"time"

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

	htmlBody, err := s.buildEmailHTML(customerName, orderNumber, tickets, templatePath)
	if err != nil {
		return fmt.Errorf("erreur génération email: %w", err)
	}
	plainBody := buildPlainTextTicketEmail(s.cfg.FestivalName, customerName, orderNumber, tickets, s.cfg.SMTPFrom)

	if err := s.sendMIMEEmail(to, subject, plainBody, htmlBody, tickets); err != nil {
		return err
	}

	fmt.Printf("📧 Email confirmation %s envoyé à %s (commande %s)\n", emailKind, to, orderNumber)
	return nil
}

type TicketEmailData struct {
	TicketTypeName string
	AttendeeName   string
	QRToken        string
	QRCodePNG      []byte
	CID            string
}

func (s *EmailService) buildEmailHTML(customerName, orderNumber string, tickets []TicketEmailData, templatePath string) (string, error) {
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

	var buf bytes.Buffer
	err = t.Execute(&buf, map[string]interface{}{
		"FestivalName": s.cfg.FestivalName,
		"FestivalDate": s.cfg.FestivalDate,
		"CustomerName": customerName,
		"OrderNumber":  orderNumber,
		"Tickets":      tickets,
		"SupportEmail": s.cfg.SMTPFrom,
	})
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (s *EmailService) sendMIMEEmail(to, subject, plainBody, htmlBody string, tickets []TicketEmailData) error {
	boundary := "==FESTIVAL_BOUNDARY=="
	now := time.Now().UTC().Format(time.RFC1123Z)
	messageID := fmt.Sprintf("<if-%d@%s>", time.Now().UnixNano(), senderDomain(s.cfg.SMTPFrom))

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", s.cfg.SMTPFromName, s.cfg.SMTPFrom))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString(fmt.Sprintf("Date: %s\r\n", now))
	msg.WriteString(fmt.Sprintf("Message-ID: %s\r\n", messageID))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/related; boundary=\"%s\"\r\n", boundary))
	msg.WriteString("\r\n")

	// Plain text part
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: 7bit\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(plainBody)
	msg.WriteString("\r\n")

	// HTML part
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: 7bit\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)
	msg.WriteString("\r\n")

	// QR code images inline
	for i, ticket := range tickets {
		if len(ticket.QRCodePNG) > 0 {
			cid := ticket.CID
			if strings.TrimSpace(cid) == "" {
				cid = fmt.Sprintf("qr-%d", i)
			}
			msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
			msg.WriteString("Content-Type: image/png\r\n")
			msg.WriteString(fmt.Sprintf("Content-ID: <%s>\r\n", cid))
			msg.WriteString("Content-Transfer-Encoding: base64\r\n")
			msg.WriteString(fmt.Sprintf("Content-Disposition: inline; filename=\"qr_%d.png\"\r\n", i))
			msg.WriteString("\r\n")
			msg.WriteString(encodeBase64RFC2045(ticket.QRCodePNG))
			msg.WriteString("\r\n")
		}
	}

	msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	auth := smtp.PlainAuth("", s.cfg.SMTPUser, s.cfg.SMTPPassword, s.cfg.SMTPHost)
	addr := fmt.Sprintf("%s:%d", s.cfg.SMTPHost, s.cfg.SMTPPort)

	return smtp.SendMail(addr, auth, s.cfg.SMTPFrom, []string{to}, []byte(msg.String()))
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

	return strings.TrimSpace(buf.String()), nil
}

const defaultTicketEmailTemplate = `<!DOCTYPE html>
<html>
<head><meta charset="utf-8"></head>
<body style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
	<div style="background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); padding: 30px; border-radius: 10px; color: white; text-align: center;">
		<h1 style="margin: 0;">🎵 {{.FestivalName}}</h1>
		<p style="margin: 10px 0 0 0; font-size: 18px;">Vos billets sont prêts !</p>
	</div>

	<div style="padding: 20px 0;">
		<p>Bonjour <strong>{{.CustomerName}}</strong>,</p>
		<p>Merci pour votre achat ! Voici vos billets pour le <strong>{{.FestivalName}}</strong>.</p>
		<p style="color: #666;">Commande : <strong>{{.OrderNumber}}</strong></p>
	</div>

	{{range $ticket := .Tickets}}
	<div style="border: 2px solid #667eea; border-radius: 10px; padding: 20px; margin: 15px 0; text-align: center;">
		<h3 style="color: #667eea; margin-top: 0;">🎫 {{$ticket.TicketTypeName}}</h3>
		{{if $ticket.AttendeeName}}<p>Participant : <strong>{{$ticket.AttendeeName}}</strong></p>{{end}}
		<p style="font-size: 12px; color: #999;">Présentez ce QR code à l'entrée du festival</p>
		<img src="cid:{{$ticket.CID}}" alt="QR Code" style="max-width: 250px;" />
	</div>
	{{end}}

	<div style="background: #f5f5f5; padding: 15px; border-radius: 5px; margin-top: 20px;">
		<p style="margin: 0; font-size: 13px; color: #666;">
			<strong>📱 Important :</strong> Présentez ce QR code (imprimé ou sur mobile) à l'entrée du festival.
			Chaque QR code ne peut être utilisé qu'une seule fois.
		</p>
	</div>

	<p style="text-align: center; color: #999; font-size: 12px; margin-top: 30px;">
		{{.FestivalName}} — {{.FestivalDate}} — Contact : {{.SupportEmail}}
	</p>
</body>
</html>`

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
