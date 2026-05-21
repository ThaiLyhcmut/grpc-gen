package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

// AdminClaims holds the admin identity in the JWT.
type AdminClaims struct {
	Email string `json:"email"`
	jwt.RegisteredClaims
}

// Middleware verifies Authorization: Bearer <jwt> against the configured secret
// and stashes the parsed claims into echo.Context under the key "admin".
func Middleware(secret string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := c.Request().Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing bearer token"})
			}
			tokenStr := strings.TrimPrefix(header, "Bearer ")

			claims := &AdminClaims{}
			_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrTokenInvalidClaims
				}
				return []byte(secret), nil
			})
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token: " + err.Error()})
			}

			c.Set("admin", claims.Email)
			return next(c)
		}
	}
}

// MintToken issues a JWT for an admin email. Used by /login for dev.
func MintToken(secret, email string, lifetime time.Duration) (string, error) {
	claims := &AdminClaims{Email: email}
	if lifetime > 0 {
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(lifetime))
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
