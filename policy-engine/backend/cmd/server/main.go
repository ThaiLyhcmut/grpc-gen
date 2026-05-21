package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/thaily/policy-engine/backend/internal/api"
	"github.com/thaily/policy-engine/backend/internal/auth"
	"github.com/thaily/policy-engine/backend/internal/enforcer"
	"github.com/thaily/policy-engine/backend/internal/store"
)

func main() {
	_ = godotenv.Load() // optional .env

	mongoURI := envDefault("MONGO_URI", "mongodb://localhost:27017")
	dbName := envDefault("MONGO_DB", "lms_policy")
	collection := envDefault("MONGO_COLLECTION", "casbin_rule")
	jwtSecret := envDefault("JWT_SECRET", "dev-secret-change-me")
	port := envDefault("PORT", "8090")

	// Admin accounts: env var ADMIN_ACCOUNTS="alice@x.com:pw1,bob@x.com:pw2"
	admins := parseAdmins(os.Getenv("ADMIN_ACCOUNTS"))
	if len(admins) == 0 {
		admins["admin@local"] = "admin" // default dev account
	}

	ctx := context.Background()
	client, err := store.ConnectMongo(ctx, mongoURI)
	if err != nil {
		log.Fatalf("mongo: %v", err)
	}
	defer client.Disconnect(ctx)

	enf, err := enforcer.NewWithClient(client, dbName, collection)
	if err != nil {
		log.Fatalf("enforcer: %v", err)
	}
	auditStore := store.NewAuditStore(client.Database(dbName), "")

	policyH := &api.PolicyHandler{Enforcer: enf, Audit: auditStore}
	testH := &api.TestHandler{Enforcer: enf}
	auditH := &api.AuditHandler{Store: auditStore}
	authH := &api.AuthHandler{
		JWTSecret:     jwtSecret,
		AdminAccounts: admins,
		TokenLifetime: 24 * time.Hour,
	}

	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowHeaders: []string{"*"},
		AllowMethods: []string{"*"},
	}))

	// Public: login + health
	e.POST("/login", authH.Login)
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{"status": "ok"})
	})

	// Authenticated admin API
	apiGroup := e.Group("/api", auth.Middleware(jwtSecret))
	apiGroup.GET("/policies", policyH.List)
	apiGroup.POST("/policies", policyH.Create)
	apiGroup.PUT("/policies", policyH.Update)
	apiGroup.DELETE("/policies", policyH.Delete)
	apiGroup.POST("/policies/reload", policyH.Reload)
	apiGroup.POST("/test", testH.Run)
	apiGroup.GET("/audit", auditH.List)

	log.Printf("Policy engine listening on :%s", port)
	if err := e.Start(":" + port); err != nil {
		log.Fatal(err)
	}
}

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseAdmins(s string) map[string]string {
	out := map[string]string{}
	if s == "" {
		return out
	}
	for _, pair := range strings.Split(s, ",") {
		kv := strings.SplitN(pair, ":", 2)
		if len(kv) == 2 {
			out[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return out
}
