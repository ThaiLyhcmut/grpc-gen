package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/thaily/policy-engine/backend/internal/enforcer"
)

// TestHandler runs the enforcer against ad-hoc input. Useful in the dashboard
// "playground" — admin tries `{sub, obj, act, ctx}` and sees what policy fires.
type TestHandler struct {
	Enforcer *enforcer.Enforcer
}

type testRequest struct {
	Sub string         `json:"sub"`
	Obj string         `json:"obj"`
	Act string         `json:"act"`
	Ctx map[string]any `json:"ctx"`
}

type testResponse struct {
	Allowed bool   `json:"allowed"`
	Error   string `json:"error,omitempty"`
}

// POST /api/test
func (h *TestHandler) Run(c echo.Context) error {
	var req testRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	if req.Ctx == nil {
		req.Ctx = map[string]any{}
	}
	ok, err := h.Enforcer.Check(c.Request().Context(), req.Sub, req.Obj, req.Act, req.Ctx)
	resp := testResponse{Allowed: ok}
	if err != nil {
		resp.Error = err.Error()
	}
	return c.JSON(http.StatusOK, resp)
}
