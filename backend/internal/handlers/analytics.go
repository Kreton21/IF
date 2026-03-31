package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/kreton/if-festival/internal/models"
	"github.com/kreton/if-festival/internal/repository"
)

type AnalyticsHandler struct {
	repo *repository.AnalyticsRepository
}

func NewAnalyticsHandler(repo *repository.AnalyticsRepository) *AnalyticsHandler {
	return &AnalyticsHandler{repo: repo}
}

// Ingest receives analytics events from the public site (no auth required)
func (h *AnalyticsHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	var events []models.AnalyticsEvent
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&events); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}

	if len(events) > 50 {
		events = events[:50]
	}

	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		ip = strings.Split(r.RemoteAddr, ":")[0]
	}
	ua := r.Header.Get("User-Agent")

	for _, ev := range events {
		sid := strings.TrimSpace(ev.SessionID)
		if sid == "" || len(sid) > 128 {
			continue
		}
		page := strings.TrimSpace(ev.Page)
		if page == "" {
			page = "/"
		}

		switch ev.Type {
		case "session_start":
			_ = h.repo.InsertSessionStart(r.Context(), sid, page, ev.Referrer, ua, ip)
		case "click":
			target := strings.TrimSpace(ev.Target)
			if len(target) > 512 {
				target = target[:512]
			}
			_ = h.repo.InsertClick(r.Context(), sid, target, page)
		case "session_end":
			if ev.Duration > 0 {
				_ = h.repo.UpdateSessionEnd(r.Context(), sid, ev.Duration)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GetKPI returns analytics KPIs (admin-only)
func (h *AnalyticsHandler) GetKPI(w http.ResponseWriter, r *http.Request) {
	rangeName := r.URL.Query().Get("range")
	if rangeName == "" {
		rangeName = "1semaine"
	}

	kpi, err := h.repo.GetKPI(r.Context(), rangeName)
	if err != nil {
		log.Printf("Erreur analytics KPI: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Erreur serveur"})
		return
	}

	writeJSON(w, http.StatusOK, kpi)
}
