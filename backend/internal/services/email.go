package services

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/smtp"
	"strings"

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
	if s.cfg.SMTPHost == "" {
		fmt.Printf("📧 [MOCK] Email envoyé à %s pour commande %s (%d tickets)\n", to, orderNumber, len(tickets))
		return nil
	}

	subject := fmt.Sprintf("%s - Vos billets (Commande %s)", s.cfg.FestivalName, orderNumber)

	htmlBody, err := s.buildEmailHTML(customerName, orderNumber, tickets)
	if err != nil {
		return fmt.Errorf("erreur génération email: %w", err)
	}

	return s.sendMIMEEmail(to, subject, htmlBody, tickets)
}

type TicketEmailData struct {
	TicketTypeName string
	AttendeeName   string
	QRToken        string
	QRCodePNG      []byte
}

func (s *EmailService) buildEmailHTML(customerName, orderNumber string, tickets []TicketEmailData) (string, error) {
	tmpl := `<!DOCTYPE html>
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

  {{range $i, $ticket := .Tickets}}
  <div style="border: 2px solid #667eea; border-radius: 10px; padding: 20px; margin: 15px 0; text-align: center;">
    <h3 style="color: #667eea; margin-top: 0;">🎫 {{$ticket.TicketTypeName}}</h3>
    {{if $ticket.AttendeeName}}<p>Participant : <strong>{{$ticket.AttendeeName}}</strong></p>{{end}}
    <p style="font-size: 12px; color: #999;">Présentez ce QR code à l'entrée du festival</p>
    <img src="cid:qr-{{$i}}" alt="QR Code" style="max-width: 250px;" />
  </div>
  {{end}}

  <div style="background: #f5f5f5; padding: 15px; border-radius: 5px; margin-top: 20px;">
    <p style="margin: 0; font-size: 13px; color: #666;">
      <strong>📱 Important :</strong> Présentez ce QR code (imprimé ou sur mobile) à l'entrée du festival.
      Chaque QR code ne peut être utilisé qu'une seule fois.
    </p>
  </div>

  <p style="text-align: center; color: #999; font-size: 12px; margin-top: 30px;">
    {{.FestivalName}} — {{.FestivalDate}}
  </p>
</body>
</html>`

	t, err := template.New("ticket").Parse(tmpl)
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
	})
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (s *EmailService) sendMIMEEmail(to, subject, htmlBody string, tickets []TicketEmailData) error {
	boundary := "==FESTIVAL_BOUNDARY=="

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", s.cfg.SMTPFromName, s.cfg.SMTPFrom))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/related; boundary=\"%s\"\r\n", boundary))
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
			msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
			msg.WriteString("Content-Type: image/png\r\n")
			msg.WriteString(fmt.Sprintf("Content-ID: <qr-%d>\r\n", i))
			msg.WriteString("Content-Transfer-Encoding: base64\r\n")
			msg.WriteString(fmt.Sprintf("Content-Disposition: inline; filename=\"qr_%d.png\"\r\n", i))
			msg.WriteString("\r\n")
			msg.WriteString(base64.StdEncoding.EncodeToString(ticket.QRCodePNG))
			msg.WriteString("\r\n")
		}
	}

	msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	auth := smtp.PlainAuth("", s.cfg.SMTPUser, s.cfg.SMTPPassword, s.cfg.SMTPHost)
	addr := fmt.Sprintf("%s:%d", s.cfg.SMTPHost, s.cfg.SMTPPort)

	return smtp.SendMail(addr, auth, s.cfg.SMTPFrom, []string{to}, []byte(msg.String()))
}

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
