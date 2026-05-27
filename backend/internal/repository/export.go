package repository

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strings"
	"time"
)

func (r *AdminRepository) ExportDatabaseCSV(ctx context.Context) ([]byte, error) {
	tableRows, err := r.pool.Query(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_type = 'BASE TABLE'
		ORDER BY table_name ASC`)
	if err != nil {
		return nil, fmt.Errorf("erreur récupération tables: %w", err)
	}
	defer tableRows.Close()

	tables := make([]string, 0)
	for tableRows.Next() {
		var tableName string
		if err := tableRows.Scan(&tableName); err != nil {
			return nil, fmt.Errorf("erreur lecture table: %w", err)
		}
		tables = append(tables, tableName)
	}

	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	if err := w.Write([]string{"table_name", "row_json"}); err != nil {
		return nil, fmt.Errorf("erreur écriture CSV header: %w", err)
	}

	for _, tableName := range tables {
		query := fmt.Sprintf(`SELECT row_to_json(t)::text FROM %s t`, quoteIdentifier(tableName))
		rows, err := r.pool.Query(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("erreur export table %s: %w", tableName, err)
		}

		for rows.Next() {
			var rowJSON string
			if err := rows.Scan(&rowJSON); err != nil {
				rows.Close()
				return nil, fmt.Errorf("erreur lecture ligne table %s: %w", tableName, err)
			}
			if err := w.Write([]string{tableName, rowJSON}); err != nil {
				rows.Close()
				return nil, fmt.Errorf("erreur écriture CSV table %s: %w", tableName, err)
			}
		}
		rows.Close()
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("erreur finalisation CSV: %w", err)
	}

	return buf.Bytes(), nil
}

func (r *AdminRepository) ExportFestivalTicketsCSV(ctx context.Context) ([]byte, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT COALESCE(NULLIF(TRIM(t.attendee_first_name || ' ' || t.attendee_last_name), ''),
		       TRIM(o.customer_first_name || ' ' || o.customer_last_name)) as attendee_name,
		       t.qr_token,
		       tt.name
		FROM tickets t
		JOIN orders o ON o.id = t.order_id
		JOIN ticket_types tt ON tt.id = t.ticket_type_id
		LEFT JOIN bus_tickets bt ON bt.ticket_id = t.id
		WHERE bt.ticket_id IS NULL
		ORDER BY t.created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("erreur export tickets festival: %w", err)
	}
	defer rows.Close()

	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	if err := w.Write([]string{"nom", "num_ticket", "type_ticket"}); err != nil {
		return nil, fmt.Errorf("erreur écriture CSV header: %w", err)
	}

	for rows.Next() {
		var name, qrToken, ticketType string
		if err := rows.Scan(&name, &qrToken, &ticketType); err != nil {
			return nil, fmt.Errorf("erreur lecture ligne ticket festival: %w", err)
		}
		if err := w.Write([]string{name, qrToken, ticketType}); err != nil {
			return nil, fmt.Errorf("erreur écriture CSV ticket festival: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erreur lecture tickets festival: %w", err)
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("erreur finalisation CSV: %w", err)
	}

	return buf.Bytes(), nil
}

func (r *AdminRepository) ExportBusTicketsCSV(ctx context.Context) ([]byte, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT TRIM(o.customer_first_name || ' ' || o.customer_last_name) as customer_name,
		       bt.from_station,
		       od.departure_time,
		       bt.to_station,
		       rd.departure_time
		FROM bus_tickets bt
		JOIN tickets t ON t.id = bt.ticket_id
		JOIN orders o ON o.id = t.order_id
		JOIN bus_departures od ON od.id = bt.outbound_departure_id
		LEFT JOIN bus_departures rd ON rd.id = bt.return_departure_id
		ORDER BY t.created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("erreur export tickets bus: %w", err)
	}
	defer rows.Close()

	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	if err := w.Write([]string{"nom", "gare_depart", "heure_depart", "gare_arrivee", "heure_arrivee"}); err != nil {
		return nil, fmt.Errorf("erreur écriture CSV header: %w", err)
	}

	for rows.Next() {
		var name, fromStation, toStation string
		var departureTime time.Time
		var returnTime *time.Time
		if err := rows.Scan(&name, &fromStation, &departureTime, &toStation, &returnTime); err != nil {
			return nil, fmt.Errorf("erreur lecture ligne ticket bus: %w", err)
		}
		departureStr := departureTime.Format(time.RFC3339)
		returnStr := ""
		if returnTime != nil {
			returnStr = returnTime.Format(time.RFC3339)
		}
		if err := w.Write([]string{name, fromStation, departureStr, toStation, returnStr}); err != nil {
			return nil, fmt.Errorf("erreur écriture CSV ticket bus: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erreur lecture tickets bus: %w", err)
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("erreur finalisation CSV: %w", err)
	}

	return buf.Bytes(), nil
}

func (r *AdminRepository) ExportOrdersCSV(ctx context.Context) ([]byte, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT o.order_number,
		       o.customer_first_name,
		       o.customer_last_name,
		       o.customer_email,
		       o.total_cents,
		       o.status,
		       o.created_at
		FROM orders o
		ORDER BY o.created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("erreur export commandes: %w", err)
	}
	defer rows.Close()

	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	if err := w.Write([]string{"numero_commande", "prenom", "nom", "email", "total_cents", "statut", "cree_le"}); err != nil {
		return nil, fmt.Errorf("erreur écriture CSV header: %w", err)
	}

	for rows.Next() {
		var orderNumber, firstName, lastName, email, status string
		var totalCents int
		var createdAt time.Time
		if err := rows.Scan(&orderNumber, &firstName, &lastName, &email, &totalCents, &status, &createdAt); err != nil {
			return nil, fmt.Errorf("erreur lecture ligne commande: %w", err)
		}
		if err := w.Write([]string{
			orderNumber,
			firstName,
			lastName,
			email,
			fmt.Sprintf("%d", totalCents),
			status,
			createdAt.Format(time.RFC3339),
		}); err != nil {
			return nil, fmt.Errorf("erreur écriture CSV commande: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erreur lecture commandes: %w", err)
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("erreur finalisation CSV: %w", err)
	}

	return buf.Bytes(), nil
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
