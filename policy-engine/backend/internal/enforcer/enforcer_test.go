package enforcer

import (
	"context"
	"os"
	"testing"

	"github.com/thaily/policy-engine/backend/internal/store"
)

func TestEnforcer_Basic(t *testing.T) {
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		t.Skip("MONGO_URI not set")
	}
	ctx := context.Background()
	client, err := store.ConnectMongo(ctx, uri)
	if err != nil {
		t.Fatalf("connect mongo: %v", err)
	}
	defer client.Disconnect(ctx)

	en, err := NewWithClient(client, "lms_policy_test", "casbin_rule")
	if err != nil {
		t.Fatalf("new enforcer: %v", err)
	}

	policies, _ := en.AllPolicies()
	for _, p := range policies {
		en.RemovePolicy(p[0], p[1], p[2], p[3])
	}

	if _, err := en.AddPolicy("ADMIN", "*", "*", "true"); err != nil {
		t.Fatalf("add policy 1: %v", err)
	}
	if _, err := en.AddPolicy("STUDENT", "User.email", "read", "ctx.viewer_id == ctx.target_id"); err != nil {
		t.Fatalf("add policy 2: %v", err)
	}

	ok, _ := en.Check(ctx, "ADMIN", "User.email", "read", map[string]any{"viewer_id": "1", "target_id": "999"})
	if !ok {
		t.Errorf("admin should be allowed")
	}

	ok, _ = en.Check(ctx, "STUDENT", "User.email", "read", map[string]any{"viewer_id": "5", "target_id": "5"})
	if !ok {
		t.Errorf("student reading own should be allowed")
	}

	ok, _ = en.Check(ctx, "STUDENT", "User.email", "read", map[string]any{"viewer_id": "5", "target_id": "6"})
	if ok {
		t.Errorf("student reading other should be DENIED")
	}
}
