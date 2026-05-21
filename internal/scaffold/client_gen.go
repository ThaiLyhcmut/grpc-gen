package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// generateClients generates gRPC client files for each service
func generateClients(serverDir string, protos []ProtoInfo, modulePath string) error {
	// Generate Redis client first
	if err := generateRedisClient(serverDir, modulePath); err != nil {
		return fmt.Errorf("failed to generate redis client: %w", err)
	}

	for _, proto := range protos {
		if len(proto.Services) == 0 {
			continue
		}

		if err := generateClientFile(serverDir, proto, modulePath); err != nil {
			return err
		}
	}
	return nil
}

// generateClientFile generates a single client file with Redis caching support
func generateClientFile(serverDir string, proto ProtoInfo, modulePath string) error {
	var sb strings.Builder

	packageName := proto.PackageName
	serviceName := ""
	if len(proto.Services) > 0 {
		serviceName = proto.Services[0].Name
	}

	// Check if we need common import (for List methods)
	needsCommonImport := false
	for _, service := range proto.Services {
		for _, method := range service.Methods {
			if strings.HasPrefix(method.RequestType, "List") {
				needsCommonImport = true
				break
			}
		}
	}

	// Derive import path for proto
	protoImportPath := fmt.Sprintf("%s/proto/%s", modulePath, packageName)
	commonImportPath := fmt.Sprintf("%s/proto/common", modulePath)

	sb.WriteString("package client\n\n")
	sb.WriteString("import (\n")
	sb.WriteString("\t\"context\"\n")
	sb.WriteString("\t\"fmt\"\n")
	sb.WriteString("\t\"log\"\n")
	sb.WriteString("\t\"os\"\n")
	sb.WriteString("\t\"time\"\n\n")
	sb.WriteString(fmt.Sprintf("\tpb \"%s\"\n", protoImportPath))
	if needsCommonImport {
		sb.WriteString(fmt.Sprintf("\tpbCommon \"%s\"\n", commonImportPath))
	}
	sb.WriteString(fmt.Sprintf("\t\"%s/src/server/pkg/tls\"\n\n", modulePath))
	sb.WriteString("\t\"github.com/redis/go-redis/v9\"\n")
	sb.WriteString("\t\"google.golang.org/grpc\"\n")
	sb.WriteString(")\n\n")

	// Cache constants
	sb.WriteString(fmt.Sprintf("const (\n"))
	sb.WriteString(fmt.Sprintf("\t// Cache TTL for %s\n", packageName))
	sb.WriteString(fmt.Sprintf("\t%sCacheTTL = 10 * time.Minute\n\n", packageName))
	sb.WriteString(fmt.Sprintf("\t// Cache key prefix for %s\n", packageName))
	sb.WriteString(fmt.Sprintf("\t%sCachePrefix = \"%s:\"\n", packageName, packageName))
	sb.WriteString(")\n\n")

	// Client struct
	clientStructName := fmt.Sprintf("GRPC%sClient", strings.Title(packageName))
	sb.WriteString(fmt.Sprintf("// %s is the gRPC client for %s service with optional Redis caching\n", clientStructName, serviceName))
	sb.WriteString(fmt.Sprintf("type %s struct {\n", clientStructName))
	sb.WriteString("\tconn        *grpc.ClientConn\n")
	sb.WriteString(fmt.Sprintf("\tclient      pb.%sClient\n", serviceName))
	sb.WriteString("\tredisClient *redis.Client // Can be nil if Redis is disabled\n")
	sb.WriteString("}\n\n")

	// Constructor
	sb.WriteString(fmt.Sprintf("// New%sClient creates a new %s gRPC client\n", strings.Title(packageName), packageName))
	sb.WriteString(fmt.Sprintf("// redisClient can be nil if caching is not needed\n"))
	sb.WriteString(fmt.Sprintf("func New%sClient(redisClient *redis.Client) (*%s, error) {\n", strings.Title(packageName), clientStructName))
	sb.WriteString(fmt.Sprintf("\taddr := os.Getenv(\"%s_SERVICE_ADDR\")\n", strings.ToUpper(packageName)))
	sb.WriteString("\tif addr == \"\" {\n")
	sb.WriteString(fmt.Sprintf("\t\taddr = \"localhost:50051\" // default %s service address\n", packageName))
	sb.WriteString("\t}\n\n")
	sb.WriteString("\t// Load TLS credentials\n")
	sb.WriteString(fmt.Sprintf("\tcreds, err := tls.LoadClientTLSCredentials(\"%s-service\")\n", packageName))
	sb.WriteString("\tif err != nil {\n")
	sb.WriteString("\t\treturn nil, fmt.Errorf(\"failed to load TLS credentials: %w\", err)\n")
	sb.WriteString("\t}\n\n")
	sb.WriteString("\tconn, err := grpc.Dial(addr, grpc.WithTransportCredentials(creds))\n")
	sb.WriteString("\tif err != nil {\n")
	sb.WriteString(fmt.Sprintf("\t\treturn nil, fmt.Errorf(\"failed to connect to %s service: %%w\", err)\n", packageName))
	sb.WriteString("\t}\n\n")
	sb.WriteString(fmt.Sprintf("\treturn &%s{\n", clientStructName))
	sb.WriteString("\t\tconn:        conn,\n")
	sb.WriteString(fmt.Sprintf("\t\tclient:      pb.New%sClient(conn),\n", serviceName))
	sb.WriteString("\t\tredisClient: redisClient,\n")
	sb.WriteString("\t}, nil\n")
	sb.WriteString("}\n\n")

	// Close method
	sb.WriteString("// Close closes the gRPC connection\n")
	sb.WriteString(fmt.Sprintf("func (g *%s) Close() error {\n", clientStructName))
	sb.WriteString("\tif g.conn != nil {\n")
	sb.WriteString("\t\treturn g.conn.Close()\n")
	sb.WriteString("\t}\n")
	sb.WriteString("\treturn nil\n")
	sb.WriteString("}\n\n")

	// Generate methods for each RPC
	for _, service := range proto.Services {
		for _, method := range service.Methods {
			generateClientMethodWithCache(&sb, clientStructName, method, packageName)
		}
	}

	// Write to file
	clientFile := filepath.Join(serverDir, "client", packageName+".go")
	return os.WriteFile(clientFile, []byte(sb.String()), 0644)
}

// generateClientMethodWithCache generates a single client method with Redis caching
func generateClientMethodWithCache(sb *strings.Builder, clientStructName string, method MethodInfo, packageName string) {
	methodName := method.Name
	reqType := method.RequestType
	respType := method.ResponseType

	// Determine method signature based on request type
	var params string
	var callArgs string

	// Parse the request type to determine parameters
	if strings.HasPrefix(reqType, "Get") && strings.HasSuffix(reqType, "Request") {
		// Get by ID methods - with caching
		generateGetMethodWithCache(sb, clientStructName, methodName, reqType, respType, packageName)
		return
	} else if strings.HasPrefix(reqType, "Delete") && strings.HasSuffix(reqType, "Request") {
		// Delete methods - invalidate cache
		generateDeleteMethodWithCache(sb, clientStructName, methodName, reqType, respType, packageName)
		return
	} else if strings.HasPrefix(reqType, "List") && strings.HasSuffix(reqType, "Request") {
		// List methods - with caching
		generateListMethodWithCache(sb, clientStructName, methodName, reqType, respType, packageName)
		return
	} else if strings.HasPrefix(reqType, "Create") && strings.HasSuffix(reqType, "Request") {
		// Create methods - invalidate cache pattern
		generateCreateMethodWithCache(sb, clientStructName, methodName, reqType, respType, packageName)
		return
	} else if strings.HasPrefix(reqType, "Update") && strings.HasSuffix(reqType, "Request") {
		// Update methods - invalidate cache
		generateUpdateMethodWithCache(sb, clientStructName, methodName, reqType, respType, packageName)
		return
	} else {
		// Generic method
		params = fmt.Sprintf("ctx context.Context, req *pb.%s", reqType)
		callArgs = "req"
	}

	sb.WriteString(fmt.Sprintf("// %s calls the %s RPC\n", methodName, methodName))
	sb.WriteString(fmt.Sprintf("func (g *%s) %s(%s) (*pb.%s, error) {\n", clientStructName, methodName, params, respType))
	sb.WriteString(fmt.Sprintf("\tresp, err := g.client.%s(ctx, %s)\n", methodName, callArgs))
	sb.WriteString("\tif err != nil {\n")
	sb.WriteString("\t\treturn nil, err\n")
	sb.WriteString("\t}\n")
	sb.WriteString("\treturn resp, nil\n")
	sb.WriteString("}\n\n")
}

// generateGetMethodWithCache generates a Get method with Redis caching
func generateGetMethodWithCache(sb *strings.Builder, clientStructName, methodName, reqType, respType, packageName string) {
	sb.WriteString(fmt.Sprintf("// %s calls the %s RPC with caching\n", methodName, methodName))
	sb.WriteString(fmt.Sprintf("func (g *%s) %s(ctx context.Context, id string) (*pb.%s, error) {\n", clientStructName, methodName, respType))

	// Cache check
	sb.WriteString(fmt.Sprintf("\tcacheKey := fmt.Sprintf(\"%%s%%s\", %sCachePrefix, id)\n", packageName))
	sb.WriteString(fmt.Sprintf("\tvar cached pb.%s\n", respType))
	sb.WriteString("\tif hit, _ := GetCachedProto(ctx, g.redisClient, cacheKey, &cached); hit {\n")
	sb.WriteString(fmt.Sprintf("\t\tlog.Printf(\"Cache HIT for %s: %%s\", id)\n", packageName))
	sb.WriteString("\t\treturn &cached, nil\n")
	sb.WriteString("\t}\n\n")

	// Cache miss - call gRPC
	sb.WriteString(fmt.Sprintf("\tlog.Printf(\"Cache MISS for %s: %%s\", id)\n", packageName))
	sb.WriteString(fmt.Sprintf("\tresp, err := g.client.%s(ctx, &pb.%s{Id: id})\n", methodName, reqType))
	sb.WriteString("\tif err != nil {\n")
	sb.WriteString("\t\treturn nil, err\n")
	sb.WriteString("\t}\n\n")

	// Store in cache
	sb.WriteString(fmt.Sprintf("\tSetCachedProto(ctx, g.redisClient, cacheKey, resp, %sCacheTTL)\n", packageName))
	sb.WriteString("\treturn resp, nil\n")
	sb.WriteString("}\n\n")
}

// generateListMethodWithCache generates a List method with Redis caching
func generateListMethodWithCache(sb *strings.Builder, clientStructName, methodName, reqType, respType, packageName string) {
	sb.WriteString(fmt.Sprintf("// %s calls the %s RPC with caching\n", methodName, methodName))
	sb.WriteString(fmt.Sprintf("func (g *%s) %s(ctx context.Context, search *pbCommon.SearchRequest) (*pb.%s, error) {\n", clientStructName, methodName, respType))

	// Cache check
	sb.WriteString(fmt.Sprintf("\tcacheKey := GenerateCacheKey(%sCachePrefix+\"list:\", search)\n", packageName))
	sb.WriteString(fmt.Sprintf("\tvar cached pb.%s\n", respType))
	sb.WriteString("\tif hit, _ := GetCachedProto(ctx, g.redisClient, cacheKey, &cached); hit {\n")
	sb.WriteString(fmt.Sprintf("\t\tlog.Printf(\"Cache HIT for %s list\")\n", packageName))
	sb.WriteString("\t\treturn &cached, nil\n")
	sb.WriteString("\t}\n\n")

	// Cache miss - call gRPC
	sb.WriteString(fmt.Sprintf("\tlog.Printf(\"Cache MISS for %s list\")\n", packageName))
	sb.WriteString(fmt.Sprintf("\tresp, err := g.client.%s(ctx, &pb.%s{Search: search})\n", methodName, reqType))
	sb.WriteString("\tif err != nil {\n")
	sb.WriteString("\t\treturn nil, err\n")
	sb.WriteString("\t}\n\n")

	// Store in cache
	sb.WriteString(fmt.Sprintf("\tSetCachedProto(ctx, g.redisClient, cacheKey, resp, %sCacheTTL)\n", packageName))
	sb.WriteString("\treturn resp, nil\n")
	sb.WriteString("}\n\n")
}

// generateCreateMethodWithCache generates a Create method that invalidates cache
func generateCreateMethodWithCache(sb *strings.Builder, clientStructName, methodName, reqType, respType, packageName string) {
	sb.WriteString(fmt.Sprintf("// %s calls the %s RPC and invalidates list cache\n", methodName, methodName))
	sb.WriteString(fmt.Sprintf("func (g *%s) %s(ctx context.Context, req *pb.%s) (*pb.%s, error) {\n", clientStructName, methodName, reqType, respType))

	// Call gRPC
	sb.WriteString(fmt.Sprintf("\tresp, err := g.client.%s(ctx, req)\n", methodName))
	sb.WriteString("\tif err != nil {\n")
	sb.WriteString("\t\treturn nil, err\n")
	sb.WriteString("\t}\n\n")

	// Invalidate list cache
	sb.WriteString(fmt.Sprintf("\t// Invalidate list cache\n"))
	sb.WriteString(fmt.Sprintf("\tInvalidateCacheByPattern(ctx, g.redisClient, %sCachePrefix+\"list:*\")\n", packageName))
	sb.WriteString("\treturn resp, nil\n")
	sb.WriteString("}\n\n")
}

// generateUpdateMethodWithCache generates an Update method that invalidates cache
func generateUpdateMethodWithCache(sb *strings.Builder, clientStructName, methodName, reqType, respType, packageName string) {
	sb.WriteString(fmt.Sprintf("// %s calls the %s RPC and invalidates cache\n", methodName, methodName))
	sb.WriteString(fmt.Sprintf("func (g *%s) %s(ctx context.Context, req *pb.%s) (*pb.%s, error) {\n", clientStructName, methodName, reqType, respType))

	// Call gRPC
	sb.WriteString(fmt.Sprintf("\tresp, err := g.client.%s(ctx, req)\n", methodName))
	sb.WriteString("\tif err != nil {\n")
	sb.WriteString("\t\treturn nil, err\n")
	sb.WriteString("\t}\n\n")

	// Invalidate cache
	sb.WriteString("\t// Invalidate cache for this item and list cache\n")
	sb.WriteString("\tif req.Id != \"\" {\n")
	sb.WriteString(fmt.Sprintf("\t\tcacheKey := fmt.Sprintf(\"%%s%%s\", %sCachePrefix, req.Id)\n", packageName))
	sb.WriteString("\t\tInvalidateCacheByKey(ctx, g.redisClient, cacheKey)\n")
	sb.WriteString("\t}\n")
	sb.WriteString(fmt.Sprintf("\tInvalidateCacheByPattern(ctx, g.redisClient, %sCachePrefix+\"list:*\")\n", packageName))
	sb.WriteString("\treturn resp, nil\n")
	sb.WriteString("}\n\n")
}

// generateDeleteMethodWithCache generates a Delete method that invalidates cache
func generateDeleteMethodWithCache(sb *strings.Builder, clientStructName, methodName, reqType, respType, packageName string) {
	sb.WriteString(fmt.Sprintf("// %s calls the %s RPC and invalidates cache\n", methodName, methodName))
	sb.WriteString(fmt.Sprintf("func (g *%s) %s(ctx context.Context, id string) (*pb.%s, error) {\n", clientStructName, methodName, respType))

	// Call gRPC
	sb.WriteString(fmt.Sprintf("\tresp, err := g.client.%s(ctx, &pb.%s{Id: id})\n", methodName, reqType))
	sb.WriteString("\tif err != nil {\n")
	sb.WriteString("\t\treturn nil, err\n")
	sb.WriteString("\t}\n\n")

	// Invalidate cache
	sb.WriteString("\t// Invalidate cache for this item and list cache\n")
	sb.WriteString(fmt.Sprintf("\tcacheKey := fmt.Sprintf(\"%%s%%s\", %sCachePrefix, id)\n", packageName))
	sb.WriteString("\tInvalidateCacheByKey(ctx, g.redisClient, cacheKey)\n")
	sb.WriteString(fmt.Sprintf("\tInvalidateCacheByPattern(ctx, g.redisClient, %sCachePrefix+\"list:*\")\n", packageName))
	sb.WriteString("\treturn resp, nil\n")
	sb.WriteString("}\n\n")
}
