package repository

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

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
	// Générer un numéro de commande aléatoire unique
	const maxAttempts = 20
	for attempt := 0; attempt < maxAttempts; attempt++ {
		candidate, err := generateRandomOrderNumber()
		if err != nil {
			return fmt.Errorf("erreur génération numéro commande: %w", err)
		}
		available, err := r.isOrderNumberAvailable(ctx, tx, candidate)
		if err != nil {
			return fmt.Errorf("erreur vérification numéro commande: %w", err)
		}
		if available {
			order.OrderNumber = candidate
			break
		}
	}
	if order.OrderNumber == "" {
		return fmt.Errorf("impossible de générer un numéro de commande unique")
	}

	query := `
		INSERT INTO orders (order_number, customer_email, customer_first_name, customer_last_name,
		                     customer_phone, date_of_birth, wants_camping, wants_refund_insurance, total_cents, status, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::inet, $12)
		RETURNING id, created_at, updated_at`

	return tx.QueryRow(ctx, query,
		order.OrderNumber, order.CustomerEmail, order.CustomerFirstName, order.CustomerLastName,
		order.CustomerPhone, order.DateOfBirth, order.WantsCamping, order.WantsRefundInsurance, order.TotalCents, order.Status, order.IPAddress, order.UserAgent,
	).Scan(&order.ID, &order.CreatedAt, &order.UpdatedAt)
}

func generateRandomOrderNumber() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("IF-2026-%06d", n.Int64()), nil
}

func (r *OrderRepository) isOrderNumberAvailable(ctx context.Context, tx pgx.Tx, orderNumber string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM orders WHERE order_number = $1)`, orderNumber).Scan(&exists)
	if err != nil {
		return false, err
	}
	return !exists, nil
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
		       COALESCE(customer_phone, ''), COALESCE(date_of_birth, ''), wants_camping, wants_refund_insurance, total_cents, status,
		       COALESCE(helloasso_checkout_id, ''), COALESCE(helloasso_payment_id, ''),
		       COALESCE(helloasso_checkout_url, ''), created_at, updated_at, paid_at, confirmed_at
		FROM orders
		WHERE id = $1`

	var o models.Order
	err := r.pool.QueryRow(ctx, query, orderID).Scan(
		&o.ID, &o.OrderNumber, &o.CustomerEmail, &o.CustomerFirstName, &o.CustomerLastName,
		&o.CustomerPhone, &o.DateOfBirth, &o.WantsCamping, &o.WantsRefundInsurance, &o.TotalCents, &o.Status, &o.HelloAssoCheckoutID, &o.HelloAssoPaymentID,
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

func (r *OrderRepository) GetOrderByReference(ctx context.Context, ref string) (*models.Order, error) {
	if ref == "" {
		return nil, nil
	}

	query := `
		SELECT id, order_number, customer_email, customer_first_name, customer_last_name,
		       COALESCE(customer_phone, ''), COALESCE(date_of_birth, ''), wants_camping, wants_refund_insurance, total_cents, status,
		       COALESCE(helloasso_checkout_id, ''), COALESCE(helloasso_payment_id, ''),
		       COALESCE(helloasso_checkout_url, ''), created_at, updated_at, paid_at, confirmed_at
		FROM orders
		WHERE id::text = $1 OR order_number = $1
		LIMIT 1`

	var o models.Order
	err := r.pool.QueryRow(ctx, query, ref).Scan(
		&o.ID, &o.OrderNumber, &o.CustomerEmail, &o.CustomerFirstName, &o.CustomerLastName,
		&o.CustomerPhone, &o.DateOfBirth, &o.WantsCamping, &o.WantsRefundInsurance, &o.TotalCents, &o.Status, &o.HelloAssoCheckoutID, &o.HelloAssoPaymentID,
		&o.HelloAssoCheckoutURL, &o.CreatedAt, &o.UpdatedAt, &o.PaidAt, &o.ConfirmedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("erreur query order by reference: %w", err)
	}
	return &o, nil
}

func (r *OrderRepository) GetOrderByCheckoutID(ctx context.Context, checkoutID string) (*models.Order, error) {
	query := `
		SELECT id, order_number, customer_email, customer_first_name, customer_last_name,
		       COALESCE(customer_phone, ''), COALESCE(date_of_birth, ''), wants_camping, wants_refund_insurance, total_cents, status,
		       COALESCE(helloasso_checkout_id, ''), COALESCE(helloasso_payment_id, ''),
		       COALESCE(helloasso_checkout_url, ''), created_at, updated_at, paid_at, confirmed_at
		FROM orders
		WHERE helloasso_checkout_id = $1`

	var o models.Order
	err := r.pool.QueryRow(ctx, query, checkoutID).Scan(
		&o.ID, &o.OrderNumber, &o.CustomerEmail, &o.CustomerFirstName, &o.CustomerLastName,
		&o.CustomerPhone, &o.DateOfBirth, &o.WantsCamping, &o.WantsRefundInsurance, &o.TotalCents, &o.Status, &o.HelloAssoCheckoutID, &o.HelloAssoPaymentID,
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

func (r *OrderRepository) ListPaidConfirmedOrderIDsWithTickets(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT o.id
		FROM orders o
		WHERE o.status IN ('paid', 'confirmed')
		  AND EXISTS (
			SELECT 1
			FROM tickets t
			WHERE t.order_id = o.id
		  )
		ORDER BY o.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("erreur query orders resend emails: %w", err)
	}
	defer rows.Close()

	orderIDs := make([]string, 0)
	for rows.Next() {
		var orderID string
		if err := rows.Scan(&orderID); err != nil {
			return nil, fmt.Errorf("erreur scan order id resend emails: %w", err)
		}
		orderIDs = append(orderIDs, orderID)
	}

	return orderIDs, nil
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
		       COALESCE(customer_phone, ''), COALESCE(date_of_birth, ''), wants_camping, wants_refund_insurance, total_cents, status,
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
			&o.CustomerPhone, &o.DateOfBirth, &o.WantsCamping, &o.WantsRefundInsurance, &o.TotalCents, &o.Status, &o.HelloAssoCheckoutID, &o.HelloAssoPaymentID,
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
			COUNT(*) FILTER (
				WHERE o.status IN ('paid', 'confirmed')
				AND NOT EXISTS (
					SELECT 1
					FROM tickets t
					JOIN bus_tickets bt ON bt.ticket_id = t.id
					WHERE t.order_id = o.id
				)
			),
			COUNT(*) FILTER (
				WHERE o.status IN ('paid', 'confirmed')
				  AND o.wants_refund_insurance = true
				AND NOT EXISTS (
					SELECT 1
					FROM tickets t
					JOIN bus_tickets bt ON bt.ticket_id = t.id
					WHERE t.order_id = o.id
				)
			),
			COALESCE(SUM(o.total_cents) FILTER (
				WHERE o.status IN ('paid', 'confirmed')
				AND NOT EXISTS (
					SELECT 1
					FROM tickets t
					JOIN bus_tickets bt ON bt.ticket_id = t.id
					WHERE t.order_id = o.id
				)
			), 0)
		FROM orders o
	`).Scan(&stats.TotalOrders, &stats.TotalRefundInsurance, &stats.TotalRevenueCents)
	if err != nil {
		return nil, fmt.Errorf("erreur stats globales: %w", err)
	}

	// Tickets vendus et validés
	err = r.pool.QueryRow(ctx, `
		SELECT 
			COUNT(*) FILTER (WHERE o.status IN ('paid', 'confirmed')),
			COUNT(*) FILTER (WHERE t.is_validated = true AND bt.ticket_id IS NULL),
			COUNT(*) FILTER (WHERE o.status IN ('paid', 'confirmed') AND t.is_camping = true AND bt.ticket_id IS NULL)
		FROM tickets t
		JOIN orders o ON o.id = t.order_id
		LEFT JOIN bus_tickets bt ON bt.ticket_id = t.id
	`).Scan(&stats.TotalTicketsSold, &stats.TotalValidated, &stats.TotalCamping)
	if err != nil {
		return nil, fmt.Errorf("erreur stats tickets: %w", err)
	}

	// Stats par type de ticket
	rows, err := r.pool.Query(ctx, `
		SELECT 
			tt.id, tt.name, tt.price_cents, tt.quantity_total, tt.quantity_sold,
			COUNT(t.id) FILTER (WHERE t.is_validated = true AND bt.ticket_id IS NULL) as validated,
			COALESCE(SUM(tt.price_cents) FILTER (WHERE o.status IN ('paid', 'confirmed')), 0) as revenue
		FROM ticket_types tt
		LEFT JOIN tickets t ON t.ticket_type_id = tt.id
		LEFT JOIN orders o ON o.id = t.order_id
		LEFT JOIN bus_tickets bt ON bt.ticket_id = t.id
		WHERE tt.description IS DISTINCT FROM 'Ticket navette festival'
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
			COALESCE(SUM((
				SELECT COUNT(*)
				FROM tickets t2
				LEFT JOIN bus_tickets bt2 ON bt2.ticket_id = t2.id
				WHERE t2.order_id = o.id AND bt2.ticket_id IS NULL
			)), 0) as ticket_count,
			COALESCE(SUM(o.total_cents), 0) as revenue
		FROM orders o
		WHERE o.status IN ('paid', 'confirmed')
		  AND NOT EXISTS (
			  SELECT 1
			  FROM tickets tb
			  JOIN bus_tickets btb ON btb.ticket_id = tb.id
			  WHERE tb.order_id = o.id
		  )
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

	stats.SalesTimeline = make(map[string][]models.SalesTimelinePoint)

	timeline1h, err := r.querySalesTimeline(ctx, "1 hour", "to_timestamp(floor(extract(epoch from o.created_at) / 300) * 300)")
	if err != nil {
		return nil, err
	}
	stats.SalesTimeline["1h"] = timeline1h

	timeline1d, err := r.querySalesTimeline(ctx, "1 day", "date_trunc('hour', o.created_at)")
	if err != nil {
		return nil, err
	}
	stats.SalesTimeline["1j"] = timeline1d

	timeline1w, err := r.querySalesTimeline(ctx, "7 days", "date_trunc('day', o.created_at)")
	if err != nil {
		return nil, err
	}
	stats.SalesTimeline["1semaine"] = timeline1w

	timeline1m, err := r.querySalesTimeline(ctx, "1 month", "date_trunc('day', o.created_at)")
	if err != nil {
		return nil, err
	}
	stats.SalesTimeline["1mois"] = timeline1m

	// 10 dernières commandes
	recentRows, err := r.pool.Query(ctx, `
		SELECT id, order_number, customer_email, customer_first_name, customer_last_name,
		       COALESCE(customer_phone, ''), wants_camping, wants_refund_insurance, total_cents, status,
		       COALESCE(helloasso_checkout_id, ''), COALESCE(helloasso_payment_id, ''),
		       COALESCE(helloasso_checkout_url, ''), created_at, updated_at, paid_at, confirmed_at
		FROM orders o
		WHERE status IN ('paid', 'confirmed')
		  AND NOT EXISTS (
			  SELECT 1
			  FROM tickets tb
			  JOIN bus_tickets btb ON btb.ticket_id = tb.id
			  WHERE tb.order_id = o.id
		  )
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
			&o.CustomerPhone, &o.WantsCamping, &o.WantsRefundInsurance, &o.TotalCents, &o.Status, &o.HelloAssoCheckoutID, &o.HelloAssoPaymentID,
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

func (r *OrderRepository) SaveOrderItems(ctx context.Context, tx pgx.Tx, orderID string, items []models.CheckoutItem) error {
	queryWithAttendees := `
		INSERT INTO order_items (order_id, ticket_type_id, category_id, quantity, attendees_json)
		VALUES ($1, $2, $3, $4, $5)`
	queryLegacy := `
		INSERT INTO order_items (order_id, ticket_type_id, category_id, quantity)
		VALUES ($1, $2, $3, $4)`

	hasAttendeesJSON, err := r.orderItemsHasAttendeesJSONColumn(ctx, tx)
	if err != nil {
		return fmt.Errorf("erreur détection schéma order_items: %w", err)
	}

	for _, item := range items {
		var categoryID interface{}
		if item.CategoryID != "" {
			categoryID = item.CategoryID
		}

		if !hasAttendeesJSON {
			if _, err := tx.Exec(ctx, queryLegacy, orderID, item.TicketTypeID, categoryID, item.Quantity); err != nil {
				return fmt.Errorf("erreur insertion order_item: %w", err)
			}
			continue
		}

		attendeesJSON, err := json.Marshal(item.Attendees)
		if err != nil {
			return fmt.Errorf("erreur sérialisation attendees order_item: %w", err)
		}

		if _, err := tx.Exec(ctx, queryWithAttendees, orderID, item.TicketTypeID, categoryID, item.Quantity, attendeesJSON); err != nil {
			return fmt.Errorf("erreur insertion order_item: %w", err)
		}
	}

	return nil
}

func (r *OrderRepository) GetOrderItems(ctx context.Context, orderID string) ([]models.CheckoutItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ticket_type_id, COALESCE(category_id::text, ''), quantity, COALESCE(attendees_json, '[]'::jsonb)
		FROM order_items
		WHERE order_id = $1`, orderID)
	if err != nil {
		if !isMissingAttendeesJSONColumn(err) {
			return nil, fmt.Errorf("erreur query order_items: %w", err)
		}

		legacyRows, legacyErr := r.pool.Query(ctx, `
			SELECT ticket_type_id, COALESCE(category_id::text, ''), quantity
			FROM order_items
			WHERE order_id = $1`, orderID)
		if legacyErr != nil {
			return nil, fmt.Errorf("erreur query order_items (legacy): %w", legacyErr)
		}
		defer legacyRows.Close()

		items := make([]models.CheckoutItem, 0)
		for legacyRows.Next() {
			var item models.CheckoutItem
			if scanErr := legacyRows.Scan(&item.TicketTypeID, &item.CategoryID, &item.Quantity); scanErr != nil {
				return nil, fmt.Errorf("erreur scan order_item (legacy): %w", scanErr)
			}
			items = append(items, item)
		}
		if legacyRowsErr := legacyRows.Err(); legacyRowsErr != nil {
			return nil, fmt.Errorf("erreur lecture order_items (legacy): %w", legacyRowsErr)
		}

		return items, nil
	}
	defer rows.Close()

	items := make([]models.CheckoutItem, 0)
	for rows.Next() {
		var item models.CheckoutItem
		var attendeesRaw []byte
		if err := rows.Scan(&item.TicketTypeID, &item.CategoryID, &item.Quantity, &attendeesRaw); err != nil {
			return nil, fmt.Errorf("erreur scan order_item: %w", err)
		}
		if len(attendeesRaw) > 0 {
			if err := json.Unmarshal(attendeesRaw, &item.Attendees); err != nil {
				return nil, fmt.Errorf("erreur parsing attendees order_item: %w", err)
			}
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erreur lecture order_items: %w", err)
	}

	return items, nil
}

func (r *OrderRepository) orderItemsHasAttendeesJSONColumn(ctx context.Context, tx pgx.Tx) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name = 'order_items'
			  AND column_name = 'attendees_json'
		)
	`).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func isMissingAttendeesJSONColumn(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "attendees_json") && strings.Contains(msg, "42703")
}

func (r *OrderRepository) CountFestivalTicketsByEmail(ctx context.Context, email string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM tickets t
		JOIN orders o ON o.id = t.order_id
		LEFT JOIN bus_tickets bt ON bt.ticket_id = t.id
		WHERE LOWER(o.customer_email) = LOWER($1)
		  AND o.status IN ('pending', 'paid', 'confirmed')
		  AND bt.ticket_id IS NULL
	`, strings.TrimSpace(email)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("erreur comptage tickets festival par email: %w", err)
	}
	return count, nil
}

func (r *OrderRepository) CountFestivalTicketsByTypeAndEmail(ctx context.Context, ticketTypeID, email string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM tickets t
		JOIN orders o ON o.id = t.order_id
		LEFT JOIN bus_tickets bt ON bt.ticket_id = t.id
		WHERE t.ticket_type_id = $1
		  AND o.status IN ('pending', 'paid', 'confirmed')
		  AND bt.ticket_id IS NULL
		  AND LOWER(COALESCE(NULLIF(t.attendee_email, ''), o.customer_email)) = LOWER($2)
	`, ticketTypeID, strings.TrimSpace(email)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("erreur comptage tickets festival par type/email: %w", err)
	}
	return count, nil
}

func (r *OrderRepository) GetExpiredPendingOrderIDs(ctx context.Context, olderThan time.Time) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id
		FROM orders
		WHERE status = 'pending' AND created_at < $1`, olderThan)
	if err != nil {
		return nil, fmt.Errorf("erreur query commandes expirées: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("erreur scan commande expirée: %w", err)
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erreur lecture commandes expirées: %w", err)
	}

	return ids, nil
}

func (r *OrderRepository) CreateReferralLink(ctx context.Context, name string) (*models.ReferralPublicInfo, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return nil, fmt.Errorf("nom de lien requis")
	}

	for attempt := 0; attempt < 5; attempt++ {
		code, err := randomReferralCode()
		if err != nil {
			return nil, fmt.Errorf("erreur génération code parrainage: %w", err)
		}

		var link models.ReferralPublicInfo
		err = r.pool.QueryRow(ctx, `
			INSERT INTO referral_links (name, code, is_active)
			VALUES ($1, $2, true)
			RETURNING id, code, name, is_active
		`, trimmedName, code).Scan(&link.ID, &link.Code, &link.Name, &link.IsActive)
		if err == nil {
			return &link, nil
		}
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			continue
		}
		return nil, fmt.Errorf("erreur création lien parrainage: %w", err)
	}

	return nil, fmt.Errorf("impossible de générer un code de parrainage unique")
}

func (r *OrderRepository) GetReferralLinkByCode(ctx context.Context, code string) (*models.ReferralPublicInfo, error) {
	trimmedCode := strings.TrimSpace(strings.ToLower(code))
	if trimmedCode == "" {
		return nil, nil
	}

	var link models.ReferralPublicInfo
	err := r.pool.QueryRow(ctx, `
		SELECT id, code, name, is_active
		FROM referral_links
		WHERE code = $1
		LIMIT 1
	`, trimmedCode).Scan(&link.ID, &link.Code, &link.Name, &link.IsActive)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("erreur récupération lien parrainage: %w", err)
	}

	return &link, nil
}

func (r *OrderRepository) RecordReferralClick(ctx context.Context, referralLinkID, visitorID, ipAddress, userAgent string) error {
	if strings.TrimSpace(referralLinkID) == "" || strings.TrimSpace(visitorID) == "" {
		return nil
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO referral_clicks (referral_link_id, visitor_id, ip_address, user_agent)
		VALUES ($1, $2, NULLIF($3, '')::inet, NULLIF($4, ''))
	`, referralLinkID, visitorID, ipAddress, userAgent)
	if err != nil {
		return fmt.Errorf("erreur enregistrement clic parrainage: %w", err)
	}

	return nil
}

func (r *OrderRepository) AttachReferralConversion(ctx context.Context, tx pgx.Tx, orderID, referralLinkID, visitorID string) error {
	if strings.TrimSpace(orderID) == "" || strings.TrimSpace(referralLinkID) == "" {
		return nil
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO referral_order_conversions (order_id, referral_link_id, visitor_id)
		VALUES ($1, $2, NULLIF($3, ''))
		ON CONFLICT (order_id) DO NOTHING
	`, orderID, referralLinkID, visitorID)
	if err != nil {
		return fmt.Errorf("erreur liaison conversion parrainage: %w", err)
	}

	return nil
}

func (r *OrderRepository) ListReferralLinks(ctx context.Context) ([]models.ReferralLinkRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			rl.id,
			rl.name,
			rl.code,
			rl.is_active,
			rl.created_at,
			(
				SELECT COUNT(*)
				FROM referral_clicks c
				WHERE c.referral_link_id = rl.id
			) AS click_count,
			(
				SELECT COUNT(DISTINCT c.visitor_id)
				FROM referral_clicks c
				WHERE c.referral_link_id = rl.id
			) AS unique_visitors,
			(
				SELECT COUNT(*)
				FROM referral_order_conversions roc
				JOIN orders o ON o.id = roc.order_id
				WHERE roc.referral_link_id = rl.id
				  AND o.status IN ('paid', 'confirmed')
				  AND EXISTS (
					  SELECT 1
					  FROM tickets t
					  LEFT JOIN bus_tickets bt ON bt.ticket_id = t.id
					  WHERE t.order_id = o.id
					    AND bt.ticket_id IS NULL
				  )
			) AS converted_orders,
			(
				SELECT COUNT(*)
				FROM referral_order_conversions roc
				JOIN orders o ON o.id = roc.order_id
				JOIN tickets t ON t.order_id = o.id
				LEFT JOIN bus_tickets bt ON bt.ticket_id = t.id
				WHERE roc.referral_link_id = rl.id
				  AND o.status IN ('paid', 'confirmed')
				  AND bt.ticket_id IS NULL
			) AS converted_tickets,
			(
				SELECT COALESCE(SUM(o.total_cents), 0)
				FROM referral_order_conversions roc
				JOIN orders o ON o.id = roc.order_id
				WHERE roc.referral_link_id = rl.id
				  AND o.status IN ('paid', 'confirmed')
				  AND EXISTS (
					  SELECT 1
					  FROM tickets t
					  LEFT JOIN bus_tickets bt ON bt.ticket_id = t.id
					  WHERE t.order_id = o.id
					    AND bt.ticket_id IS NULL
				  )
			) AS converted_revenue_cents
		FROM referral_links rl
		ORDER BY rl.created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("erreur récupération liens parrainage: %w", err)
	}
	defer rows.Close()

	out := make([]models.ReferralLinkRow, 0)
	for rows.Next() {
		var row models.ReferralLinkRow
		if scanErr := rows.Scan(
			&row.ID,
			&row.Name,
			&row.Code,
			&row.IsActive,
			&row.CreatedAt,
			&row.ClickCount,
			&row.UniqueVisitors,
			&row.ConvertedOrders,
			&row.ConvertedTickets,
			&row.ConvertedRevenue,
		); scanErr != nil {
			return nil, fmt.Errorf("erreur lecture lien parrainage: %w", scanErr)
		}

		daily, dailyErr := r.getReferralDailySalesByLink(ctx, row.ID)
		if dailyErr != nil {
			return nil, dailyErr
		}
		row.DailySalesByDay = daily

		out = append(out, row)
	}

	if rows.Err() != nil {
		return nil, fmt.Errorf("erreur parcours liens parrainage: %w", rows.Err())
	}

	return out, nil
}

func (r *OrderRepository) getReferralDailySalesByLink(ctx context.Context, referralLinkID string) ([]models.DailyReferralSales, error) {
	rows, err := r.pool.Query(ctx, `
		WITH clicks_by_day AS (
			SELECT
				DATE(c.created_at) AS d,
				COUNT(*) AS click_count
			FROM referral_clicks c
			WHERE c.referral_link_id = $1
			GROUP BY DATE(c.created_at)
		),
		tickets_by_day AS (
			SELECT
				DATE(roc.created_at) AS d,
				COUNT(*) AS ticket_count
			FROM referral_order_conversions roc
			JOIN orders o ON o.id = roc.order_id
			JOIN tickets t ON t.order_id = o.id
			LEFT JOIN bus_tickets bt ON bt.ticket_id = t.id
			WHERE roc.referral_link_id = $1
			  AND o.status IN ('paid', 'confirmed')
			  AND bt.ticket_id IS NULL
			GROUP BY DATE(roc.created_at)
		)
		SELECT
			COALESCE(c.d, t.d)::text AS sale_date,
			COALESCE(c.click_count, 0) AS click_count,
			COALESCE(t.ticket_count, 0) AS ticket_count
		FROM clicks_by_day c
		FULL OUTER JOIN tickets_by_day t ON t.d = c.d
		ORDER BY COALESCE(c.d, t.d) DESC
		LIMIT 30
	`, referralLinkID)
	if err != nil {
		return nil, fmt.Errorf("erreur stats par jour par lien: %w", err)
	}
	defer rows.Close()

	out := make([]models.DailyReferralSales, 0)
	for rows.Next() {
		var d models.DailyReferralSales
		if scanErr := rows.Scan(&d.Date, &d.ClickCount, &d.TicketCount); scanErr != nil {
			return nil, fmt.Errorf("erreur scan stats par jour par lien: %w", scanErr)
		}
		out = append(out, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erreur lecture stats par jour par lien: %w", err)
	}

	return out, nil
}

func (r *OrderRepository) querySalesTimeline(ctx context.Context, intervalLiteral, bucketExpr string) ([]models.SalesTimelinePoint, error) {
	query := fmt.Sprintf(`
		SELECT
			%s AS bucket,
			COALESCE(SUM(o.total_cents), 0) AS revenue,
			COALESCE(SUM((
				SELECT COUNT(*)
				FROM tickets t2
				LEFT JOIN bus_tickets bt2 ON bt2.ticket_id = t2.id
				WHERE t2.order_id = o.id AND bt2.ticket_id IS NULL
			)), 0) AS ticket_count
		FROM orders o
		WHERE o.status IN ('paid', 'confirmed')
		  AND o.created_at >= NOW() - INTERVAL '%s'
		  AND NOT EXISTS (
			  SELECT 1
			  FROM tickets tb
			  JOIN bus_tickets btb ON btb.ticket_id = tb.id
			  WHERE tb.order_id = o.id
		  )
		GROUP BY bucket
		ORDER BY bucket ASC
	`, bucketExpr, intervalLiteral)

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("erreur série ventes (%s): %w", intervalLiteral, err)
	}
	defer rows.Close()

	out := make([]models.SalesTimelinePoint, 0)
	for rows.Next() {
		var bucket time.Time
		var point models.SalesTimelinePoint
		if scanErr := rows.Scan(&bucket, &point.RevenueCents, &point.TicketCount); scanErr != nil {
			return nil, fmt.Errorf("erreur lecture série ventes (%s): %w", intervalLiteral, scanErr)
		}
		point.Bucket = bucket.Format(time.RFC3339)
		out = append(out, point)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erreur parcours série ventes (%s): %w", intervalLiteral, err)
	}

	return out, nil
}

func randomReferralCode() (string, error) {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.ToLower(hex.EncodeToString(b)), nil
}
