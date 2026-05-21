package api

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/thaily/policy-engine/backend/internal/auth"
)

// AuthHandler issues admin JWTs.
type AuthHandler struct {
	JWTSecret     string
	AdminAccounts map[string]string // email → bcrypt or plaintext (dev) password
	TokenLifetime time.Duration
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
}

// POST /login
//
// Dev-only: passwords are matched plaintext from AdminAccounts. For prod
// swap to bcrypt + DB-backed admin table.
func (h *AuthHandler) Login(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	want, ok := h.AdminAccounts[req.Email]
	if !ok || want != req.Password {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
	}
	tok, err := auth.MintToken(h.JWTSecret, req.Email, h.TokenLifetime)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, loginResponse{Token: tok})
}
