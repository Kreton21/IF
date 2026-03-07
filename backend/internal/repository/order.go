package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kreton/if-festival/internal/models"
)

type OrderRepository struct {
	pool *pgxpool.Pool
}

func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{pool: pool}
}

func (r *OrderRepository) CreateOrder(ctx context.Context, tx pgx.Tx, order *models.Order) error {
	// Générer le numéro de commande
	var seq int
	err := tx.QueryRow(ctx, `SELECT nextval('order_number_seq')`).Scan(&seq)
	if err != nil {
		return fmt.Errorf("erreur génération numéro commande: %w", err)
	}
	order.OrderNumber = fmt.Sprintf("IF-2026-%05d", seq)

	query := `
		INSERT INTO orders (order_number, customer_email, customer_first_name, customer_last_name,
		                     customer_phone, total_cents, status, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::inet, $9)
		RETURNING id, created_at, updated_at`

	return tx.QueryRow(ctx, query,
		order.OrderNumber, order.CustomerEmail, order.CustomerFirstName, order.CustomerLastName,
		order.CustomerPhone, order.TotalCents, order.Status, order.IPAddress, order.UserAgent,
	).Scan(&order.ID, &order.CreatedAt, &order.UpdatedAt)
}

func (r *OrderRepository) UpdateOrderHelloAsso(ctx context.Context, orderID string, checkoutID string, checkoutURL string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE orders SET helloasso_checkout_id = $1, helloasso_checkout_url = $2 WHERE id = $3`,
		checkoutID, checkoutURL, orderID,
	)
	return err
}

func (r *OrderRepository) UpdateOrderStatus(ctx context.Context, orderID string, status models.OrderStatus) error {
	query := `UPDATE orders SET status = $1`
	args := []interface{}{status}

	switch status {
	case models.OrderStatusPaid:
		query += `, paid_at = NOW()`
	case models.OrderStatusConfirmed:
		query += `, confirmed_at = NOW()`
	}

	query += ` WHERE id = $` + fmt.Sprintf("%d", len(args)+1)
	args = append(args, orderID)

	_, err := r.pool.Exec(ctx, query, args...)
	return err
}

func (r *OrderRepository) GetOrderByID(ctx context.Context, orderID string) (*models.Order, error) {
	query := `
		SELECT id, order_number, customer_email, customer_first_name, customer_last_name,
		       COALESCE(customer_phone, ''), total_cents, status,
		       COALESCE(helloasso_checkout_id, ''), COALESCE(helloasso_payment_id, ''),
		       COALESCE(helloasso_checkout_url, ''), created_at, updated_at, paid_at, confirmed_at
		FROM orders
		WHERE id = $1`

	var o models.Order
	err := r.pool.QueryRow(ctx, query, orderID).Scan(
		&o.ID, &o.OrderNumber, &o.CustomerEmail, &o.CustomerFirstName, &o.CustomerLastName,
		&o.CustomerPhone, &o.TotalCents, &o.Status, &o.HelloAssoCheckoutID, &o.HelloAssoPaymentID,
		&o.HelloAssoCheckoutURL, &o.CreatedAt, &o.UpdatedAt, &o.PaidAt, &o.ConfirmedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("erreur query order: %w", err)
	}

	return &o, nil
}

func (r *OrderRepository) GetOrderByCheckoutID(ctx context.Context, checkoutID string) (*models.Order, error) {
	query := `
		SELECT id, order_number, customer_email, customer_first_name, customer_last_name,
		       COALESCE(customer_phone, ''), total_cents, status,
		       COALESCE(helloasso_checkout_id, ''), COALESCE(helloasso_payment_id, ''),
		       COALESCE(helloasso_checkout_url, ''), created_at, updated_at, paid_at, confirmed_at
		FROM orders
		WHERE helloasso_checkout_id = $1`

	var o models.Order
	err := r.pool.QueryRow(ctx, query, checkoutID).Scan(
		&o.ID, &o.OrderNumber, &o.CustomerEmail, &o.CustomerFirstName, &o.CustomerLastName,
		&o.CustomerPhone, &o.TotalCents, &o.Status, &o.HelloAssoCheckoutID, &o.HelloAssoPaymentID,
		&o.HelloAssoCheckoutURL, &o.CreatedAt, &o.UpdatedAt, &o.PaidAt, &o.ConfirmedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("erreur query order by checkout: %w", err)
	}

	return &o, nil
}

func (r *OrderRepository) SetHelloAssoPaymentID(ctx context.Context, orderID string, paymentID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE orders SET helloasso_payment_id = $1 WHERE id = $2`,
		paymentID, orderID,
	)
	return err
}

func (r *OrderRepository) ListOrders(ctx context.Context, params models.OrderListParams) (*models.OrderListResponse, error) {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 || params.PageSize > 100 {
		params.PageSize = 20
	}

	var conditions []string
	var args []interface{}
	argIdx := 1

	if params.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, params.Status)
		argIdx++
	}

	if params.Search != "" {
		conditions = append(conditions, fmt.Sprintf(
			"(customer_email ILIKE $%d OR customer_first_name ILIKE $%d OR customer_last_name ILIKE $%d OR order_number ILIKE $%d)",
			argIdx, argIdx, argIdx, argIdx,
		))
		args = append(args, "%"+params.Search+"%")
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM orders %s", whereClause)
	var totalCount int
	err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		return nil, fmt.Errorf("erreur count orders: %w", err)
	}

	// Fetch page
	offset := (params.Page - 1) * params.PageSize
	dataQuery := fmt.Sprintf(`
		SELECT id, order_number, customer_email, customer_first_name, customer_last_name,
		       COALESCE(customer_phone, ''), total_cents, status,
		       COALESCE(helloasso_checkout_id, ''), COALESCE(helloasso_payment_id, ''),
		       COALESCE(helloasso_checkout_url, ''), created_at, updated_at, paid_at, confirmed_at
		FROM orders %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)
	args = append(args, params.PageSize, offset)

	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("erreur query orders: %w", err)
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var o models.Order
		err := rows.Scan(
			&o.ID, &o.OrderNumber, &o.CustomerEmail, &o.CustomerFirstName, &o.CustomerLastName,
			&o.CustomerPhone, &o.TotalCents, &o.Status, &o.HelloAssoCheckoutID, &o.HelloAssoPaymentID,
			&o.HelloAssoCheckoutURL, &o.CreatedAt, &o.UpdatedAt, &o.PaidAt, &o.ConfirmedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("erreur scan order: %w", err)
		}
		orders = append(orders, o)
	}

	return &models.OrderListResponse{
		Orders:     orders,
		TotalCount: totalCount,
		Page:       params.Page,
		PageSize:   params.PageSize,
	}, nil
}

func (r *OrderRepository) GetSalesStats(ctx context.Context) (*models.SalesStats, error) {
	stats := &models.SalesStats{}

	// Totaux globaux
	err := r.pool.QueryRow(ctx, `
		SELECT 
			COUNT(*) FILTER (WHERE status IN ('paid', 'confirmed')),
			COALESCE(SUM(total_cents) FILTER (WHERE status IN ('paid', 'confirmed')), 0)
		FROM orders
	`).Scan(&stats.TotalOrders, &stats.TotalRevenueCents)
	if err != nil {
		return nil, fmt.Errorf("erreur stats globales: %w", err)
	}

	// Tickets vendus et validés
	err = r.pool.QueryRow(ctx, `
		SELECT 
			COUNT(*) FILTER (WHERE o.status IN ('paid', 'confirmed')),
			COUNT(*) FILTER (WHERE t.is_validated = true)
		FROM tickets t
		JOIN orders o ON o.id = t.order_id
	`).Scan(&stats.TotalTicketsSold, &stats.TotalValidated)
	if err != nil {
		return nil, fmt.Errorf("erreur stats tickets: %w", err)
	}

	// Stats par type de ticket
	rows, err := r.pool.Query(ctx, `
		SELECT 
			tt.id, tt.name, tt.price_cents, tt.quantity_total, tt.quantity_sold,
			COUNT(t.id) FILTER (WHERE t.is_validated = true) as validated,
			COALESCE(SUM(tt.price_cents) FILTER (WHERE o.status IN ('paid', 'confirmed')), 0) as revenue
		FROM ticket_types tt
		LEFT JOIN tickets t ON t.ticket_type_id = tt.id
		LEFT JOIN orders o ON o.id = t.order_id
		GROUP BY tt.id, tt.name, tt.price_cents, tt.quantity_total, tt.quantity_sold
		ORDER BY tt.name
	`)
	if err != nil {
		return nil, fmt.Errorf("erreur stats par type: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var s models.TicketTypeStat
		err := rows.Scan(&s.TicketTypeID, &s.Name, &s.PriceCents, &s.QuantityTotal,
			&s.QuantitySold, &s.QuantityValidated, &s.RevenueCents)
		if err != nil {
			return nil, fmt.Errorf("erreur scan stat: %w", err)
		}
		stats.ByTicketType = append(stats.ByTicketType, s)
	}

	// Ventes par jour
	dayRows, err := r.pool.Query(ctx, `
		SELECT 
			DATE(o.created_at)::text as sale_date,
			COUNT(DISTINCT o.id) as order_count,
			COALESCE(SUM((SELECT COUNT(*) FROM tickets t2 WHERE t2.order_id = o.id)), 0) as ticket_count,
			COALESCE(SUM(o.total_cents), 0) as revenue
		FROM orders o
		WHERE o.status IN ('paid', 'confirmed')
		GROUP BY DATE(o.created_at)
		ORDER BY DATE(o.created_at) DESC
		LIMIT 30
	`)
	if err != nil {
		return nil, fmt.Errorf("erreur stats par jour: %w", err)
	}
	defer dayRows.Close()

	for dayRows.Next() {
		var d models.DailySales
		err := dayRows.Scan(&d.Date, &d.OrderCount, &d.TicketCount, &d.RevenueCents)
		if err != nil {
			return nil, fmt.Errorf("erreur scan daily: %w", err)
		}
		stats.SalesByDay = append(stats.SalesByDay, d)
	}

	// 10 dernières commandes
	recentRows, err := r.pool.Query(ctx, `
		SELECT id, order_number, customer_email, customer_first_name, customer_last_name,
		       COALESCE(customer_phone, ''), total_cents, status,
		       COALESCE(helloasso_checkout_id, ''), COALESCE(helloasso_payment_id, ''),
		       COALESCE(helloasso_checkout_url, ''), created_at, updated_at, paid_at, confirmed_at
		FROM orders
		WHERE status IN ('paid', 'confirmed')
		ORDER BY created_at DESC
		LIMIT 10
	`)
	if err != nil {
		return nil, fmt.Errorf("erreur recent orders: %w", err)
	}
	defer recentRows.Close()

	for recentRows.Next() {
		var o models.Order
		err := recentRows.Scan(
			&o.ID, &o.OrderNumber, &o.CustomerEmail, &o.CustomerFirstName, &o.CustomerLastName,
			&o.CustomerPhone, &o.TotalCents, &o.Status, &o.HelloAssoCheckoutID, &o.HelloAssoPaymentID,
			&o.HelloAssoCheckoutURL, &o.CreatedAt, &o.UpdatedAt, &o.PaidAt, &o.ConfirmedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("erreur scan recent order: %w", err)
		}
		stats.RecentOrders = append(stats.RecentOrders, o)
	}

	return stats, nil
}

func (r *OrderRepository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}
