package api
import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"github.com/golang-jwt/jwt/v5"
	"sancireai/backend/internal/store"
)
type contextKey string
const UserContextKey contextKey = "user_claims"
type Claims struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Name     string `json:"name"`
	jwt.RegisteredClaims
}
func GenerateToken(user *store.User, secret string, ttl time.Duration) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		Name:     user.Name,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    "sancireai",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
func ParseToken(tokenStr string, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, fmt.Errorf("invalid token")
}
// AuthMiddleware optionally extracts user claims from Bearer token
func AuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
				tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
				claims, err := ParseToken(tokenStr, secret)
				if err == nil {
					ctx := context.WithValue(r.Context(), UserContextKey, claims)
					r = r.WithContext(ctx)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
// RequireAdmin enforces DOU_Admin role or returns 403 Forbidden
func RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(UserContextKey).(*Claims)
		if !ok || claims == nil {
			http.Error(w, `{"error":"unauthorized","message":"Требуется авторизация"}`, http.StatusUnauthorized)
			return
		}
		if claims.Role != "DOU_Admin" {
			http.Error(w, `{"error":"forbidden","message":"Доступ разрешен только Администраторам ДОУ (п. 1.4 Регламента)"}`, http.StatusForbidden)
			return
		}
		next(w, r)
	}
}
