package enforcer

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr"
)

// registerBuiltinFunctions adds frequently-used helpers.
// Custom funcs that need gRPC calls (isEnrolled, sameClass, ...) are wired up
// by the caller via RegisterFunction so they can inject service clients.
func (en *Enforcer) registerBuiltinFunctions() {
	// evalCondition: evaluate a Go-like expression against the ctx map.
	// Empty condition or "true" → always allow.
	en.e.AddFunction("evalCondition", func(args ...any) (any, error) {
		if len(args) < 2 {
			return false, fmt.Errorf("evalCondition expects (ctx, expression)")
		}
		ctx, _ := args[0].(map[string]any)
		expression, _ := args[1].(string)

		expression = strings.TrimSpace(expression)
		if expression == "" || expression == "true" {
			return true, nil
		}
		if expression == "false" {
			return false, nil
		}

		// Wrap under "ctx" namespace so admin policies are explicit about where
		// values come from (`ctx.viewer_id`, `ctx.target_id`). Easier to reason.
		if ctx == nil {
			ctx = map[string]any{}
		}
		env := map[string]any{"ctx": ctx}

		program, err := expr.Compile(expression, expr.AsBool(), expr.Env(env))
		if err != nil {
			return false, fmt.Errorf("compile expr %q: %w", expression, err)
		}
		out, err := expr.Run(program, env)
		if err != nil {
			return false, fmt.Errorf("run expr %q: %w", expression, err)
		}
		b, _ := out.(bool)
		return b, nil
	})
}
