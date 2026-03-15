package services

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"github.com/kreton/if-festival/internal/config"
	"github.com/kreton/if-festival/internal/models"
	"github.com/kreton/if-festival/internal/repository"
)

// AdminService gère la logique admin (auth, stats, validation QR)
type AdminService struct {
	cfg        *config.Config
	adminRepo  *repository.AdminRepository
	orderRepo  *repository.OrderRepository
	ticketRepo *repository.TicketRepository
	redis      *redis.Client
}

func NewAdminService(
	cfg *config.Config,
	adminRepo *repository.AdminRepository,
	orderRepo *repository.OrderRepository,
	ticketRepo *repository.TicketRepository,
	redis *redis.Client,
) *AdminService {
	return &AdminService{
		cfg:        cfg,
		adminRepo:  adminRepo,
		orderRepo:  orderRepo,
		ticketRepo: ticketRepo,
		redis:      redis,
	}
}

// Login authentifie un admin et retourne un JWT
func (s *AdminService) Login(ctx context.Context, req models.LoginRequest) (*models.LoginResponse, error) {
	admin, err := s.adminRepo.GetByUsername(ctx, req.Username)
	if err != nil {
		return nil, fmt.Errorf("erreur recherche admin: %w", err)
	}
	if admin == nil {
		return nil, fmt.Errorf("identifiants invalides")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
		return nil, fmt.Errorf("identifiants invalides")
	}

	// Générer le JWT
	claims := jwt.MapClaims{
		"sub":  admin.ID,
		"name": admin.DisplayName,
		"role": admin.Role,
		"exp":  time.Now().Add(12 * time.Hour).Unix(),
		"iat":  time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return nil, fmt.Errorf("erreur génération token: %w", err)
	}

	// Mettre à jour last_login
	_ = s.adminRepo.UpdateLastLogin(ctx, admin.ID)

	return &models.LoginResponse{
		Token:       tokenStr,
		DisplayName: admin.DisplayName,
		Role:        admin.Role,
	}, nil
}

// GetStats retourne les statistiques de vente
func (s *AdminService) GetStats(ctx context.Context) (*models.SalesStats, error) {
	return s.orderRepo.GetSalesStats(ctx)
}

// ListOrders retourne la liste paginée des commandes
func (s *AdminService) ListOrders(ctx context.Context, params models.OrderListParams) (*models.OrderListResponse, error) {
	return s.orderRepo.ListOrders(ctx, params)
}

// ValidateQR valide un QR code à l'entrée du festival
func (s *AdminService) ValidateQR(ctx context.Context, qrToken string, adminName string) (*models.ValidateQRResponse, error) {
	return s.ticketRepo.ValidateTicket(ctx, qrToken, adminName)
}

// CreateAdmin crée un nouveau compte admin
func (s *AdminService) CreateAdmin(ctx context.Context, username, password, displayName, role string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("erreur hash password: %w", err)
	}

	admin := &models.Admin{
		Username:     username,
		PasswordHash: string(hash),
		DisplayName:  displayName,
		Role:         role,
	}

	return s.adminRepo.CreateAdmin(ctx, admin)
}

func (s *AdminService) ChangePassword(ctx context.Context, adminID, currentPassword, newPassword string) error {
	if len(newPassword) < 8 {
		return fmt.Errorf("le nouveau mot de passe doit contenir au moins 8 caractères")
	}

	admin, err := s.adminRepo.GetByID(ctx, adminID)
	if err != nil {
		return fmt.Errorf("erreur récupération admin: %w", err)
	}
	if admin == nil {
		return fmt.Errorf("admin introuvable")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(currentPassword)); err != nil {
		return fmt.Errorf("mot de passe actuel invalide")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("erreur hash password: %w", err)
	}

	if err := s.adminRepo.UpdatePasswordHash(ctx, adminID, string(hash)); err != nil {
		return fmt.Errorf("erreur mise à jour mot de passe: %w", err)
	}

	return nil
}

// SaveWebhookLog enregistre un webhook reçu
func (s *AdminService) SaveWebhookLog(ctx context.Context, eventType string, payload []byte) (int64, error) {
	return s.adminRepo.SaveWebhookLog(ctx, eventType, payload)
}

// MarkWebhookProcessed marque un webhook comme traité
func (s *AdminService) MarkWebhookProcessed(ctx context.Context, id int64, errMsg string) error {
	return s.adminRepo.MarkWebhookProcessed(ctx, id, errMsg)
}

// ValidateJWT valide un token JWT et retourne les claims
func (s *AdminService) ValidateJWT(tokenStr string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("méthode de signature inattendue: %v", token.Header["alg"])
		}
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("token invalide: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("token invalide")
	}

	return claims, nil
}
