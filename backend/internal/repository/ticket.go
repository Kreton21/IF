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
		       sale_start, sale_end, is_active, is_masked, max_per_order, COALESCE(allowed_domains, '{}'), created_at, updated_at
		FROM ticket_types
		WHERE is_active = true AND is_masked = false
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
			&tt.IsActive, &tt.IsMasked, &tt.MaxPerOrder, &tt.AllowedDomains, &tt.CreatedAt, &tt.UpdatedAt,
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
		       sale_start, sale_end, is_active, is_masked, max_per_order, COALESCE(allowed_domains, '{}'), created_at, updated_at
		FROM ticket_types
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
			&tt.IsActive, &tt.IsMasked, &tt.MaxPerOrder, &tt.AllowedDomains, &tt.CreatedAt, &tt.UpdatedAt,
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
		       sale_start, sale_end, is_active, is_masked, max_per_order, COALESCE(allowed_domains, '{}'), created_at, updated_at
		FROM ticket_types
		WHERE id = $1`

	var tt models.TicketType
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&tt.ID, &tt.Name, &tt.Description, &tt.PriceCents,
		&tt.QuantityTotal, &tt.QuantitySold, &tt.SaleStart, &tt.SaleEnd,
		&tt.IsActive, &tt.IsMasked, &tt.MaxPerOrder, &tt.AllowedDomains, &tt.CreatedAt, &tt.UpdatedAt,
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
		INSERT INTO ticket_types (name, description, price_cents, quantity_total, sale_start, sale_end, max_per_order, allowed_domains)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, name, description, price_cents, quantity_total, quantity_sold,
		          sale_start, sale_end, is_active, is_masked, max_per_order, COALESCE(allowed_domains, '{}'), created_at, updated_at`

	var tt models.TicketType
	err := r.pool.QueryRow(ctx, query,
		req.Name, req.Description, req.PriceCents, req.QuantityTotal,
		req.SaleStart, req.SaleEnd, req.MaxPerOrder, req.AllowedDomains,
	).Scan(
		&tt.ID, &tt.Name, &tt.Description, &tt.PriceCents,
		&tt.QuantityTotal, &tt.QuantitySold, &tt.SaleStart, &tt.SaleEnd,
		&tt.IsActive, &tt.IsMasked, &tt.MaxPerOrder, &tt.AllowedDomains, &tt.CreatedAt, &tt.UpdatedAt,
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
		    sale_start = $6, sale_end = $7, allowed_domains = $8
		WHERE id = $1
		RETURNING id, name, description, price_cents, quantity_total, quantity_sold,
		          sale_start, sale_end, is_active, is_masked, max_per_order, COALESCE(allowed_domains, '{}'), created_at, updated_at`

	var tt models.TicketType
	err = r.pool.QueryRow(ctx, query, id,
		req.Name, req.Description, req.PriceCents, req.QuantityTotal,
		req.SaleStart, req.SaleEnd, req.AllowedDomains,
	).Scan(
		&tt.ID, &tt.Name, &tt.Description, &tt.PriceCents,
		&tt.QuantityTotal, &tt.QuantitySold, &tt.SaleStart, &tt.SaleEnd,
		&tt.IsActive, &tt.IsMasked, &tt.MaxPerOrder, &tt.AllowedDomains, &tt.CreatedAt, &tt.UpdatedAt,
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
		          sale_start, sale_end, is_active, is_masked, max_per_order, COALESCE(allowed_domains, '{}'), created_at, updated_at`

	var tt models.TicketType
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&tt.ID, &tt.Name, &tt.Description, &tt.PriceCents,
		&tt.QuantityTotal, &tt.QuantitySold, &tt.SaleStart, &tt.SaleEnd,
		&tt.IsActive, &tt.IsMasked, &tt.MaxPerOrder, &tt.AllowedDomains, &tt.CreatedAt, &tt.UpdatedAt,
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
		          is_masked, COALESCE(allowed_domains, '{}'), created_at, updated_at`

	var c models.TicketCategory
	err := r.pool.QueryRow(ctx, query, categoryID).Scan(
		&c.ID, &c.TicketTypeID, &c.Name, &c.QuantityAllocated,
		&c.QuantitySold, &c.IsMasked, &c.AllowedDomains, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("erreur toggle mask catégorie: %w", err)
	}
	return &c, nil
}

// ============================================
// Catégories
// ============================================

func (r *TicketRepository) GetCategoriesByTicketType(ctx context.Context, ticketTypeID string) ([]models.TicketCategory, error) {
	query := `
		SELECT id, ticket_type_id, name, quantity_allocated, quantity_sold,
		       is_masked, COALESCE(allowed_domains, '{}'), created_at, updated_at
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
			&c.QuantitySold, &c.IsMasked, &c.AllowedDomains, &c.CreatedAt, &c.UpdatedAt)
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
		          is_masked, COALESCE(allowed_domains, '{}'), created_at, updated_at`

	var c models.TicketCategory
	err = r.pool.QueryRow(ctx, query, req.TicketTypeID, req.Name, req.Quantity, req.AllowedDomains).Scan(
		&c.ID, &c.TicketTypeID, &c.Name, &c.QuantityAllocated,
		&c.QuantitySold, &c.IsMasked, &c.AllowedDomains, &c.CreatedAt, &c.UpdatedAt)
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
		INSERT INTO tickets (order_id, ticket_type_id, qr_token, qr_code_data, attendee_first_name, attendee_last_name)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`

	return tx.QueryRow(ctx, query,
		ticket.OrderID, ticket.TicketTypeID, ticket.QRToken,
		ticket.QRCodeData, ticket.AttendeeFirstName, ticket.AttendeeLastName,
	).Scan(&ticket.ID, &ticket.CreatedAt)
}

func (r *TicketRepository) GetTicketByQRToken(ctx context.Context, qrToken string) (*models.Ticket, error) {
	query := `
		SELECT t.id, t.order_id, t.ticket_type_id, t.qr_token, t.is_validated,
		       t.validated_at, COALESCE(t.validated_by, ''), t.attendee_first_name, t.attendee_last_name,
		       t.created_at, tt.name as ticket_type_name
		FROM tickets t
		JOIN ticket_types tt ON tt.id = t.ticket_type_id
		WHERE t.qr_token = $1`

	var t models.Ticket
	err := r.pool.QueryRow(ctx, query, qrToken).Scan(
		&t.ID, &t.OrderID, &t.TicketTypeID, &t.QRToken, &t.IsValidated,
		&t.ValidatedAt, &t.ValidatedBy, &t.AttendeeFirstName, &t.AttendeeLastName,
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
		SELECT t.id, t.order_id, t.is_validated, t.attendee_first_name, t.attendee_last_name,
		       tt.name as ticket_type_name, o.order_number, o.status
		FROM tickets t
		JOIN ticket_types tt ON tt.id = t.ticket_type_id
		JOIN orders o ON o.id = t.order_id
		WHERE t.qr_token = $1
		FOR UPDATE OF t`

	var (
		ticketID          string
		orderID           string
		isValidated       bool
		attendeeFirstName *string
		attendeeLastName  *string
		ticketTypeName    string
		orderNumber       string
		orderStatus       models.OrderStatus
	)

	err = tx.QueryRow(ctx, query, qrToken).Scan(
		&ticketID, &orderID, &isValidated, &attendeeFirstName, &attendeeLastName,
		&ticketTypeName, &orderNumber, &orderStatus,
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
	}, nil
}

func (r *TicketRepository) GetTicketsByOrderID(ctx context.Context, orderID string) ([]models.Ticket, error) {
	query := `
		SELECT t.id, t.order_id, t.ticket_type_id, t.qr_token, t.is_validated,
		       t.validated_at, COALESCE(t.validated_by, ''), t.attendee_first_name, t.attendee_last_name,
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
			&t.ID, &t.OrderID, &t.TicketTypeID, &t.QRToken, &t.IsValidated,
			&t.ValidatedAt, &t.ValidatedBy, &t.AttendeeFirstName, &t.AttendeeLastName,
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

func (r *TicketRepository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}
