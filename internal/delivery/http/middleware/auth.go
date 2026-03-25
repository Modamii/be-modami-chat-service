package middleware

import (
	"context"
	"net/http"
	"strings"

	"be-modami-chat-service/internal/delivery/http/response"
	"be-modami-chat-service/pkg/jwt"
)

type contextKey string

const UserIDKey contextKey = "user_id"

// JWTAuth creates a middleware that validates JWT tokens.
func JWTAuth(jwtService *jwt.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				response.Error(w, http.StatusUnauthorized, "missing authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				response.Error(w, http.StatusUnauthorized, "invalid authorization format")
				return
			}

			claims, err := jwtService.ValidateToken(parts[1])
			if err != nil {
				response.Error(w, http.StatusUnauthorized, "invalid token")
				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserID extracts the user ID from the request context.
func GetUserID(ctx context.Context) string {
	userID, _ := ctx.Value(UserIDKey).(string)
	return userID
}
