package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// generateController generates the controller base file
func generateController(serverDir string, protos []ProtoInfo, modulePath string) error {
	var sb strings.Builder

	sb.WriteString("package controller\n\n")
	sb.WriteString("import (\n")
	sb.WriteString(fmt.Sprintf("\tpb \"%s/proto/common\"\n", modulePath))
	sb.WriteString(fmt.Sprintf("\t\"%s/src/server/client\"\n", modulePath))
	sb.WriteString(fmt.Sprintf("\t\"%s/src/server/graph/model\"\n", modulePath))
	sb.WriteString(")\n\n")

	// Controller struct
	sb.WriteString("// Controller handles business logic and calls gRPC clients\n")
	sb.WriteString("type Controller struct {\n")
	for _, proto := range protos {
		if len(proto.Services) > 0 {
			fieldName := proto.PackageName
			clientType := fmt.Sprintf("*client.GRPC%sClient", strings.Title(proto.PackageName))
			sb.WriteString(fmt.Sprintf("\t%s %s\n", fieldName, clientType))
		}
	}
	sb.WriteString("}\n\n")

	// Constructor
	sb.WriteString("// NewController creates a new Controller with all gRPC clients\n")
	sb.WriteString("func NewController(\n")
	params := []string{}
	for _, proto := range protos {
		if len(proto.Services) > 0 {
			paramName := proto.PackageName
			clientType := fmt.Sprintf("*client.GRPC%sClient", strings.Title(proto.PackageName))
			params = append(params, fmt.Sprintf("\t%s %s", paramName, clientType))
		}
	}
	sb.WriteString(strings.Join(params, ",\n"))
	sb.WriteString(",\n) *Controller {\n")
	sb.WriteString("\treturn &Controller{\n")
	for _, proto := range protos {
		if len(proto.Services) > 0 {
			sb.WriteString(fmt.Sprintf("\t\t%s: %s,\n", proto.PackageName, proto.PackageName))
		}
	}
	sb.WriteString("\t}\n")
	sb.WriteString("}\n\n")

	// Add filter conversion helpers
	sb.WriteString(getControllerHelpers())

	controllerFile := filepath.Join(serverDir, "graph", "controller", "controller.go")
	return os.WriteFile(controllerFile, []byte(sb.String()), 0644)
}

// getControllerHelpers returns the filter/pagination conversion helper functions
func getControllerHelpers() string {
	return `// ============================================
// PAGINATION & SEARCH HELPERS
// ============================================

// DefaultPagination returns default pagination settings
func (c *Controller) DefaultPagination() *model.PaginationInput {
	page := int32(1)
	pageSize := int32(20)
	sortBy := "created_at"
	descending := true
	return &model.PaginationInput{
		Page:       &page,
		PageSize:   &pageSize,
		SortBy:     &sortBy,
		Descending: &descending,
	}
}

// ConvertSearchRequestToPB converts GraphQL SearchRequestInput to Protobuf SearchRequest
func (c *Controller) ConvertSearchRequestToPB(input *model.SearchRequestInput) *pb.SearchRequest {
	if input == nil {
		return nil
	}

	if input.Pagination == nil && (input.Filters == nil || len(input.Filters) == 0) {
		return nil
	}

	req := &pb.SearchRequest{}

	if input.Pagination != nil {
		req.Pagination = convertPaginationToPB(input.Pagination)
	}

	if input.Filters != nil && len(input.Filters) > 0 {
		req.Filters = make([]*pb.FilterCriteria, 0, len(input.Filters))
		for _, filter := range input.Filters {
			if filter != nil {
				req.Filters = append(req.Filters, convertFilterCriteriaToPB(filter))
			}
		}
	}

	return req
}

// ============================================
// INTERNAL CONVERSION HELPERS
// ============================================

func convertPaginationToPB(input *model.PaginationInput) *pb.Pagination {
	if input == nil {
		return nil
	}

	pagination := &pb.Pagination{}

	if input.Page != nil {
		pagination.Page = *input.Page
	}
	if input.PageSize != nil {
		pagination.PageSize = *input.PageSize
	}
	if input.SortBy != nil {
		pagination.SortBy = *input.SortBy
	}
	if input.Descending != nil {
		pagination.Descending = *input.Descending
	}

	return pagination
}

func convertFilterCriteriaToPB(input *model.FilterCriteriaInput) *pb.FilterCriteria {
	if input == nil {
		return nil
	}

	criteria := &pb.FilterCriteria{}

	if input.Condition != nil {
		criteria.Criteria = &pb.FilterCriteria_Condition{
			Condition: convertFilterConditionToPB(input.Condition),
		}
	} else if input.Group != nil {
		criteria.Criteria = &pb.FilterCriteria_Group{
			Group: convertFilterGroupToPB(input.Group),
		}
	}

	return criteria
}

func convertFilterConditionToPB(input *model.FilterConditionInput) *pb.FilterCondition {
	if input == nil {
		return nil
	}

	return &pb.FilterCondition{
		Field:    input.Field,
		Operator: convertFilterOperatorToPB(input.Operator),
		Values:   input.Values,
	}
}

func convertFilterGroupToPB(input *model.FilterGroupInput) *pb.FilterGroup {
	if input == nil {
		return nil
	}

	group := &pb.FilterGroup{}

	if input.Logic != nil {
		group.Logic = convertLogicalConditionToPB(*input.Logic)
	}

	if input.Filters != nil && len(input.Filters) > 0 {
		group.Filters = make([]*pb.FilterCriteria, 0, len(input.Filters))
		for _, filter := range input.Filters {
			if filter != nil {
				group.Filters = append(group.Filters, convertFilterCriteriaToPB(filter))
			}
		}
	}

	return group
}

func convertFilterOperatorToPB(op model.FilterOperator) pb.FilterOperator {
	switch op {
	case model.FilterOperatorEqual:
		return pb.FilterOperator_EQUAL
	case model.FilterOperatorNotEqual:
		return pb.FilterOperator_NOT_EQUAL
	case model.FilterOperatorGreaterThan:
		return pb.FilterOperator_GREATER_THAN
	case model.FilterOperatorGreaterThanEqual:
		return pb.FilterOperator_GREATER_THAN_EQUAL
	case model.FilterOperatorLessThan:
		return pb.FilterOperator_LESS_THAN
	case model.FilterOperatorLessThanEqual:
		return pb.FilterOperator_LESS_THAN_EQUAL
	case model.FilterOperatorLike:
		return pb.FilterOperator_LIKE
	case model.FilterOperatorIn:
		return pb.FilterOperator_IN
	case model.FilterOperatorNotIn:
		return pb.FilterOperator_NOT_IN
	case model.FilterOperatorIsNull:
		return pb.FilterOperator_IS_NULL
	case model.FilterOperatorIsNotNull:
		return pb.FilterOperator_IS_NOT_NULL
	case model.FilterOperatorBetween:
		return pb.FilterOperator_BETWEEN
	default:
		return pb.FilterOperator_EQUAL
	}
}

func convertLogicalConditionToPB(cond model.LogicalCondition) pb.LogicalCondition {
	switch cond {
	case model.LogicalConditionAnd:
		return pb.LogicalCondition_AND
	case model.LogicalConditionOr:
		return pb.LogicalCondition_OR
	default:
		return pb.LogicalCondition_AND
	}
}
`
}

// generateResolverBase generates the resolver base file
func generateResolverBase(serverDir string, modulePath string) error {
	content := fmt.Sprintf(`package resolver

import (
	"%s/src/server/graph/controller"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

type Resolver struct {
	Controller *controller.Controller
}

// NewResolver creates a new resolver with controller
func NewResolver(ctrl *controller.Controller) *Resolver {
	return &Resolver{
		Controller: ctrl,
	}
}
`, modulePath)

	resolverFile := filepath.Join(serverDir, "graph", "resolver", "resolver.go")
	return os.WriteFile(resolverFile, []byte(content), 0644)
}

// generateConfig generates the config file
func generateConfig(serverDir string) error {
	content := `package config

import (
	"os"
)

// Config holds the server configuration
type Config struct {
	Port        string
	Environment string

	// TLS
	CertsPath string

	// Service addresses
	ServiceAddresses map[string]string
}

// Load loads configuration from environment variables
func Load() *Config {
	cfg := &Config{
		Port:             getEnv("SERVER_PORT", "8080"),
		Environment:      getEnv("ENVIRONMENT", "development"),
		CertsPath:        getEnv("CERTS_PATH", "./certs"),
		ServiceAddresses: make(map[string]string),
	}

	return cfg
}

// getEnv gets environment variable with default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// IsProduction returns true if running in production
func (c *Config) IsProduction() bool {
	return c.Environment == "production"
}
`
	configFile := filepath.Join(serverDir, "config", "config.go")
	return os.WriteFile(configFile, []byte(content), 0644)
}

// generateTLSPackage generates the TLS utilities
func generateTLSPackage(serverDir string) error {
	content := `package tls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"

	"google.golang.org/grpc/credentials"
)

// GetCertsPath returns the path to certs directory
func GetCertsPath() string {
	if certsPath := os.Getenv("CERTS_PATH"); certsPath != "" {
		return certsPath
	}
	return "./certs"
}

// LoadClientTLSCredentials loads client TLS credentials for mTLS
func LoadClientTLSCredentials(serverName string) (credentials.TransportCredentials, error) {
	certsPath := GetCertsPath()

	// Load client certificate and private key
	clientCert := filepath.Join(certsPath, "clients", "client.crt")
	clientKey := filepath.Join(certsPath, "clients", "client.key")

	certificate, err := tls.LoadX509KeyPair(clientCert, clientKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	// Load CA certificate for server verification
	caCert := filepath.Join(certsPath, "clients", "ca.crt")
	caPool := x509.NewCertPool()

	ca, err := os.ReadFile(caCert)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	if !caPool.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("failed to append CA certificate")
	}

	// Create TLS configuration
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{certificate},
		RootCAs:      caPool,
		ServerName:   serverName,
		MinVersion:   tls.VersionTLS12,
	}

	return credentials.NewTLS(tlsConfig), nil
}

// LoadClientTLSCredentialsInsecure loads client TLS without client cert (one-way TLS)
func LoadClientTLSCredentialsInsecure(serverName string) (credentials.TransportCredentials, error) {
	certsPath := GetCertsPath()

	// Load CA certificate for server verification
	caCert := filepath.Join(certsPath, "clients", "ca.crt")
	caPool := x509.NewCertPool()

	ca, err := os.ReadFile(caCert)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	if !caPool.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("failed to append CA certificate")
	}

	tlsConfig := &tls.Config{
		RootCAs:    caPool,
		ServerName: serverName,
		MinVersion: tls.VersionTLS12,
	}

	return credentials.NewTLS(tlsConfig), nil
}
`
	tlsFile := filepath.Join(serverDir, "pkg", "tls", "tls.go")
	return os.WriteFile(tlsFile, []byte(content), 0644)
}

// generateRouter generates the router file
func generateRouter(serverDir string, modulePath string) error {
	content := fmt.Sprintf(`package router

import (
	"net/http"

	"%s/src/server/graph/generated"
	"%s/src/server/graph/resolver"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// NewRouter creates a new HTTP router
func NewRouter(resolver *resolver.Resolver) http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// GraphQL handler
	srv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{
		Resolvers: resolver,
	}))

	// Routes
	r.Handle("/", playground.Handler("GraphQL Playground", "/query"))
	r.Handle("/query", srv)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	return r
}
`, modulePath, modulePath)

	routerFile := filepath.Join(serverDir, "router", "router.go")
	return os.WriteFile(routerFile, []byte(content), 0644)
}

// generateServerMain generates the main.go file
func generateServerMain(serverDir string, modulePath string, port int) error {
	content := fmt.Sprintf(`package main

import (
	"log"
	"net/http"
	"os"

	"%s/src/server/client"
	"%s/src/server/config"
	"%s/src/server/graph/controller"
	"%s/src/server/graph/resolver"
	"%s/src/server/router"

	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load("server.env"); err != nil {
		log.Printf("Warning: server.env file not found: %%v", err)
	}

	// Load configuration
	cfg := config.Load()
	_ = cfg // Use cfg as needed

	// Initialize Redis client (optional - returns nil if REDIS_ENABLED != "true")
	redisClient, err := client.NewRedisClient()
	if err != nil {
		log.Printf("Warning: Failed to create Redis client: %%v", err)
		// Continue without Redis caching
	}
	if redisClient != nil {
		defer redisClient.Close()
		log.Println("Redis caching enabled")
	} else {
		log.Println("Redis caching disabled")
	}

	// Get Redis client for gRPC clients (can be nil)
	var redisConn = redisClient.GetClient()

	// Initialize gRPC clients
	// TODO: Uncomment and add your service clients here
	// Example:
	// userClient, err := client.NewUserClient(redisConn)
	// if err != nil {
	// 	log.Fatalf("Failed to create user client: %%v", err)
	// }
	// defer userClient.Close()

	_ = redisConn // Remove this line when you add clients

	// Create controller with clients
	ctrl := controller.NewController(
		// Pass your clients here
	)

	// Create resolver
	res := resolver.NewResolver(ctrl)

	// Create router
	r := router.NewRouter(res)

	// Get port
	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "%d"
	}

	log.Printf("GraphQL server starting on http://localhost:%%s", port)
	log.Printf("GraphQL Playground available at http://localhost:%%s/", port)

	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("Failed to start server: %%v", err)
	}
}
`, modulePath, modulePath, modulePath, modulePath, modulePath, port)

	mainFile := filepath.Join(serverDir, "main.go")
	return os.WriteFile(mainFile, []byte(content), 0644)
}

// generateGqlgenConfig generates the gqlgen.yml file
func generateGqlgenConfig(serverDir string, modulePath string) error {
	content := fmt.Sprintf(`# gqlgen configuration
# See https://gqlgen.com/config/ for more information

schema:
  - graph/schema/*.graphqls

exec:
  filename: graph/generated/generated.go
  package: generated

model:
  filename: graph/model/models_gen.go
  package: model

resolver:
  layout: follow-schema
  dir: graph/resolver
  package: resolver
  filename_template: "{name}.resolvers.go"

# Optional: autobind models
autobind:
  - "%s/src/server/graph/model"

# Models configuration
models:
  ID:
    model:
      - github.com/99designs/gqlgen/graphql.ID
      - github.com/99designs/gqlgen/graphql.IntID
  Int:
    model:
      - github.com/99designs/gqlgen/graphql.Int
      - github.com/99designs/gqlgen/graphql.Int64
      - github.com/99designs/gqlgen/graphql.Int32
  Time:
    model:
      - github.com/99designs/gqlgen/graphql.Time
`, modulePath)

	gqlgenFile := filepath.Join(serverDir, "gqlgen.yml")
	return os.WriteFile(gqlgenFile, []byte(content), 0644)
}

// generateServerEnv generates the server.env file
func generateServerEnv(serverDir string, port int, protos []ProtoInfo) error {
	var sb strings.Builder

	sb.WriteString("# Server Configuration\n")
	sb.WriteString(fmt.Sprintf("SERVER_PORT=%d\n", port))
	sb.WriteString("ENVIRONMENT=development\n\n")

	sb.WriteString("# TLS Configuration\n")
	sb.WriteString("CERTS_PATH=./certs\n\n")

	sb.WriteString("# Redis Configuration (optional - set REDIS_ENABLED=true to enable caching)\n")
	sb.WriteString("REDIS_ENABLED=false\n")
	sb.WriteString("REDIS_ADDRESS=localhost:6379\n")
	sb.WriteString("REDIS_PASSWORD=\n")
	sb.WriteString("REDIS_DB=0\n")
	sb.WriteString("REDIS_DIAL_TIMEOUT=5\n")
	sb.WriteString("REDIS_READ_TIMEOUT=3\n")
	sb.WriteString("REDIS_WRITE_TIMEOUT=3\n")
	sb.WriteString("REDIS_POOL_SIZE=10\n")
	sb.WriteString("REDIS_MIN_IDLE_CONNS=5\n\n")

	sb.WriteString("# Service Addresses\n")
	basePort := 50051
	for i, proto := range protos {
		envKey := fmt.Sprintf("%s_SERVICE_ADDR", strings.ToUpper(proto.PackageName))
		sb.WriteString(fmt.Sprintf("%s=localhost:%d\n", envKey, basePort+i))
	}

	envFile := filepath.Join(serverDir, "server.env")
	return os.WriteFile(envFile, []byte(sb.String()), 0644)
}

// generateServerDockerfile generates the Dockerfile
func generateServerDockerfile(serverDir string, port int) error {
	content := fmt.Sprintf(`# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the server
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

# Runtime stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates for TLS
RUN apk --no-cache add ca-certificates

# Copy binary from builder
COPY --from=builder /app/server .

# Copy certs directory (for client certs)
COPY --from=builder /app/certs ./certs

# Expose port
EXPOSE %d

# Run the server
CMD ["./server"]
`, port)

	dockerFile := filepath.Join(serverDir, "Dockerfile")
	return os.WriteFile(dockerFile, []byte(content), 0644)
}

// generateServerDockerCompose generates the docker-compose.yml
func generateServerDockerCompose(serverDir string, modulePath string, port int) error {
	content := fmt.Sprintf(`version: '3.8'

services:
  graphql-server:
    build:
      context: .
      dockerfile: Dockerfile
    image: %s/graphql-server:latest
    container_name: graphql_server
    env_file:
      - server.env
    ports:
      - "%d:%d"
    volumes:
      - ./certs:/app/certs:ro
    networks:
      - server_network
    restart: unless-stopped

networks:
  server_network:
    driver: bridge
`, modulePath, port, port)

	composeFile := filepath.Join(serverDir, "docker-compose.yml")
	return os.WriteFile(composeFile, []byte(content), 0644)
}

// generateServerGoMod generates go.mod for the server
func generateServerGoMod(serverDir string, modulePath string) error {
	content := fmt.Sprintf(`module %s/src/server

go 1.24

require (
	github.com/99designs/gqlgen v0.17.49
	github.com/go-chi/chi/v5 v5.0.12
	github.com/go-chi/cors v1.2.1
	github.com/joho/godotenv v1.5.1
	github.com/redis/go-redis/v9 v9.5.1
	github.com/vektah/gqlparser/v2 v2.5.16
	google.golang.org/grpc v1.64.0
	google.golang.org/protobuf v1.34.2
)
`, modulePath)

	goModFile := filepath.Join(serverDir, "go.mod")
	return os.WriteFile(goModFile, []byte(content), 0644)
}
