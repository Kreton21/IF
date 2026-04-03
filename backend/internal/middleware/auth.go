package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/kreton/if-festival/internal/services"
)

type contextKey string

const (
	AdminIDKey   contextKey = "admin_id"
	AdminNameKey contextKey = "admin_name"
	AdminRoleKey contextKey = "admin_role"
	AdminRawRoleKey contextKey = "admin_raw_role"
)

// JWTAuth middleware pour protéger les routes admin
func JWTAuth(adminService *services.AdminService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error": "Token manquant"}`, http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				http.Error(w, `{"error": "Format token invalide"}`, http.StatusUnauthorized)
				return
			}

			claims, err := adminService.ValidateJWT(parts[1])
			if err != nil {
				http.Error(w, `{"error": "Token invalide ou expiré"}`, http.StatusUnauthorized)
				return
			}

			// Ajouter les infos admin au context
			ctx := r.Context()
			if sub, ok := claims["sub"].(string); ok {
				ctx = context.WithValue(ctx, AdminIDKey, sub)
			}
			if name, ok := claims["name"].(string); ok {
				ctx = context.WithValue(ctx, AdminNameKey, name)
			}
			if role, ok := claims["role"].(string); ok {
				ctx = context.WithValue(ctx, AdminRawRoleKey, role)
				effectiveRole := role
				if role == "super-admin" {
					effectiveRole = "admin"
				}
				ctx = context.WithValue(ctx, AdminRoleKey, effectiveRole)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetAdminName extrait le nom admin du context
func GetAdminName(ctx context.Context) string {
	if name, ok := ctx.Value(AdminNameKey).(string); ok {
		return name
	}
	return "unknown"
}

// GetAdminRole extrait le rôle admin du context
func GetAdminRole(ctx context.Context) string {
	if role, ok := ctx.Value(AdminRoleKey).(string); ok {
		return role
	}
	return ""
}

// GetAdminRawRole extrait le rôle réel admin du context (ex: super-admin)
func GetAdminRawRole(ctx context.Context) string {
	if role, ok := ctx.Value(AdminRawRoleKey).(string); ok {
		return role
	}
	return ""
}

// GetAdminID extrait l'ID admin du context
func GetAdminID(ctx context.Context) string {
	if id, ok := ctx.Value(AdminIDKey).(string); ok {
		return id
	}
	return ""
}

func RequireNotRawRole(disallowedRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if GetAdminRawRole(r.Context()) == disallowedRole {
				http.Error(w, `{"error": "Accès interdit"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
