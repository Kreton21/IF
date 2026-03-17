package repository

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strings"
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

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
