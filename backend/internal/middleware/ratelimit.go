package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimit middleware basé sur Redis pour limiter les requêtes
func RateLimit(redisClient *redis.Client, maxRequests int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Identifier le client par IP
			ip := r.Header.Get("X-Forwarded-For")
			if ip == "" {
				ip = r.Header.Get("X-Real-IP")
			}
			if ip == "" {
				ip = r.RemoteAddr
			}

			key := fmt.Sprintf("ratelimit:%s", ip)
			ctx := context.Background()

			// Incrémenter et vérifier
			count, err := redisClient.Incr(ctx, key).Result()
			if err != nil {
				// En cas d'erreur Redis, on laisse passer (fail open)
				next.ServeHTTP(w, r)
				return
			}

			if count == 1 {
				redisClient.Expire(ctx, key, window)
			}

			if count > int64(maxRequests) {
				w.Header().Set("Retry-After", fmt.Sprintf("%d", int(window.Seconds())))
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
	return RateLimit(redisClient, 10, 1*time.Minute) // 10 checkout par minute max
}
