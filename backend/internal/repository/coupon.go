package repository

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kreton/if-festival/internal/models"
)

type CouponRepository struct {
	pool *pgxpool.Pool
}

func NewCouponRepository(pool *pgxpool.Pool) *CouponRepository {
	return &CouponRepository{pool: pool}
}

func (r *CouponRepository) CreateCoupon(ctx context.Context, req models.CreateCouponRequest) (*models.Coupon, error) {
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	if code == "" {
		var err error
		code, err = generateCouponCode(8)
		if err != nil {
			return nil, fmt.Errorf("erreur génération code coupon: %w", err)
		}
	}

	var c models.Coupon
	query := `
		INSERT INTO coupons (name, code, ticket_type_id, max_uses, discount_cents)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, code, ticket_type_id, max_uses, used_count, discount_cents, created_at`

	err := r.pool.QueryRow(ctx, query,
		strings.TrimSpace(req.Name),
		code,
		req.TicketTypeID,
		req.MaxUses,
		req.DiscountCents,
	).Scan(
		&c.ID,
		&c.Name,
		&c.Code,
		&c.TicketTypeID,
		&c.MaxUses,
		&c.UsedCount,
		&c.DiscountCents,
		&c.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("erreur création coupon: %w", err)
	}

	return &c, nil
}

func (r *CouponRepository) ListCoupons(ctx context.Context) ([]models.Coupon, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.name, c.code, c.ticket_type_id, tt.name, c.max_uses, c.used_count, c.discount_cents, c.created_at
		FROM coupons c
		JOIN ticket_types tt ON tt.id = c.ticket_type_id
		ORDER BY c.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("erreur liste coupons: %w", err)
	}
	defer rows.Close()

	var result []models.Coupon
	for rows.Next() {
		var c models.Coupon
		if err := rows.Scan(
			&c.ID,
			&c.Name,
			&c.Code,
			&c.TicketTypeID,
			&c.TicketTypeName,
			&c.MaxUses,
			&c.UsedCount,
			&c.DiscountCents,
			&c.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, nil
}

func (r *CouponRepository) GetCouponByCode(ctx context.Context, code string) (*models.Coupon, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil, nil
	}

	var c models.Coupon
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, code, ticket_type_id, max_uses, used_count, discount_cents, created_at
		FROM coupons
		WHERE code = $1`, code).Scan(
		&c.ID,
		&c.Name,
		&c.Code,
		&c.TicketTypeID,
		&c.MaxUses,
		&c.UsedCount,
		&c.DiscountCents,
		&c.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("erreur lookup coupon: %w", err)
	}

	return &c, nil
}

func (r *CouponRepository) GetCouponByCodeForUpdate(ctx context.Context, tx pgx.Tx, code string) (*models.Coupon, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil, nil
	}

	var c models.Coupon
	err := tx.QueryRow(ctx, `
		SELECT id, name, code, ticket_type_id, max_uses, used_count, discount_cents, created_at
		FROM coupons
		WHERE code = $1
		FOR UPDATE`, code).Scan(
		&c.ID,
		&c.Name,
		&c.Code,
		&c.TicketTypeID,
		&c.MaxUses,
		&c.UsedCount,
		&c.DiscountCents,
		&c.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("erreur lock coupon: %w", err)
	}

	return &c, nil
}

func (r *CouponRepository) IncrementCouponUsage(ctx context.Context, tx pgx.Tx, couponID string, uses int) error {
	if uses <= 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `UPDATE coupons SET used_count = used_count + $1 WHERE id = $2`, uses, couponID)
	if err != nil {
		return fmt.Errorf("erreur update coupon usage: %w", err)
	}
	return nil
}

func (r *CouponRepository) InsertCouponRedemption(ctx context.Context, tx pgx.Tx, couponID, orderID string, uses int) error {
	if uses <= 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO coupon_redemptions (coupon_id, order_id, uses)
		VALUES ($1, $2, $3)`, couponID, orderID, uses)
	if err != nil {
		return fmt.Errorf("erreur insertion redemption: %w", err)
	}
	return nil
}

func generateCouponCode(length int) (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	if length <= 0 {
		length = 8
	}
	buf := make([]byte, length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		buf[i] = alphabet[n.Int64()]
	}
	return string(buf), nil
}
