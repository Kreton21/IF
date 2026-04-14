package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimit middleware basé sur Redis pour limiter les requêtes
func RateLimit(redisClient *redis.Client, maxRequests int, window time.Duration) func(http.Handler) http.Handler {
	return RateLimitScoped(redisClient, "global", maxRequests, window)
}

func RateLimitScoped(redisClient *redis.Client, scope string, maxRequests int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			if ip == "" {
				http.Error(w, `{"error":"unable to identify client"}`, http.StatusBadRequest)
				return
			}

			key := fmt.Sprintf("ratelimit:%s:%s", scope, ip)
			ctx := context.Background()

			// Incrémenter et vérifier
			count, err := redisClient.Incr(ctx, key).Result()
			if err != nil {
				http.Error(w, `{"error":"service temporairement indisponible"}`, http.StatusServiceUnavailable)
				return
			}

			if count == 1 {
				redisClient.Expire(ctx, key, window)
			}

			if count > int64(maxRequests) {
				w.Header().Set("Retry-After", fmt.Sprintf("%d", int(window.Seconds())))
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, `{"error": "Trop de requêtes, veuillez réessayer plus tard"}`, http.StatusTooManyRequests)
				return
			}

			// Ajouter les headers de rate limit
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", maxRequests))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", maxRequests-int(count)))

			next.ServeHTTP(w, r)
		})
	}
}

// StrictRateLimit pour les endpoints sensibles (checkout)
func StrictRateLimit(redisClient *redis.Client) func(http.Handler) http.Handler {
	return RateLimitScoped(redisClient, "strict", 10, 1*time.Minute) // 10 checkout par minute max
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	if parsed := net.ParseIP(strings.TrimSpace(r.RemoteAddr)); parsed != nil {
		return parsed.String()
	}
	return ""
}
