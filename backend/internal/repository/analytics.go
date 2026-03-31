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

func (r *AnalyticsRepository) InsertSessionStart(ctx context.Context, sessionID, page, referrer, userAgent, ip string, isNew bool, utmSource, utmMedium, utmCampaign string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO analytics_sessions (session_id, page, referrer, user_agent, ip, is_new_session, utm_source, utm_medium, utm_campaign)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT DO NOTHING`,
		sessionID, page, referrer, userAgent, ip, isNew, utmSource, utmMedium, utmCampaign,
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

func (r *AnalyticsRepository) InsertPageView(ctx context.Context, sessionID, page string, durationMs int64) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO analytics_page_views (session_id, page, duration_ms)
		 VALUES ($1, $2, NULLIF($3, 0))`,
		sessionID, page, durationMs,
	)
	return err
}

// GetKPI returns all analytics KPIs (admin-only).
// rangeStr can be: "1h", "1j", "1semaine", "1mois", or a custom range like "custom:2026-01-01"
func (r *AnalyticsRepository) GetKPI(ctx context.Context, rangeStr string) (*models.AnalyticsKPI, error) {
	since := rangeToTime(rangeStr)

	kpi := &models.AnalyticsKPI{}

	// ── Trafic: Total sessions & avg duration ─────────────────────────────
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(AVG(duration_ms), 0)
		 FROM analytics_sessions WHERE started_at >= $1`,
		since,
	).Scan(&kpi.TotalSessions, &kpi.AvgSessionDuration)
	if err != nil {
		return nil, fmt.Errorf("sessions kpi: %w", err)
	}
	kpi.AvgSessionDuration = kpi.AvgSessionDuration / 1000.0

	// ── Trafic: New sessions ───────────────────────────────────────────────
	err = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM analytics_sessions WHERE started_at >= $1 AND is_new_session = TRUE`,
		since,
	).Scan(&kpi.NewSessions)
	if err != nil {
		return nil, fmt.Errorf("new sessions kpi: %w", err)
	}

	// ── Trafic: Sessions timeline ──────────────────────────────────────────
	bucket := bucketForRange(rangeStr)
	kpi.SessionsTimeline, err = r.timelineQuery(ctx,
		fmt.Sprintf(`SELECT date_trunc('%s', started_at) AS bucket, COUNT(*)
		 FROM analytics_sessions WHERE started_at >= $1
		 GROUP BY bucket ORDER BY bucket`, bucket),
		since,
	)
	if err != nil {
		return nil, fmt.Errorf("sessions timeline: %w", err)
	}

	// ── Trafic: New sessions timeline ─────────────────────────────────────
	kpi.NewSessionsTimeline, err = r.timelineQuery(ctx,
		fmt.Sprintf(`SELECT date_trunc('%s', started_at) AS bucket, COUNT(*)
		 FROM analytics_sessions WHERE started_at >= $1 AND is_new_session = TRUE
		 GROUP BY bucket ORDER BY bucket`, bucket),
		since,
	)
	if err != nil {
		return nil, fmt.Errorf("new sessions timeline: %w", err)
	}

	// ── Comportement: Total clicks ─────────────────────────────────────────
	err = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM analytics_clicks WHERE clicked_at >= $1`,
		since,
	).Scan(&kpi.TotalClicks)
	if err != nil {
		return nil, fmt.Errorf("clicks kpi: %w", err)
	}

	// ── Comportement: Clicks timeline ─────────────────────────────────────
	kpi.ClicksTimeline, err = r.timelineQuery(ctx,
		fmt.Sprintf(`SELECT date_trunc('%s', clicked_at) AS bucket, COUNT(*)
		 FROM analytics_clicks WHERE clicked_at >= $1
		 GROUP BY bucket ORDER BY bucket`, bucket),
		since,
	)
	if err != nil {
		return nil, fmt.Errorf("clicks timeline: %w", err)
	}

	// ── Comportement: Bounce rate ──────────────────────────────────────────
	// A bounce = session with 0 clicks
	var bouncedSessions int
	err = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM analytics_sessions s
		 WHERE s.started_at >= $1
		 AND NOT EXISTS (SELECT 1 FROM analytics_clicks c WHERE c.session_id = s.session_id AND c.clicked_at >= $1)`,
		since,
	).Scan(&bouncedSessions)
	if err != nil {
		return nil, fmt.Errorf("bounce rate: %w", err)
	}
	if kpi.TotalSessions > 0 {
		kpi.BounceRate = float64(bouncedSessions) / float64(kpi.TotalSessions) * 100.0
	}

	// ── Comportement: Pages per session ───────────────────────────────────
	var totalPageViews int
	err = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM analytics_page_views WHERE entered_at >= $1`,
		since,
	).Scan(&totalPageViews)
	if err != nil {
		return nil, fmt.Errorf("page views: %w", err)
	}
	if kpi.TotalSessions > 0 {
		kpi.PagesPerSession = float64(totalPageViews) / float64(kpi.TotalSessions)
	}

	// ── Comportement: Top pages (with % of total sessions) ────────────────
	rows, err := r.pool.Query(ctx,
		`SELECT page, COUNT(DISTINCT session_id) AS sessions,
		        (SELECT COUNT(*) FROM analytics_clicks c WHERE c.page = s.page AND c.clicked_at >= $1) AS clicks
		 FROM analytics_sessions s
		 WHERE started_at >= $1
		 GROUP BY page ORDER BY sessions DESC LIMIT 10`,
		since,
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
		if kpi.TotalSessions > 0 {
			p.Percentage = float64(p.Sessions) / float64(kpi.TotalSessions) * 100.0
		}
		kpi.TopPages = append(kpi.TopPages, p)
	}
	rows.Close()

	// ── Trafic: Top traffic sources ────────────────────────────────────────
	srcRows, err := r.pool.Query(ctx,
		`SELECT
		    CASE
		        WHEN utm_source IS NOT NULL AND utm_source <> '' THEN utm_source
		        WHEN referrer ILIKE '%instagram%' OR referrer ILIKE '%instagr.am%' THEN 'Instagram'
		        WHEN referrer ILIKE '%tiktok%' THEN 'TikTok'
		        WHEN referrer ILIKE '%facebook%' OR referrer ILIKE '%fb.com%' THEN 'Facebook'
		        WHEN referrer ILIKE '%twitter%' OR referrer ILIKE '%t.co%' THEN 'Twitter/X'
		        WHEN referrer ILIKE '%linktree%' THEN 'Linktree'
		        WHEN referrer ILIKE '%youtube%' THEN 'YouTube'
		        WHEN referrer = '' OR referrer IS NULL THEN 'Direct'
		        ELSE 'SEO / Autre'
		    END AS source,
		    COUNT(*) AS sessions
		 FROM analytics_sessions
		 WHERE started_at >= $1
		 GROUP BY source ORDER BY sessions DESC LIMIT 10`,
		since,
	)
	if err != nil {
		return nil, fmt.Errorf("top sources: %w", err)
	}
	defer srcRows.Close()
	for srcRows.Next() {
		var s models.AnalyticsSourceStat
		if err := srcRows.Scan(&s.Source, &s.Sessions); err != nil {
			return nil, err
		}
		kpi.TopSources = append(kpi.TopSources, s)
	}
	srcRows.Close()

	// ── Conversion: paid orders vs sessions ───────────────────────────────
	err = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM orders WHERE status IN ('paid','confirmed') AND created_at >= $1`,
		since,
	).Scan(&kpi.TotalConversions)
	if err != nil {
		return nil, fmt.Errorf("conversions: %w", err)
	}
	if kpi.TotalSessions > 0 {
		kpi.ConversionRate = float64(kpi.TotalConversions) / float64(kpi.TotalSessions) * 100.0
	}

	// ── Coût d'acquisition: marketing costs in range ──────────────────────
	costRows, err := r.pool.Query(ctx,
		`SELECT id, cost_date, amount_eur, COALESCE(label,'') FROM marketing_costs WHERE cost_date >= $1::date ORDER BY cost_date`,
		since,
	)
	if err != nil {
		return nil, fmt.Errorf("marketing costs: %w", err)
	}
	defer costRows.Close()
	for costRows.Next() {
		var c models.MarketingCostEntry
		if err := costRows.Scan(&c.ID, &c.CostDate, &c.AmountEur, &c.Label); err != nil {
			return nil, err
		}
		kpi.MarketingCosts = append(kpi.MarketingCosts, c)
		kpi.TotalMarketingCost += c.AmountEur
	}
	costRows.Close()
	if kpi.TotalConversions > 0 && kpi.TotalMarketingCost > 0 {
		kpi.CostPerConversion = kpi.TotalMarketingCost / float64(kpi.TotalConversions)
	}

	return kpi, nil
}

// CreateMarketingCost inserts a new marketing cost entry.
func (r *AnalyticsRepository) CreateMarketingCost(ctx context.Context, req models.CreateMarketingCostRequest) (*models.MarketingCostEntry, error) {
	var entry models.MarketingCostEntry
	err := r.pool.QueryRow(ctx,
		`INSERT INTO marketing_costs (cost_date, amount_eur, label) VALUES ($1::date, $2, $3)
		 RETURNING id, cost_date::text, amount_eur, COALESCE(label,'')`,
		req.CostDate, req.AmountEur, req.Label,
	).Scan(&entry.ID, &entry.CostDate, &entry.AmountEur, &entry.Label)
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// DeleteMarketingCost removes a marketing cost entry by ID.
func (r *AnalyticsRepository) DeleteMarketingCost(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM marketing_costs WHERE id = $1`, id)
	return err
}

func (r *AnalyticsRepository) timelineQuery(ctx context.Context, query string, since time.Time) ([]models.AnalyticsTimePoint, error) {
	rows, err := r.pool.Query(ctx, query, since)
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
		// Support "custom:YYYY-MM-DD" for date-range from a specific date
		if len(rangeName) > 7 && rangeName[:7] == "custom:" {
			t, err := time.Parse("2006-01-02", rangeName[7:])
			if err == nil {
				return t
			}
		}
		return now.Add(-7 * 24 * time.Hour)
	}
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

