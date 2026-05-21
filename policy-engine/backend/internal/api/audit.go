package api

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/thaily/policy-engine/backend/internal/store"
)

type AuditHandler struct {
	Store *store.AuditStore
}

// GET /api/audit?limit=100
func (h *AuditHandler) List(c echo.Context) error {
	limit, _ := strconv.ParseInt(c.QueryParam("limit"), 10, 64)
	entries, err := h.Store.List(c.Request().Context(), limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, entries)
}
