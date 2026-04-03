package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kreton/if-festival/internal/models"
)

type TicketRepository struct {
	pool *pgxpool.Pool
}

func NewTicketRepository(pool *pgxpool.Pool) *TicketRepository {
	return &TicketRepository{pool: pool}
}

// ============================================
// Ticket Types
// ============================================

func (r *TicketRepository) GetActiveTicketTypes(ctx context.Context) ([]models.TicketType, error) {
	query := `
		SELECT id, name, description, price_cents, quantity_total, quantity_sold,
		       sale_start, sale_end, is_active, is_masked, max_per_order, one_ticket_per_email, COALESCE(allowed_domains, '{}'), created_at, updated_at
		FROM ticket_types
		WHERE is_active = true
		  AND is_masked = false
		  AND COALESCE(description, '') <> 'Ticket navette festival'
		  AND id NOT IN (
			SELECT DISTINCT t.ticket_type_id
			FROM tickets t
			JOIN bus_tickets bt ON bt.ticket_id = t.id
		  )
		ORDER BY price_cents ASC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("erreur query ticket types: %w", err)
	}
	defer rows.Close()

	var types []models.TicketType
	for rows.Next() {
		var tt models.TicketType
		err := rows.Scan(
			&tt.ID, &tt.Name, &tt.Description, &tt.PriceCents,
			&tt.QuantityTotal, &tt.QuantitySold, &tt.SaleStart, &tt.SaleEnd,
			&tt.IsActive, &tt.IsMasked, &tt.MaxPerOrder, &tt.OneTicketPerEmail, &tt.AllowedDomains, &tt.CreatedAt, &tt.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("erreur scan ticket type: %w", err)
		}
		types = append(types, tt)
	}

	return types, nil
}

// GetAllTicketTypes returns all ticket types (including inactive and masked) for admin
func (r *TicketRepository) GetAllTicketTypes(ctx context.Context) ([]models.TicketType, error) {
	query := `
		SELECT id, name, description, price_cents, quantity_total, quantity_sold,
		       sale_start, sale_end, is_active, is_masked, max_per_order, one_ticket_per_email, COALESCE(allowed_domains, '{}'), created_at, updated_at
		FROM ticket_types
		WHERE COALESCE(description, '') <> 'Ticket navette festival'
		  AND id NOT IN (
			SELECT DISTINCT t.ticket_type_id
			FROM tickets t
			JOIN bus_tickets bt ON bt.ticket_id = t.id
		  )
		ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("erreur query all ticket types: %w", err)
	}
	defer rows.Close()

	var types []models.TicketType
	for rows.Next() {
		var tt models.TicketType
		err := rows.Scan(
			&tt.ID, &tt.Name, &tt.Description, &tt.PriceCents,
			&tt.QuantityTotal, &tt.QuantitySold, &tt.SaleStart, &tt.SaleEnd,
			&tt.IsActive, &tt.IsMasked, &tt.MaxPerOrder, &tt.OneTicketPerEmail, &tt.AllowedDomains, &tt.CreatedAt, &tt.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("erreur scan ticket type: %w", err)
		}
		types = append(types, tt)
	}

	return types, nil
}

func (r *TicketRepository) GetTicketTypeByID(ctx context.Context, id string) (*models.TicketType, error) {
	query := `
		SELECT id, name, description, price_cents, quantity_total, quantity_sold,
		       sale_start, sale_end, is_active, is_masked, max_per_order, one_ticket_per_email, COALESCE(allowed_domains, '{}'), created_at, updated_at
		FROM ticket_types
		WHERE id = $1`

	var tt models.TicketType
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&tt.ID, &tt.Name, &tt.Description, &tt.PriceCents,
		&tt.QuantityTotal, &tt.QuantitySold, &tt.SaleStart, &tt.SaleEnd,
		&tt.IsActive, &tt.IsMasked, &tt.MaxPerOrder, &tt.OneTicketPerEmail, &tt.AllowedDomains, &tt.CreatedAt, &tt.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("erreur query ticket type: %w", err)
	}

	return &tt, nil
}

func (r *TicketRepository) CreateTicketType(ctx context.Context, req models.CreateTicketTypeRequest) (*models.TicketType, error) {
	query := `
		INSERT INTO ticket_types (name, description, price_cents, quantity_total, sale_start, sale_end, max_per_order, one_ticket_per_email, allowed_domains)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, name, description, price_cents, quantity_total, quantity_sold,
		          sale_start, sale_end, is_active, is_masked, max_per_order, one_ticket_per_email, COALESCE(allowed_domains, '{}'), created_at, updated_at`

	var tt models.TicketType
	err := r.pool.QueryRow(ctx, query,
		req.Name, req.Description, req.PriceCents, req.QuantityTotal,
		req.SaleStart, req.SaleEnd, req.MaxPerOrder, req.OneTicketPerEmail, req.AllowedDomains,
	).Scan(
		&tt.ID, &tt.Name, &tt.Description, &tt.PriceCents,
		&tt.QuantityTotal, &tt.QuantitySold, &tt.SaleStart, &tt.SaleEnd,
		&tt.IsActive, &tt.IsMasked, &tt.MaxPerOrder, &tt.OneTicketPerEmail, &tt.AllowedDomains, &tt.CreatedAt, &tt.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("erreur création ticket type: %w", err)
	}

	return &tt, nil
}

// UpdateTicketType updates editable fields of a ticket type
func (r *TicketRepository) UpdateTicketType(ctx context.Context, id string, req models.UpdateTicketTypeRequest) (*models.TicketType, error) {
	// Verify quantity_total is not reduced below quantity_sold
	var currentSold int
	err := r.pool.QueryRow(ctx, `SELECT quantity_sold FROM ticket_types WHERE id = $1`, id).Scan(&currentSold)
	if err != nil {
		return nil, fmt.Errorf("ticket type introuvable: %w", err)
	}
	if req.QuantityTotal < currentSold {
		return nil, fmt.Errorf("impossible de réduire à %d : %d tickets déjà vendus", req.QuantityTotal, currentSold)
	}

	query := `
		UPDATE ticket_types
		SET name = $2, description = $3, price_cents = $4, quantity_total = $5,
		    sale_start = $6, sale_end = $7, one_ticket_per_email = $8, allowed_domains = $9
		WHERE id = $1
		RETURNING id, name, description, price_cents, quantity_total, quantity_sold,
		          sale_start, sale_end, is_active, is_masked, max_per_order, one_ticket_per_email, COALESCE(allowed_domains, '{}'), created_at, updated_at`

	var tt models.TicketType
	err = r.pool.QueryRow(ctx, query, id,
		req.Name, req.Description, req.PriceCents, req.QuantityTotal,
		req.SaleStart, req.SaleEnd, req.OneTicketPerEmail, req.AllowedDomains,
	).Scan(
		&tt.ID, &tt.Name, &tt.Description, &tt.PriceCents,
		&tt.QuantityTotal, &tt.QuantitySold, &tt.SaleStart, &tt.SaleEnd,
		&tt.IsActive, &tt.IsMasked, &tt.MaxPerOrder, &tt.OneTicketPerEmail, &tt.AllowedDomains, &tt.CreatedAt, &tt.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("erreur mise à jour ticket type: %w", err)
	}
	return &tt, nil
}

// ToggleTicketTypeMask toggles the is_masked flag on a ticket type
func (r *TicketRepository) ToggleTicketTypeMask(ctx context.Context, id string) (*models.TicketType, error) {
	query := `
		UPDATE ticket_types SET is_masked = NOT is_masked
		WHERE id = $1
		RETURNING id, name, description, price_cents, quantity_total, quantity_sold,
		          sale_start, sale_end, is_active, is_masked, max_per_order, one_ticket_per_email, COALESCE(allowed_domains, '{}'), created_at, updated_at`

	var tt models.TicketType
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&tt.ID, &tt.Name, &tt.Description, &tt.PriceCents,
		&tt.QuantityTotal, &tt.QuantitySold, &tt.SaleStart, &tt.SaleEnd,
		&tt.IsActive, &tt.IsMasked, &tt.MaxPerOrder, &tt.OneTicketPerEmail, &tt.AllowedDomains, &tt.CreatedAt, &tt.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("erreur toggle mask ticket type: %w", err)
	}
	return &tt, nil
}

// ToggleCategoryMask toggles the is_masked flag on a category
func (r *TicketRepository) ToggleCategoryMask(ctx context.Context, categoryID string) (*models.TicketCategory, error) {
	query := `
		UPDATE ticket_categories SET is_masked = NOT is_masked
		WHERE id = $1
		RETURNING id, ticket_type_id, name, quantity_allocated, quantity_sold,
		          is_masked, is_checkbox, COALESCE(allowed_domains, '{}'), created_at, updated_at`

	var c models.TicketCategory
	err := r.pool.QueryRow(ctx, query, categoryID).Scan(
		&c.ID, &c.TicketTypeID, &c.Name, &c.QuantityAllocated,
		&c.QuantitySold, &c.IsMasked, &c.IsCheckbox, &c.AllowedDomains, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("erreur toggle mask catégorie: %w", err)
	}
	return &c, nil
}

// ToggleCategoryCheckbox toggles checkbox mode for one category (unique per ticket type)
func (r *TicketRepository) ToggleCategoryCheckbox(ctx context.Context, categoryID string) (*models.TicketCategory, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("erreur début transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var ticketTypeID string
	var currentlyChecked bool
	err = tx.QueryRow(ctx,
		`SELECT ticket_type_id, is_checkbox FROM ticket_categories WHERE id = $1 FOR UPDATE`,
		categoryID,
	).Scan(&ticketTypeID, &currentlyChecked)
	if err != nil {
		return nil, fmt.Errorf("catégorie introuvable: %w", err)
	}

	if currentlyChecked {
		_, err = tx.Exec(ctx,
			`UPDATE ticket_categories SET is_checkbox = false, updated_at = NOW() WHERE id = $1`,
			categoryID,
		)
		if err != nil {
			return nil, fmt.Errorf("erreur mise à jour catégorie checkbox: %w", err)
		}
	} else {
		_, err = tx.Exec(ctx,
			`UPDATE ticket_categories SET is_checkbox = false, updated_at = NOW() WHERE ticket_type_id = $1 AND is_checkbox = true`,
			ticketTypeID,
		)
		if err != nil {
			return nil, fmt.Errorf("erreur reset catégorie checkbox: %w", err)
		}

		_, err = tx.Exec(ctx,
			`UPDATE ticket_categories SET is_checkbox = true, updated_at = NOW() WHERE id = $1`,
			categoryID,
		)
		if err != nil {
			return nil, fmt.Errorf("erreur activation catégorie checkbox: %w", err)
		}
	}

	var c models.TicketCategory
	err = tx.QueryRow(ctx, `
		SELECT id, ticket_type_id, name, quantity_allocated, quantity_sold,
		       is_masked, is_checkbox, COALESCE(allowed_domains, '{}'), created_at, updated_at
		FROM ticket_categories
		WHERE id = $1`, categoryID).Scan(
		&c.ID, &c.TicketTypeID, &c.Name, &c.QuantityAllocated,
		&c.QuantitySold, &c.IsMasked, &c.IsCheckbox, &c.AllowedDomains, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("erreur lecture catégorie mise à jour: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("erreur commit transaction: %w", err)
	}

	return &c, nil
}

// ============================================
// Catégories
// ============================================

func (r *TicketRepository) GetCategoriesByTicketType(ctx context.Context, ticketTypeID string) ([]models.TicketCategory, error) {
	query := `
		SELECT id, ticket_type_id, name, quantity_allocated, quantity_sold,
		       is_masked, is_checkbox, COALESCE(allowed_domains, '{}'), created_at, updated_at
		FROM ticket_categories
		WHERE ticket_type_id = $1
		ORDER BY name ASC`

	rows, err := r.pool.Query(ctx, query, ticketTypeID)
	if err != nil {
		return nil, fmt.Errorf("erreur query categories: %w", err)
	}
	defer rows.Close()

	var cats []models.TicketCategory
	for rows.Next() {
		var c models.TicketCategory
		err := rows.Scan(&c.ID, &c.TicketTypeID, &c.Name, &c.QuantityAllocated,
			&c.QuantitySold, &c.IsMasked, &c.IsCheckbox, &c.AllowedDomains, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("erreur scan category: %w", err)
		}
		cats = append(cats, c)
	}
	return cats, nil
}

func (r *TicketRepository) CreateCategory(ctx context.Context, req models.CreateCategoryRequest) (*models.TicketCategory, error) {
	// Vérifier que le total alloué ne dépasse pas quantity_total du ticket type
	var totalAllocated, quantityTotal int
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(quantity_allocated), 0) FROM ticket_categories WHERE ticket_type_id = $1`,
		req.TicketTypeID).Scan(&totalAllocated)
	if err != nil {
		return nil, fmt.Errorf("erreur vérification allocation: %w", err)
	}
	err = r.pool.QueryRow(ctx,
		`SELECT quantity_total FROM ticket_types WHERE id = $1`,
		req.TicketTypeID).Scan(&quantityTotal)
	if err != nil {
		return nil, fmt.Errorf("erreur récupération quantité totale: %w", err)
	}
	if totalAllocated+req.Quantity > quantityTotal {
		return nil, fmt.Errorf("dépassement : %d alloués + %d demandés > %d total", totalAllocated, req.Quantity, quantityTotal)
	}

	query := `
		INSERT INTO ticket_categories (ticket_type_id, name, quantity_allocated, allowed_domains)
		VALUES ($1, $2, $3, $4)
		RETURNING id, ticket_type_id, name, quantity_allocated, quantity_sold,
		          is_masked, is_checkbox, COALESCE(allowed_domains, '{}'), created_at, updated_at`

	var c models.TicketCategory
	err = r.pool.QueryRow(ctx, query, req.TicketTypeID, req.Name, req.Quantity, req.AllowedDomains).Scan(
		&c.ID, &c.TicketTypeID, &c.Name, &c.QuantityAllocated,
		&c.QuantitySold, &c.IsMasked, &c.IsCheckbox, &c.AllowedDomains, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("erreur création catégorie: %w", err)
	}
	return &c, nil
}

func (r *TicketRepository) ReallocateCategories(ctx context.Context, req models.ReallocateCategoryRequest) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("erreur début transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock both categories
	var srcAllocated, srcSold int
	var srcTypeID string
	err = tx.QueryRow(ctx,
		`SELECT ticket_type_id, quantity_allocated, quantity_sold FROM ticket_categories WHERE id = $1 FOR UPDATE`,
		req.SourceCategoryID).Scan(&srcTypeID, &srcAllocated, &srcSold)
	if err != nil {
		return fmt.Errorf("catégorie source introuvable: %w", err)
	}

	var dstTypeID string
	err = tx.QueryRow(ctx,
		`SELECT ticket_type_id FROM ticket_categories WHERE id = $1 FOR UPDATE`,
		req.TargetCategoryID).Scan(&dstTypeID)
	if err != nil {
		return fmt.Errorf("catégorie cible introuvable: %w", err)
	}

	if srcTypeID != dstTypeID {
		return fmt.Errorf("les deux catégories doivent appartenir au même type de ticket")
	}

	// Vérifier qu'il y a assez de places non vendues dans la source
	availableToMove := srcAllocated - srcSold
	if req.Quantity > availableToMove {
		return fmt.Errorf("impossible de déplacer %d places : seulement %d non vendues dans la source", req.Quantity, availableToMove)
	}

	// Vérifier que le total ne dépasse pas quantity_total du ticket type
	var ttTotal int
	err = tx.QueryRow(ctx, `SELECT quantity_total FROM ticket_types WHERE id = $1 FOR UPDATE`, srcTypeID).Scan(&ttTotal)
	if err != nil {
		return fmt.Errorf("erreur récupération ticket type: %w", err)
	}

	// Diminuer la source, augmenter la cible
	_, err = tx.Exec(ctx,
		`UPDATE ticket_categories SET quantity_allocated = quantity_allocated - $1, updated_at = NOW() WHERE id = $2`,
		req.Quantity, req.SourceCategoryID)
	if err != nil {
		return fmt.Errorf("erreur mise à jour source: %w", err)
	}

	_, err = tx.Exec(ctx,
		`UPDATE ticket_categories SET quantity_allocated = quantity_allocated + $1, updated_at = NOW() WHERE id = $2`,
		req.Quantity, req.TargetCategoryID)
	if err != nil {
		return fmt.Errorf("erreur mise à jour cible: %w", err)
	}

	return tx.Commit(ctx)
}

func (r *TicketRepository) DeleteCategory(ctx context.Context, categoryID string) error {
	// Only allow deleting empty categories (0 sold)
	var sold int
	err := r.pool.QueryRow(ctx, `SELECT quantity_sold FROM ticket_categories WHERE id = $1`, categoryID).Scan(&sold)
	if err != nil {
		return fmt.Errorf("catégorie introuvable: %w", err)
	}
	if sold > 0 {
		return fmt.Errorf("impossible de supprimer : %d tickets déjà vendus", sold)
	}
	_, err = r.pool.Exec(ctx, `DELETE FROM ticket_categories WHERE id = $1`, categoryID)
	return err
}

func (r *TicketRepository) ReserveCategoryTickets(ctx context.Context, categoryID string, quantity int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("erreur début transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var remaining int
	err = tx.QueryRow(ctx,
		`SELECT quantity_allocated - quantity_sold FROM ticket_categories WHERE id = $1 FOR UPDATE`,
		categoryID).Scan(&remaining)
	if err != nil {
		return fmt.Errorf("erreur vérification stock catégorie: %w", err)
	}
	if remaining < quantity {
		return fmt.Errorf("stock insuffisant dans cette catégorie: %d restant(s), %d demandé(s)", remaining, quantity)
	}

	// Update category sold count
	_, err = tx.Exec(ctx,
		`UPDATE ticket_categories SET quantity_sold = quantity_sold + $1, updated_at = NOW() WHERE id = $2`,
		quantity, categoryID)
	if err != nil {
		return fmt.Errorf("erreur mise à jour stock catégorie: %w", err)
	}

	// Also update the ticket type's total sold count
	_, err = tx.Exec(ctx,
		`UPDATE ticket_types SET quantity_sold = quantity_sold + $1 WHERE id = (
			SELECT ticket_type_id FROM ticket_categories WHERE id = $2
		)`, quantity, categoryID)
	if err != nil {
		return fmt.Errorf("erreur mise à jour stock type: %w", err)
	}

	return tx.Commit(ctx)
}

// ReserveTickets réserve les tickets de manière atomique (avec lock)
func (r *TicketRepository) ReserveTickets(ctx context.Context, items []models.CheckoutItem) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("erreur début transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, item := range items {
		// SELECT FOR UPDATE pour verrouiller la ligne
		var remaining int
		err := tx.QueryRow(ctx,
			`SELECT quantity_total - quantity_sold FROM ticket_types WHERE id = $1 FOR UPDATE`,
			item.TicketTypeID,
		).Scan(&remaining)
		if err != nil {
			return fmt.Errorf("erreur vérification stock: %w", err)
		}

		if remaining < item.Quantity {
			return fmt.Errorf("stock insuffisant pour le type %s: %d restant(s), %d demandé(s)",
				item.TicketTypeID, remaining, item.Quantity)
		}

		// Mettre à jour le stock global
		_, err = tx.Exec(ctx,
			`UPDATE ticket_types SET quantity_sold = quantity_sold + $1 WHERE id = $2`,
			item.Quantity, item.TicketTypeID,
		)
		if err != nil {
			return fmt.Errorf("erreur mise à jour stock: %w", err)
		}

		// Mettre à jour le stock de la catégorie si spécifiée
		if item.CategoryID != "" {
			_, err = tx.Exec(ctx,
				`UPDATE ticket_categories SET quantity_sold = quantity_sold + $1 WHERE id = $2`,
				item.Quantity, item.CategoryID,
			)
			if err != nil {
				return fmt.Errorf("erreur mise à jour stock catégorie: %w", err)
			}
		}
	}

	return tx.Commit(ctx)
}

// ReleaseTickets libère les tickets réservés (en cas d'annulation)
func (r *TicketRepository) ReleaseTickets(ctx context.Context, items []models.CheckoutItem) error {
	for _, item := range items {
		_, err := r.pool.Exec(ctx,
			`UPDATE ticket_types SET quantity_sold = GREATEST(0, quantity_sold - $1) WHERE id = $2`,
			item.Quantity, item.TicketTypeID,
		)
		if err != nil {
			return fmt.Errorf("erreur libération stock: %w", err)
		}

		// Libérer aussi le stock catégorie si spécifié
		if item.CategoryID != "" {
			_, err = r.pool.Exec(ctx,
				`UPDATE ticket_categories SET quantity_sold = GREATEST(0, quantity_sold - $1) WHERE id = $2`,
				item.Quantity, item.CategoryID,
			)
			if err != nil {
				return fmt.Errorf("erreur libération stock catégorie: %w", err)
			}
		}
	}
	return nil
}

// ============================================
// Tickets (entrées individuelles)
// ============================================

func (r *TicketRepository) CreateTicket(ctx context.Context, tx pgx.Tx, ticket *models.Ticket) error {
	query := `
		INSERT INTO tickets (order_id, ticket_type_id, qr_token, qr_code_data, attendee_first_name, attendee_last_name, attendee_email, is_camping)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at`

	return tx.QueryRow(ctx, query,
		ticket.OrderID, ticket.TicketTypeID, ticket.QRToken,
		ticket.QRCodeData, ticket.AttendeeFirstName, ticket.AttendeeLastName, ticket.AttendeeEmail, ticket.IsCamping,
	).Scan(&ticket.ID, &ticket.CreatedAt)
}

func (r *TicketRepository) EnsureBusTicketType(ctx context.Context, tx pgx.Tx, name string) (*models.TicketType, error) {
	query := `
		SELECT id, name, description, price_cents, quantity_total, quantity_sold,
		       sale_start, sale_end, is_active, is_masked, max_per_order, one_ticket_per_email, COALESCE(allowed_domains, '{}'), created_at, updated_at
		FROM ticket_types
		WHERE name = $1
		LIMIT 1`

	var tt models.TicketType
	err := tx.QueryRow(ctx, query, name).Scan(
		&tt.ID, &tt.Name, &tt.Description, &tt.PriceCents, &tt.QuantityTotal, &tt.QuantitySold,
		&tt.SaleStart, &tt.SaleEnd, &tt.IsActive, &tt.IsMasked, &tt.MaxPerOrder, &tt.OneTicketPerEmail, &tt.AllowedDomains, &tt.CreatedAt, &tt.UpdatedAt,
	)
	if err == nil {
		if !tt.IsMasked || tt.Description != "Ticket navette festival" {
			if _, updateErr := tx.Exec(ctx, `
				UPDATE ticket_types
				SET description = 'Ticket navette festival', is_masked = true
				WHERE id = $1`, tt.ID); updateErr != nil {
				return nil, fmt.Errorf("erreur normalisation ticket type bus: %w", updateErr)
			}
			tt.Description = "Ticket navette festival"
			tt.IsMasked = true
		}
		return &tt, nil
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("erreur lecture ticket type bus: %w", err)
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO ticket_types (name, description, price_cents, quantity_total, quantity_sold, sale_start, sale_end, is_active, is_masked, max_per_order, one_ticket_per_email, allowed_domains)
		VALUES ($1, $2, 0, 999999, 0, NOW() - INTERVAL '1 day', NOW() + INTERVAL '10 years', true, true, 1, false, '{}')
		RETURNING id, name, description, price_cents, quantity_total, quantity_sold,
		          sale_start, sale_end, is_active, is_masked, max_per_order, one_ticket_per_email, COALESCE(allowed_domains, '{}'), created_at, updated_at`,
		name, "Ticket navette festival",
	).Scan(
		&tt.ID, &tt.Name, &tt.Description, &tt.PriceCents, &tt.QuantityTotal, &tt.QuantitySold,
		&tt.SaleStart, &tt.SaleEnd, &tt.IsActive, &tt.IsMasked, &tt.MaxPerOrder, &tt.OneTicketPerEmail, &tt.AllowedDomains, &tt.CreatedAt, &tt.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("erreur création ticket type bus: %w", err)
	}

	return &tt, nil
}

func (r *TicketRepository) GetTicketByQRToken(ctx context.Context, qrToken string) (*models.Ticket, error) {
	query := `
		SELECT t.id, t.order_id, t.ticket_type_id, t.qr_token, t.is_validated, t.is_camping,
		       t.validated_at, COALESCE(t.validated_by, ''), t.attendee_first_name, t.attendee_last_name, COALESCE(t.attendee_email, ''),
		       t.created_at, tt.name as ticket_type_name
		FROM tickets t
		JOIN ticket_types tt ON tt.id = t.ticket_type_id
		WHERE t.qr_token = $1`

	var t models.Ticket
	err := r.pool.QueryRow(ctx, query, qrToken).Scan(
		&t.ID, &t.OrderID, &t.TicketTypeID, &t.QRToken, &t.IsValidated, &t.IsCamping,
		&t.ValidatedAt, &t.ValidatedBy, &t.AttendeeFirstName, &t.AttendeeLastName, &t.AttendeeEmail,
		&t.CreatedAt, &t.TicketTypeName,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("erreur query ticket by QR: %w", err)
	}

	return &t, nil
}

func (r *TicketRepository) ValidateTicket(ctx context.Context, qrToken string, validatedBy string) (*models.ValidateQRResponse, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("erreur début transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Récupérer le ticket avec lock
	query := `
		SELECT t.id, t.order_id, t.is_validated, t.is_camping, t.attendee_first_name, t.attendee_last_name,
		       tt.name as ticket_type_name, o.order_number, o.status,
		       COALESCE(bt.from_station, ''), COALESCE(bt.to_station, ''),
		       od.departure_time, rd.departure_time, COALESCE(bt.is_round_trip, false)
		FROM tickets t
		JOIN ticket_types tt ON tt.id = t.ticket_type_id
		JOIN orders o ON o.id = t.order_id
		LEFT JOIN bus_tickets bt ON bt.ticket_id = t.id
		LEFT JOIN bus_departures od ON od.id = bt.outbound_departure_id
		LEFT JOIN bus_departures rd ON rd.id = bt.return_departure_id
		WHERE t.qr_token = $1
		FOR UPDATE OF t`

	var (
		ticketID          string
		orderID           string
		isValidated       bool
		isCamping         bool
		attendeeFirstName *string
		attendeeLastName  *string
		ticketTypeName    string
		orderNumber       string
		orderStatus       models.OrderStatus
		fromStation       string
		toStation         string
		outboundAt        *time.Time
		returnAt          *time.Time
		isRoundTrip       bool
	)

	err = tx.QueryRow(ctx, query, qrToken).Scan(
		&ticketID, &orderID, &isValidated, &isCamping, &attendeeFirstName, &attendeeLastName,
		&ticketTypeName, &orderNumber, &orderStatus,
		&fromStation, &toStation, &outboundAt, &returnAt, &isRoundTrip,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &models.ValidateQRResponse{
				Valid:   false,
				Message: "QR code invalide — ticket non trouvé",
			}, nil
		}
		return nil, fmt.Errorf("erreur query ticket: %w", err)
	}

	// Vérifier le statut de la commande
	if orderStatus != models.OrderStatusConfirmed && orderStatus != models.OrderStatusPaid {
		return &models.ValidateQRResponse{
			Valid:   false,
			Message: fmt.Sprintf("Commande non valide (statut: %s)", orderStatus),
		}, nil
	}

	// Vérifier si déjà validé
	if isValidated {
		firstName := ""
		lastName := ""
		if attendeeFirstName != nil {
			firstName = *attendeeFirstName
		}
		if attendeeLastName != nil {
			lastName = *attendeeLastName
		}
		return &models.ValidateQRResponse{
			Valid:             false,
			Message:           "⚠️ Ce ticket a déjà été validé !",
			TicketID:          ticketID,
			AttendeeFirstName: firstName,
			AttendeeLastName:  lastName,
			TicketTypeName:    ticketTypeName,
			OrderNumber:       orderNumber,
			AlreadyValidated:  true,
			IsCamping:         isCamping,
			RideType:          busRideType(isRoundTrip, fromStation),
			FromStation:       fromStation,
			ToStation:         toStation,
			DepartureAt:       formatTimePtr(outboundAt),
			ReturnDepartureAt: formatTimePtr(returnAt),
		}, nil
	}

	// Valider le ticket
	now := time.Now()
	_, err = tx.Exec(ctx,
		`UPDATE tickets SET is_validated = true, validated_at = $1, validated_by = $2 WHERE id = $3`,
		now, validatedBy, ticketID,
	)
	if err != nil {
		return nil, fmt.Errorf("erreur validation ticket: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("erreur commit: %w", err)
	}

	firstName := ""
	lastName := ""
	if attendeeFirstName != nil {
		firstName = *attendeeFirstName
	}
	if attendeeLastName != nil {
		lastName = *attendeeLastName
	}

	return &models.ValidateQRResponse{
		Valid:             true,
		Message:           "✅ Ticket validé avec succès",
		TicketID:          ticketID,
		AttendeeFirstName: firstName,
		AttendeeLastName:  lastName,
		TicketTypeName:    ticketTypeName,
		OrderNumber:       orderNumber,
		IsCamping:         isCamping,
		RideType:          busRideType(isRoundTrip, fromStation),
		FromStation:       fromStation,
		ToStation:         toStation,
		DepartureAt:       formatTimePtr(outboundAt),
		ReturnDepartureAt: formatTimePtr(returnAt),
	}, nil
}

func busRideType(roundTrip bool, fromStation string) string {
	if fromStation == "" {
		return ""
	}
	if roundTrip {
		return "bus_round_trip"
	}
	return "bus_one_way"
}

func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func (r *TicketRepository) GetTicketsByOrderID(ctx context.Context, orderID string) ([]models.Ticket, error) {
	query := `
		SELECT t.id, t.order_id, t.ticket_type_id, t.qr_token, t.is_validated, t.is_camping,
		       t.validated_at, COALESCE(t.validated_by, ''), t.attendee_first_name, t.attendee_last_name, COALESCE(t.attendee_email, ''),
		       t.created_at, tt.name as ticket_type_name
		FROM tickets t
		JOIN ticket_types tt ON tt.id = t.ticket_type_id
		WHERE t.order_id = $1
		ORDER BY t.created_at ASC`

	rows, err := r.pool.Query(ctx, query, orderID)
	if err != nil {
		return nil, fmt.Errorf("erreur query tickets: %w", err)
	}
	defer rows.Close()

	var tickets []models.Ticket
	for rows.Next() {
		var t models.Ticket
		err := rows.Scan(
			&t.ID, &t.OrderID, &t.TicketTypeID, &t.QRToken, &t.IsValidated, &t.IsCamping,
			&t.ValidatedAt, &t.ValidatedBy, &t.AttendeeFirstName, &t.AttendeeLastName, &t.AttendeeEmail,
			&t.CreatedAt, &t.TicketTypeName,
		)
		if err != nil {
			return nil, fmt.Errorf("erreur scan ticket: %w", err)
		}
		tickets = append(tickets, t)
	}

	return tickets, nil
}

func (r *TicketRepository) GetQRCodeDataByToken(ctx context.Context, qrToken string) ([]byte, error) {
	var data []byte
	err := r.pool.QueryRow(ctx,
		`SELECT qr_code_data FROM tickets WHERE qr_token = $1`, qrToken,
	).Scan(&data)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (r *TicketRepository) ClaimCampingByEmail(ctx context.Context, email string) (int, error) {
	commandTag, err := r.pool.Exec(ctx, `
		UPDATE tickets t
		SET is_camping = true
		FROM orders o
		WHERE t.order_id = o.id
		  AND LOWER(o.customer_email) = LOWER($1)
		  AND o.status IN ('paid', 'confirmed')
		  AND NOT EXISTS (
			  SELECT 1
			  FROM bus_tickets bt
			  WHERE bt.ticket_id = t.id
		  )
		  AND t.is_camping = false
	`, email)
	if err != nil {
		return 0, fmt.Errorf("erreur activation camping: %w", err)
	}

	return int(commandTag.RowsAffected()), nil
}

func (r *TicketRepository) GetCampingClaimStats(ctx context.Context, email string) (int, int, int, error) {
	var (
		totalTickets   int
		alreadyCamping int
		updatable      int
	)

	err := r.pool.QueryRow(ctx, `
		SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE t.is_camping = true)::int,
			COUNT(*) FILTER (WHERE t.is_camping = false)::int
		FROM tickets t
		JOIN orders o ON o.id = t.order_id
		LEFT JOIN bus_tickets bt ON bt.ticket_id = t.id
		WHERE LOWER(o.customer_email) = LOWER($1)
		  AND o.status IN ('paid', 'confirmed')
		  AND bt.ticket_id IS NULL
	`, email).Scan(&totalTickets, &alreadyCamping, &updatable)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("erreur lecture statut camping: %w", err)
	}

	return totalTickets, alreadyCamping, updatable, nil
}

func (r *TicketRepository) GetBusStations(ctx context.Context) ([]models.BusStation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, is_active, created_at, updated_at
		FROM bus_stations
		ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("erreur query stations bus: %w", err)
	}
	defer rows.Close()

	stations := make([]models.BusStation, 0)
	for rows.Next() {
		var s models.BusStation
		if err := rows.Scan(&s.ID, &s.Name, &s.IsActive, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("erreur scan station bus: %w", err)
		}
		stations = append(stations, s)
	}
	return stations, nil
}

func (r *TicketRepository) GetBusDepartures(ctx context.Context, direction string) ([]models.BusDeparture, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT d.id, d.station_id, s.name, d.direction, d.departure_time, d.price_cents, d.capacity, d.sold, d.is_active, d.created_at, d.updated_at
		FROM bus_departures d
		JOIN bus_stations s ON s.id = d.station_id
		WHERE d.direction = $1
		ORDER BY d.departure_time ASC`, direction)
	if err != nil {
		return nil, fmt.Errorf("erreur query départs bus: %w", err)
	}
	defer rows.Close()

	departures := make([]models.BusDeparture, 0)
	for rows.Next() {
		var d models.BusDeparture
		if err := rows.Scan(&d.ID, &d.StationID, &d.StationName, &d.Direction, &d.DepartureTime, &d.PriceCents, &d.Capacity, &d.Sold, &d.IsActive, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("erreur scan départ bus: %w", err)
		}
		departures = append(departures, d)
	}
	return departures, nil
}

func (r *TicketRepository) CreateBusStation(ctx context.Context, name string) (*models.BusStation, error) {
	var s models.BusStation
	err := r.pool.QueryRow(ctx, `
		INSERT INTO bus_stations (name)
		VALUES ($1)
		RETURNING id, name, is_active, created_at, updated_at`, name).Scan(
		&s.ID, &s.Name, &s.IsActive, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("erreur création station bus: %w", err)
	}
	return &s, nil
}

func (r *TicketRepository) CreateBusDeparture(ctx context.Context, req models.CreateBusDepartureRequest) (*models.BusDeparture, error) {
	var d models.BusDeparture
	err := r.pool.QueryRow(ctx, `
		INSERT INTO bus_departures (station_id, direction, departure_time, price_cents, capacity, is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, station_id, direction, departure_time, price_cents, capacity, sold, is_active, created_at, updated_at`,
		req.StationID, req.Direction, req.DepartureTime, req.PriceCents, req.Capacity, req.IsActive,
	).Scan(
		&d.ID, &d.StationID, &d.Direction, &d.DepartureTime, &d.PriceCents, &d.Capacity, &d.Sold, &d.IsActive, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("erreur création départ bus: %w", err)
	}

	_ = r.pool.QueryRow(ctx, `SELECT name FROM bus_stations WHERE id = $1`, d.StationID).Scan(&d.StationName)
	return &d, nil
}

func (r *TicketRepository) UpdateBusDeparture(ctx context.Context, id string, req models.UpdateBusDepartureRequest) (*models.BusDeparture, error) {
	var sold int
	err := r.pool.QueryRow(ctx, `SELECT sold FROM bus_departures WHERE id = $1`, id).Scan(&sold)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("départ navette introuvable")
		}
		return nil, fmt.Errorf("erreur lecture départ navette: %w", err)
	}

	if req.Capacity < sold {
		return nil, fmt.Errorf("capacité invalide: %d déjà vendu(s)", sold)
	}

	var d models.BusDeparture
	err = r.pool.QueryRow(ctx, `
		UPDATE bus_departures
		SET station_id = $2,
		    direction = $3,
		    departure_time = $4,
		    price_cents = $5,
		    capacity = $6,
		    is_active = $7,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING id, station_id, direction, departure_time, price_cents, capacity, sold, is_active, created_at, updated_at`,
		id, req.StationID, req.Direction, req.DepartureTime, req.PriceCents, req.Capacity, req.IsActive,
	).Scan(
		&d.ID, &d.StationID, &d.Direction, &d.DepartureTime, &d.PriceCents, &d.Capacity, &d.Sold, &d.IsActive, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("erreur mise à jour départ navette: %w", err)
	}

	_ = r.pool.QueryRow(ctx, `SELECT name FROM bus_stations WHERE id = $1`, d.StationID).Scan(&d.StationName)
	return &d, nil
}

func (r *TicketRepository) ToggleBusDepartureMask(ctx context.Context, id string) (*models.BusDeparture, error) {
	var d models.BusDeparture
	err := r.pool.QueryRow(ctx, `
		UPDATE bus_departures
		SET is_active = NOT is_active,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING id, station_id, direction, departure_time, price_cents, capacity, sold, is_active, created_at, updated_at`, id,
	).Scan(
		&d.ID, &d.StationID, &d.Direction, &d.DepartureTime, &d.PriceCents, &d.Capacity, &d.Sold, &d.IsActive, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("départ navette introuvable")
		}
		return nil, fmt.Errorf("erreur masquage départ navette: %w", err)
	}

	_ = r.pool.QueryRow(ctx, `SELECT name FROM bus_stations WHERE id = $1`, d.StationID).Scan(&d.StationName)
	return &d, nil
}

func (r *TicketRepository) DeleteBusDeparture(ctx context.Context, id string) error {
	var sold int
	err := r.pool.QueryRow(ctx, `SELECT sold FROM bus_departures WHERE id = $1`, id).Scan(&sold)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("départ navette introuvable")
		}
		return fmt.Errorf("erreur lecture départ navette: %w", err)
	}
	if sold > 0 {
		return fmt.Errorf("impossible de supprimer: %d ticket(s) vendu(s)", sold)
	}

	var linked int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM bus_order_rides WHERE departure_id = $1`, id).Scan(&linked); err != nil {
		return fmt.Errorf("erreur vérification rides navette: %w", err)
	}
	if linked > 0 {
		return fmt.Errorf("impossible de supprimer: départ déjà utilisé par des commandes")
	}

	cmd, err := r.pool.Exec(ctx, `DELETE FROM bus_departures WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("erreur suppression départ navette: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("départ navette introuvable")
	}

	return nil
}

func (r *TicketRepository) GetBusDepartureByID(ctx context.Context, id string) (*models.BusDeparture, error) {
	var d models.BusDeparture
	err := r.pool.QueryRow(ctx, `
		SELECT d.id, d.station_id, s.name, d.direction, d.departure_time, d.price_cents, d.capacity, d.sold, d.is_active, d.created_at, d.updated_at
		FROM bus_departures d
		JOIN bus_stations s ON s.id = d.station_id
		WHERE d.id = $1`, id).Scan(
		&d.ID, &d.StationID, &d.StationName, &d.Direction, &d.DepartureTime, &d.PriceCents, &d.Capacity, &d.Sold, &d.IsActive, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("erreur récupération départ bus: %w", err)
	}
	return &d, nil
}

func (r *TicketRepository) ReserveBusDepartureSeat(ctx context.Context, tx pgx.Tx, departureID string) error {
	cmd, err := tx.Exec(ctx, `
		UPDATE bus_departures
		SET sold = sold + 1, updated_at = NOW()
		WHERE id = $1 AND is_active = true AND sold < capacity`, departureID)
	if err != nil {
		return fmt.Errorf("erreur réservation navette: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("navette complète ou indisponible")
	}
	return nil
}

func (r *TicketRepository) SaveBusOrderRide(ctx context.Context, tx pgx.Tx, orderID, departureID, rideKind, fromStation, toStation string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO bus_order_rides (order_id, departure_id, ride_kind, from_station, to_station)
		VALUES ($1, $2, $3, $4, $5)`, orderID, departureID, rideKind, fromStation, toStation)
	if err != nil {
		return fmt.Errorf("erreur sauvegarde ride bus: %w", err)
	}
	return nil
}

func (r *TicketRepository) GetBusOrderRides(ctx context.Context, orderID string) ([]map[string]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT departure_id::text, ride_kind, from_station, to_station
		FROM bus_order_rides
		WHERE order_id = $1
		ORDER BY id ASC`, orderID)
	if err != nil {
		return nil, fmt.Errorf("erreur query rides bus commande: %w", err)
	}
	defer rows.Close()

	rides := make([]map[string]string, 0)
	for rows.Next() {
		var departureID, rideKind, fromStation, toStation string
		if err := rows.Scan(&departureID, &rideKind, &fromStation, &toStation); err != nil {
			return nil, fmt.Errorf("erreur scan ride bus: %w", err)
		}
		rides = append(rides, map[string]string{
			"departure_id": departureID,
			"ride_kind":    rideKind,
			"from_station": fromStation,
			"to_station":   toStation,
		})
	}
	return rides, nil
}

func (r *TicketRepository) UpdateOrderAttendees(ctx context.Context, orderID string, firstName, lastName, email string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tickets
		SET attendee_first_name = $1,
		    attendee_last_name = $2,
		    attendee_email = $3
		WHERE order_id = $4`,
		firstName,
		lastName,
		email,
		orderID,
	)
	if err != nil {
		return fmt.Errorf("erreur update attendees commande: %w", err)
	}
	return nil
}

func (r *TicketRepository) ReleaseBusOrderRides(ctx context.Context, orderID string) error {
	rows, err := r.pool.Query(ctx, `
		SELECT departure_id
		FROM bus_order_rides
		WHERE order_id = $1`, orderID)
	if err != nil {
		return fmt.Errorf("erreur récupération rides à libérer: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("erreur scan departure_id: %w", err)
		}
		ids = append(ids, id)
	}

	for _, id := range ids {
		_, err := r.pool.Exec(ctx, `
			UPDATE bus_departures
			SET sold = GREATEST(0, sold - 1), updated_at = NOW()
			WHERE id = $1`, id)
		if err != nil {
			return fmt.Errorf("erreur libération place bus: %w", err)
		}
	}

	return nil
}

func (r *TicketRepository) SaveBusTicketDetails(ctx context.Context, tx pgx.Tx, ticketID, outboundDepartureID string, returnDepartureID *string, fromStation, toStation string, isRoundTrip bool) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO bus_tickets (ticket_id, outbound_departure_id, return_departure_id, from_station, to_station, is_round_trip)
		VALUES ($1, $2, $3, $4, $5, $6)`, ticketID, outboundDepartureID, returnDepartureID, fromStation, toStation, isRoundTrip)
	if err != nil {
		return fmt.Errorf("erreur sauvegarde détails ticket bus: %w", err)
	}
	return nil
}

func (r *TicketRepository) ListBusTickets(ctx context.Context, limit int) ([]models.BusTicketAdminRow, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := r.pool.Query(ctx, `
		SELECT t.id, o.order_number, o.total_cents, o.customer_first_name, o.customer_last_name, o.customer_email,
		       bt.from_station, bt.to_station, od.departure_time, rd.departure_time,
		       bt.is_round_trip, t.is_validated, t.created_at
		FROM bus_tickets bt
		JOIN tickets t ON t.id = bt.ticket_id
		JOIN orders o ON o.id = t.order_id
		JOIN bus_departures od ON od.id = bt.outbound_departure_id
		LEFT JOIN bus_departures rd ON rd.id = bt.return_departure_id
		ORDER BY t.created_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("erreur query tickets bus admin: %w", err)
	}
	defer rows.Close()

	out := make([]models.BusTicketAdminRow, 0)
	for rows.Next() {
		var row models.BusTicketAdminRow
		var returnDeparture *time.Time
		if err := rows.Scan(
			&row.TicketID, &row.OrderNumber, &row.OrderTotalCents, &row.CustomerFirstName, &row.CustomerLastName, &row.CustomerEmail,
			&row.FromStation, &row.ToStation, &row.DepartureTime, &returnDeparture,
			&row.IsRoundTrip, &row.IsValidated, &row.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("erreur scan ticket bus admin: %w", err)
		}
		row.ReturnDepartureTime = returnDeparture
		out = append(out, row)
	}

	return out, nil
}

func (r *TicketRepository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}
