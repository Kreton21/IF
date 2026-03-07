package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	qrcode "github.com/skip2/go-qrcode"
)

// QRCodeService gère la génération de QR codes
type QRCodeService struct {
	baseURL string // URL de base pour la validation
}

func NewQRCodeService(baseURL string) *QRCodeService {
	return &QRCodeService{baseURL: baseURL}
}

// GenerateToken génère un token unique pour un QR code
func (s *QRCodeService) GenerateToken() (string, error) {
	bytes := make([]byte, 32) // 256 bits
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("erreur génération token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// GenerateQRCode génère l'image QR code PNG à partir d'un token
func (s *QRCodeService) GenerateQRCode(token string) ([]byte, error) {
	// Le QR code contient le token directement
	// Au scan, l'app admin enverra ce token au backend pour validation
	content := token

	png, err := qrcode.Encode(content, qrcode.Medium, 512)
	if err != nil {
		return nil, fmt.Errorf("erreur génération QR code: %w", err)
	}

	return png, nil
}
