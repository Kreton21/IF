package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kreton/if-festival/internal/models"
)

type AdminRepository struct {
	pool *pgxpool.Pool
}

func NewAdminRepository(pool *pgxpool.Pool) *AdminRepository {
	return &AdminRepository{pool: pool}
}

func (r *AdminRepository) GetByUsername(ctx context.Context, username string) (*models.Admin, error) {
	query := `
		SELECT id, username, password_hash, display_name, role, is_active, last_login, created_at
		FROM admins
		WHERE username = $1 AND is_active = true`

	var a models.Admin
	err := r.pool.QueryRow(ctx, query, username).Scan(
		&a.ID, &a.Username, &a.PasswordHash, &a.DisplayName,
		&a.Role, &a.IsActive, &a.LastLogin, &a.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("erreur query admin: %w", err)
	}

	return &a, nil
}

func (r *AdminRepository) UpdateLastLogin(ctx context.Context, adminID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE admins SET last_login = NOW() WHERE id = $1`, adminID)
	return err
}

func (r *AdminRepository) CreateAdmin(ctx context.Context, admin *models.Admin) error {
	query := `
		INSERT INTO admins (username, password_hash, display_name, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`

	return r.pool.QueryRow(ctx, query,
		admin.Username, admin.PasswordHash, admin.DisplayName, admin.Role,
	).Scan(&admin.ID, &admin.CreatedAt)
}

// SaveWebhookLog enregistre un webhook reçu pour audit
func (r *AdminRepository) SaveWebhookLog(ctx context.Context, eventType string, payload []byte) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx,
		`INSERT INTO webhook_logs (event_type, payload) VALUES ($1, $2) RETURNING id`,
		eventType, payload,
	).Scan(&id)
	return id, err
}

func (r *AdminRepository) MarkWebhookProcessed(ctx context.Context, id int64, errMsg string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE webhook_logs SET processed = true, processed_at = NOW(), error_message = $1 WHERE id = $2`,
		errMsg, id,
	)
	return err
}
