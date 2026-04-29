package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kreton/if-festival/internal/models"
)

type AnalyticsRepository struct {
	pool *pgxpool.Pool
}

func NewAnalyticsRepository(pool *pgxpool.Pool) *AnalyticsRepository {
	return &AnalyticsRepository{pool: pool}
}

func (r *AnalyticsRepository) InsertSessionStart(ctx context.Context, sessionID, page, referrer, userAgent, ip string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO analytics_sessions (session_id, page, referrer, user_agent, ip)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT DO NOTHING`,
		sessionID, page, referrer, userAgent, ip,
	)
	return err
}

func (r *AnalyticsRepository) UpdateSessionEnd(ctx context.Context, sessionID string, durationMs int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE analytics_sessions
		 SET ended_at = now(), duration_ms = $2
		 WHERE session_id = $1 AND ended_at IS NULL`,
		sessionID, durationMs,
	)
	return err
}

func (r *AnalyticsRepository) InsertClick(ctx context.Context, sessionID, target, page string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO analytics_clicks (session_id, target, page)
		 VALUES ($1, $2, $3)`,
		sessionID, target, page,
	)
	return err
}

func (r *AnalyticsRepository) GetKPI(ctx context.Context, rangeName string, startAt, endAt *time.Time) (*models.AnalyticsKPI, error) {
	since, until := rangeToBounds(rangeName, startAt, endAt)

	kpi := &models.AnalyticsKPI{}

	// Total sessions & avg duration
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(AVG(duration_ms), 0)
		 FROM analytics_sessions WHERE started_at >= $1 AND started_at <= $2`,
		since, until,
	).Scan(&kpi.TotalSessions, &kpi.AvgSessionDuration)
	if err != nil {
		return nil, fmt.Errorf("sessions kpi: %w", err)
	}
	// Convert from ms to seconds
	kpi.AvgSessionDuration = kpi.AvgSessionDuration / 1000.0

	// Total clicks
	err = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM analytics_clicks WHERE clicked_at >= $1 AND clicked_at <= $2`,
		since, until,
	).Scan(&kpi.TotalClicks)
	if err != nil {
		return nil, fmt.Errorf("clicks kpi: %w", err)
	}

	// Clicks timeline
	bucket := bucketForBounds(rangeName, since, until)
	kpi.ClicksTimeline, err = r.timelineQuery(ctx,
		fmt.Sprintf(`SELECT date_trunc('%s', clicked_at) AS bucket, COUNT(*)
		 FROM analytics_clicks WHERE clicked_at >= $1 AND clicked_at <= $2
		 GROUP BY bucket ORDER BY bucket`, bucket),
		since, until,
	)
	if err != nil {
		return nil, fmt.Errorf("clicks timeline: %w", err)
	}

	// Sessions timeline
	kpi.SessionsTimeline, err = r.timelineQuery(ctx,
		fmt.Sprintf(`SELECT date_trunc('%s', started_at) AS bucket, COUNT(*)
		 FROM analytics_sessions WHERE started_at >= $1 AND started_at <= $2
		 GROUP BY bucket ORDER BY bucket`, bucket),
		since, until,
	)
	if err != nil {
		return nil, fmt.Errorf("sessions timeline: %w", err)
	}

	// Top pages
	rows, err := r.pool.Query(ctx,
		`SELECT page, COUNT(DISTINCT session_id) AS sessions,
		        (SELECT COUNT(*) FROM analytics_clicks c WHERE c.page = s.page AND c.clicked_at >= $1 AND c.clicked_at <= $2) AS clicks
		 FROM analytics_sessions s
		 WHERE started_at >= $1 AND started_at <= $2
		 GROUP BY page ORDER BY sessions DESC LIMIT 10`,
		since, until,
	)
	if err != nil {
		return nil, fmt.Errorf("top pages: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p models.AnalyticsPageStat
		if err := rows.Scan(&p.Page, &p.Sessions, &p.Clicks); err != nil {
			return nil, err
		}
		kpi.TopPages = append(kpi.TopPages, p)
	}
	rows.Close()

	// Ticket origins by email domain and category
	rows, err = r.pool.Query(ctx,
		`SELECT
			COALESCE(NULLIF(LOWER(SPLIT_PART(o.customer_email, '@', 2)), ''), 'inconnu') AS domain,
			COALESCE(tc.name, tt.name) AS category,
			tt.name AS ticket_type,
			SUM(oi.quantity) AS ticket_count
		FROM order_items oi
		JOIN orders o ON o.id = oi.order_id
		JOIN ticket_types tt ON tt.id = oi.ticket_type_id
		LEFT JOIN ticket_categories tc ON tc.id = oi.category_id
		WHERE o.status IN ('paid', 'confirmed')
		  AND o.created_at >= $1 AND o.created_at <= $2
		GROUP BY domain, category, ticket_type
		ORDER BY ticket_count DESC, domain ASC, category ASC
		LIMIT 300`,
		since, until,
	)
	if err != nil {
		return nil, fmt.Errorf("ticket origins kpi: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var row models.AnalyticsTicketOrigin
		if err := rows.Scan(&row.Domain, &row.Category, &row.TicketType, &row.TicketCount); err != nil {
			return nil, err
		}
		kpi.TicketOrigins = append(kpi.TicketOrigins, row)
	}

	return kpi, nil
}

func (r *AnalyticsRepository) timelineQuery(ctx context.Context, query string, since, until time.Time) ([]models.AnalyticsTimePoint, error) {
	rows, err := r.pool.Query(ctx, query, since, until)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []models.AnalyticsTimePoint
	for rows.Next() {
		var tp models.AnalyticsTimePoint
		var t time.Time
		if err := rows.Scan(&t, &tp.Count); err != nil {
			return nil, err
		}
		tp.Bucket = t.Format(time.RFC3339)
		result = append(result, tp)
	}
	return result, nil
}

func rangeToTime(rangeName string) time.Time {
	now := time.Now()
	switch rangeName {
	case "1h":
		return now.Add(-1 * time.Hour)
	case "1j":
		return now.Add(-24 * time.Hour)
	case "1semaine":
		return now.Add(-7 * 24 * time.Hour)
	case "1mois":
		return now.Add(-30 * 24 * time.Hour)
	default:
		return now.Add(-7 * 24 * time.Hour)
	}
}

func rangeToBounds(rangeName string, startAt, endAt *time.Time) (time.Time, time.Time) {
	if startAt != nil && endAt != nil {
		return *startAt, *endAt
	}
	return rangeToTime(rangeName), time.Now()
}

func bucketForRange(rangeName string) string {
	switch rangeName {
	case "1h":
		return "minute"
	case "1j":
		return "hour"
	case "1semaine":
		return "day"
	case "1mois":
		return "day"
	default:
		return "day"
	}
}

func bucketForBounds(rangeName string, since, until time.Time) string {
	if rangeName != "custom" {
		return bucketForRange(rangeName)
	}
	if until.Sub(since) <= 24*time.Hour {
		return "hour"
	}
	return "day"
}
