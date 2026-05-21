package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/thaily/policy-engine/backend/internal/enforcer"
	"github.com/thaily/policy-engine/backend/internal/store"
)

// PolicyHandler exposes CRUD over Casbin policies.
type PolicyHandler struct {
	Enforcer *enforcer.Enforcer
	Audit    *store.AuditStore
}

type policyDTO struct {
	Sub       string `json:"sub"`
	Obj       string `json:"obj"`
	Act       string `json:"act"`
	Condition string `json:"condition"`
}

func (dto policyDTO) tuple() store.PolicyTuple {
	return store.PolicyTuple{Sub: dto.Sub, Obj: dto.Obj, Act: dto.Act, Condition: dto.Condition}
}

// GET /api/policies — list everything.
func (h *PolicyHandler) List(c echo.Context) error {
	rows, err := h.Enforcer.AllPolicies()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	out := make([]policyDTO, 0, len(rows))
	for _, r := range rows {
		// rows are [sub, obj, act, condition]
		dto := policyDTO{Sub: r[0], Obj: r[1], Act: r[2]}
		if len(r) > 3 {
			dto.Condition = r[3]
		}
		out = append(out, dto)
	}
	return c.JSON(http.StatusOK, out)
}

// POST /api/policies — add a new rule. Empty `condition` defaults to "true".
func (h *PolicyHandler) Create(c echo.Context) error {
	var dto policyDTO
	if err := c.Bind(&dto); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	if dto.Sub == "" || dto.Obj == "" || dto.Act == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "sub/obj/act required"})
	}
	if dto.Condition == "" {
		dto.Condition = "true"
	}

	added, err := h.Enforcer.AddPolicy(dto.Sub, dto.Obj, dto.Act, dto.Condition)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if !added {
		return c.JSON(http.StatusConflict, map[string]string{"error": "policy already exists"})
	}

	actor, _ := c.Get("admin").(string)
	t := dto.tuple()
	_ = h.Audit.Log(c.Request().Context(), store.AuditEntry{
		Actor: actor, Action: "add", NewPolicy: &t,
	})
	return c.JSON(http.StatusCreated, dto)
}

// PUT /api/policies — replace: body { old: {...}, new: {...} }.
// Casbin doesn't have stable IDs so we delete + add as the update primitive.
func (h *PolicyHandler) Update(c echo.Context) error {
	var body struct {
		Old policyDTO `json:"old"`
		New policyDTO `json:"new"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	if body.New.Condition == "" {
		body.New.Condition = "true"
	}

	removed, err := h.Enforcer.RemovePolicy(body.Old.Sub, body.Old.Obj, body.Old.Act, body.Old.Condition)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if !removed {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "old policy not found"})
	}

	added, err := h.Enforcer.AddPolicy(body.New.Sub, body.New.Obj, body.New.Act, body.New.Condition)
	if err != nil {
		// best-effort rollback
		_, _ = h.Enforcer.AddPolicy(body.Old.Sub, body.Old.Obj, body.Old.Act, body.Old.Condition)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to add new policy: " + err.Error()})
	}
	if !added {
		return c.JSON(http.StatusConflict, map[string]string{"error": "new policy duplicates an existing one"})
	}

	actor, _ := c.Get("admin").(string)
	oldT, newT := body.Old.tuple(), body.New.tuple()
	_ = h.Audit.Log(c.Request().Context(), store.AuditEntry{
		Actor: actor, Action: "update", OldPolicy: &oldT, NewPolicy: &newT,
	})
	return c.JSON(http.StatusOK, body.New)
}

// DELETE /api/policies — body identifies the rule by full tuple.
func (h *PolicyHandler) Delete(c echo.Context) error {
	var dto policyDTO
	if err := c.Bind(&dto); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "bad body"})
	}
	removed, err := h.Enforcer.RemovePolicy(dto.Sub, dto.Obj, dto.Act, dto.Condition)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if !removed {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "policy not found"})
	}
	actor, _ := c.Get("admin").(string)
	t := dto.tuple()
	_ = h.Audit.Log(c.Request().Context(), store.AuditEntry{
		Actor: actor, Action: "remove", OldPolicy: &t,
	})
	return c.JSON(http.StatusOK, map[string]string{"status": "removed"})
}

// POST /api/policies/reload — pull fresh policies from MongoDB.
// Useful if multiple gateway instances share a Mongo and we want a manual sync.
func (h *PolicyHandler) Reload(c echo.Context) error {
	if err := h.Enforcer.LoadPolicies(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "reloaded"})
}
