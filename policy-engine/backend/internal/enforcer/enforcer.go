package enforcer

import (
	"context"
	_ "embed"
	"fmt"
	"sync"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	mongodbadapter "github.com/casbin/mongodb-adapter/v4"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

//go:embed model.conf
var modelConf string

// Enforcer wraps Casbin enforcer with our custom functions + reload hooks.
type Enforcer struct {
	e         *casbin.Enforcer
	mu        sync.RWMutex
	functions map[string]CustomFunc
}

// CustomFunc is a policy-side function admin can use in `condition` field.
// Example: `isEnrolled(viewer_id, class_id)`. ctx is the per-request context map.
type CustomFunc func(ctx map[string]any, args ...any) (any, error)

// NewWithClient builds an Enforcer using an already-connected Mongo client so
// the audit store can share the same connection / driver.
func NewWithClient(client *mongo.Client, database, collection string) (*Enforcer, error) {
	a, err := mongodbadapter.NewAdapterByDB(client, &mongodbadapter.AdapterConfig{
		DatabaseName:   database,
		CollectionName: collection,
	})
	if err != nil {
		return nil, fmt.Errorf("mongodb adapter: %w", err)
	}

	m, err := model.NewModelFromString(modelConf)
	if err != nil {
		return nil, fmt.Errorf("parse model: %w", err)
	}

	e, err := casbin.NewEnforcer(m, a)
	if err != nil {
		return nil, fmt.Errorf("new enforcer: %w", err)
	}

	enf := &Enforcer{
		e:         e,
		functions: map[string]CustomFunc{},
	}
	enf.registerBuiltinFunctions()
	return enf, nil
}

// Check evaluates whether subject can do action on object given a request context.
func (en *Enforcer) Check(_ context.Context, sub, obj, act string, reqCtx map[string]any) (bool, error) {
	en.mu.RLock()
	defer en.mu.RUnlock()
	return en.e.Enforce(sub, obj, act, reqCtx)
}

func (en *Enforcer) LoadPolicies() error {
	en.mu.Lock()
	defer en.mu.Unlock()
	return en.e.LoadPolicy()
}

func (en *Enforcer) AddPolicy(sub, obj, act, condition string) (bool, error) {
	en.mu.Lock()
	defer en.mu.Unlock()
	return en.e.AddPolicy(sub, obj, act, condition)
}

func (en *Enforcer) RemovePolicy(sub, obj, act, condition string) (bool, error) {
	en.mu.Lock()
	defer en.mu.Unlock()
	return en.e.RemovePolicy(sub, obj, act, condition)
}

func (en *Enforcer) AllPolicies() ([][]string, error) {
	en.mu.RLock()
	defer en.mu.RUnlock()
	return en.e.GetPolicy()
}

func (en *Enforcer) AddRoleForUser(user, role string) (bool, error) {
	en.mu.Lock()
	defer en.mu.Unlock()
	return en.e.AddRoleForUser(user, role)
}

// RegisterFunction adds a custom function available in policy `condition` expression.
func (en *Enforcer) RegisterFunction(name string, fn CustomFunc) {
	en.mu.Lock()
	defer en.mu.Unlock()
	en.functions[name] = fn

	en.e.AddFunction(name, func(args ...any) (any, error) {
		var reqCtx map[string]any
		var rest []any
		if len(args) > 0 {
			if m, ok := args[0].(map[string]any); ok {
				reqCtx = m
				rest = args[1:]
			} else {
				rest = args
			}
		}
		return fn(reqCtx, rest...)
	})
}
