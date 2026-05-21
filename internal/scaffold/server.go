package scaffold

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ProtoInfo holds parsed information from proto files
type ProtoInfo struct {
	PackageName string
	GoPackage   string
	Messages    []MessageInfo
	Enums       []EnumInfo
	Services    []ServiceInfo
}

// MessageInfo holds message definition
type MessageInfo struct {
	Name   string
	Fields []FieldInfo
}

// FieldInfo holds field definition
type FieldInfo struct {
	Name       string
	Type       string
	IsOptional bool
	IsRepeated bool
}

// EnumInfo holds enum definition
type EnumInfo struct {
	Name   string
	Values []string
}

// ServiceInfo holds service definition
type ServiceInfo struct {
	Name    string
	Methods []MethodInfo
}

// MethodInfo holds RPC method definition
type MethodInfo struct {
	Name         string
	RequestType  string
	ResponseType string
}

// AddServer creates a GraphQL server scaffold
func AddServer(port int) error {
	// Check if in a valid project
	if _, err := os.Stat("go.mod"); os.IsNotExist(err) {
		return fmt.Errorf("not in a project directory (go.mod not found)")
	}

	// Get module path
	modulePath, err := getModulePath()
	if err != nil {
		return fmt.Errorf("failed to get module path: %w", err)
	}

	// Parse all proto files
	protoInfos, err := parseAllProtoFiles()
	if err != nil {
		return fmt.Errorf("failed to parse proto files: %w", err)
	}

	if len(protoInfos) == 0 {
		return fmt.Errorf("no proto files found. Please add services first using 'grpc-gen add-service'")
	}

	// Create server directory structure
	serverDir := filepath.Join("src", "server")
	dirs := []string{
		filepath.Join(serverDir, "certs", "ca"),
		filepath.Join(serverDir, "certs", "clients"),
		filepath.Join(serverDir, "client"),
		filepath.Join(serverDir, "config"),
		filepath.Join(serverDir, "graph", "schema"),
		filepath.Join(serverDir, "graph", "model"),
		filepath.Join(serverDir, "graph", "generated"),
		filepath.Join(serverDir, "graph", "resolver"),
		filepath.Join(serverDir, "graph", "controller"),
		filepath.Join(serverDir, "graph", "convert"),
		filepath.Join(serverDir, "graph", "directive"),
		filepath.Join(serverDir, "pkg", "tls"),
		filepath.Join(serverDir, "router"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	fmt.Println("  ✓ Created directory structure")

	// Generate types.graphqls from proto files
	if err := generateGraphQLTypes(serverDir, protoInfos); err != nil {
		return fmt.Errorf("failed to generate GraphQL types: %w", err)
	}
	fmt.Println("  ✓ Generated graph/schema/types.graphqls")

	// Generate schema.graphqls (root schema)
	if err := generateRootSchema(serverDir); err != nil {
		return fmt.Errorf("failed to generate root schema: %w", err)
	}
	fmt.Println("  ✓ Generated graph/schema/schema.graphqls")

	// Generate query.graphqls template
	if err := generateQuerySchema(serverDir, protoInfos); err != nil {
		return fmt.Errorf("failed to generate query schema: %w", err)
	}
	fmt.Println("  ✓ Generated graph/schema/query.graphqls")

	// Generate mutation.graphqls template
	if err := generateMutationSchema(serverDir, protoInfos); err != nil {
		return fmt.Errorf("failed to generate mutation schema: %w", err)
	}
	fmt.Println("  ✓ Generated graph/schema/mutation.graphqls")

	// Generate gRPC clients
	if err := generateClients(serverDir, protoInfos, modulePath); err != nil {
		return fmt.Errorf("failed to generate clients: %w", err)
	}
	fmt.Println("  ✓ Generated client/*.go")

	// Generate convert functions and helpers
	if err := generateConvertHelpers(serverDir, modulePath); err != nil {
		return fmt.Errorf("failed to generate convert helpers: %w", err)
	}
	if err := generateConvertFunctions(serverDir, protoInfos, modulePath); err != nil {
		return fmt.Errorf("failed to generate convert functions: %w", err)
	}
	fmt.Println("  ✓ Generated graph/convert/*.go")

	// Generate controller
	if err := generateController(serverDir, protoInfos, modulePath); err != nil {
		return fmt.Errorf("failed to generate controller: %w", err)
	}
	fmt.Println("  ✓ Generated graph/controller/controller.go")

	// Generate resolver base
	if err := generateResolverBase(serverDir, modulePath); err != nil {
		return fmt.Errorf("failed to generate resolver: %w", err)
	}
	fmt.Println("  ✓ Generated graph/resolver/resolver.go")

	// Generate config
	if err := generateConfig(serverDir); err != nil {
		return fmt.Errorf("failed to generate config: %w", err)
	}
	fmt.Println("  ✓ Generated config/config.go")

	// Generate TLS package
	if err := generateTLSPackage(serverDir); err != nil {
		return fmt.Errorf("failed to generate TLS package: %w", err)
	}
	fmt.Println("  ✓ Generated pkg/tls/tls.go")

	// Generate router
	if err := generateRouter(serverDir, modulePath); err != nil {
		return fmt.Errorf("failed to generate router: %w", err)
	}
	fmt.Println("  ✓ Generated router/router.go")

	// Generate main.go
	if err := generateServerMain(serverDir, modulePath, port); err != nil {
		return fmt.Errorf("failed to generate main.go: %w", err)
	}
	fmt.Println("  ✓ Generated main.go")

	// Generate gqlgen.yml
	if err := generateGqlgenConfig(serverDir, modulePath); err != nil {
		return fmt.Errorf("failed to generate gqlgen.yml: %w", err)
	}
	fmt.Println("  ✓ Generated gqlgen.yml")

	// Generate server.env
	if err := generateServerEnv(serverDir, port, protoInfos); err != nil {
		return fmt.Errorf("failed to generate server.env: %w", err)
	}
	fmt.Println("  ✓ Generated server.env")

	// Generate Dockerfile
	if err := generateServerDockerfile(serverDir, port); err != nil {
		return fmt.Errorf("failed to generate Dockerfile: %w", err)
	}
	fmt.Println("  ✓ Generated Dockerfile")

	// Generate docker-compose.yml
	if err := generateServerDockerCompose(serverDir, modulePath, port); err != nil {
		return fmt.Errorf("failed to generate docker-compose.yml: %w", err)
	}
	fmt.Println("  ✓ Generated docker-compose.yml")

	// Generate go.mod for server
	if err := generateServerGoMod(serverDir, modulePath); err != nil {
		return fmt.Errorf("failed to generate go.mod: %w", err)
	}
	fmt.Println("  ✓ Generated go.mod")

	return nil
}

// parseAllProtoFiles reads all proto files from proto/ directory
func parseAllProtoFiles() ([]ProtoInfo, error) {
	protoDir := "proto"
	var protos []ProtoInfo

	entries, err := os.ReadDir(protoDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "common" {
			continue
		}

		protoFile := filepath.Join(protoDir, entry.Name(), entry.Name()+".proto")
		if _, err := os.Stat(protoFile); os.IsNotExist(err) {
			continue
		}

		info, err := parseProtoFile(protoFile)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", protoFile, err)
		}

		protos = append(protos, info)
	}

	return protos, nil
}

// parseProtoFile parses a single proto file
func parseProtoFile(filename string) (ProtoInfo, error) {
	file, err := os.Open(filename)
	if err != nil {
		return ProtoInfo{}, err
	}
	defer file.Close()

	info := ProtoInfo{}
	scanner := bufio.NewScanner(file)

	packageRegex := regexp.MustCompile(`^package\s+(\w+);`)
	goPackageRegex := regexp.MustCompile(`option\s+go_package\s*=\s*"([^"]+)"`)
	messageRegex := regexp.MustCompile(`^message\s+(\w+)\s*\{`)
	enumRegex := regexp.MustCompile(`^enum\s+(\w+)\s*\{`)
	enumValueRegex := regexp.MustCompile(`^\s*(\w+)\s*=\s*\d+;`)
	fieldRegex := regexp.MustCompile(`^\s*(optional\s+)?(repeated\s+)?(\w+(?:\.\w+)*)\s+(\w+)\s*=\s*\d+;`)
	serviceRegex := regexp.MustCompile(`^service\s+(\w+)\s*\{`)
	rpcRegex := regexp.MustCompile(`^\s*rpc\s+(\w+)\s*\(\s*(\w+)\s*\)\s*returns\s*\(\s*(\w+)\s*\)`)

	var currentMessage *MessageInfo
	var currentEnum *EnumInfo
	var currentService *ServiceInfo
	inMessage := false
	inEnum := false
	inService := false
	braceCount := 0

	for scanner.Scan() {
		line := scanner.Text()
		trimmedLine := strings.TrimSpace(line)

		// Skip comments
		if strings.HasPrefix(trimmedLine, "//") {
			continue
		}

		// Package
		if matches := packageRegex.FindStringSubmatch(trimmedLine); len(matches) == 2 {
			info.PackageName = matches[1]
			continue
		}

		// Go package
		if matches := goPackageRegex.FindStringSubmatch(trimmedLine); len(matches) == 2 {
			info.GoPackage = matches[1]
			continue
		}

		// Count braces for tracking scope
		braceCount += strings.Count(trimmedLine, "{") - strings.Count(trimmedLine, "}")

		// Message start
		if matches := messageRegex.FindStringSubmatch(trimmedLine); len(matches) == 2 {
			// Skip Request/Response messages
			msgName := matches[1]
			if !strings.HasSuffix(msgName, "Request") && !strings.HasSuffix(msgName, "Response") {
				currentMessage = &MessageInfo{Name: msgName}
				inMessage = true
			}
			continue
		}

		// Message end
		if inMessage && strings.Contains(trimmedLine, "}") && braceCount <= 0 {
			if currentMessage != nil {
				info.Messages = append(info.Messages, *currentMessage)
			}
			currentMessage = nil
			inMessage = false
			braceCount = 0
			continue
		}

		// Field in message
		if inMessage && currentMessage != nil {
			if matches := fieldRegex.FindStringSubmatch(trimmedLine); len(matches) >= 5 {
				field := FieldInfo{
					Name:       matches[4],
					Type:       matches[3],
					IsOptional: matches[1] != "",
					IsRepeated: matches[2] != "",
				}
				currentMessage.Fields = append(currentMessage.Fields, field)
			}
		}

		// Enum start
		if matches := enumRegex.FindStringSubmatch(trimmedLine); len(matches) == 2 {
			currentEnum = &EnumInfo{Name: matches[1]}
			inEnum = true
			continue
		}

		// Enum end
		if inEnum && strings.Contains(trimmedLine, "}") {
			if currentEnum != nil {
				info.Enums = append(info.Enums, *currentEnum)
			}
			currentEnum = nil
			inEnum = false
			continue
		}

		// Enum value
		if inEnum && currentEnum != nil {
			if matches := enumValueRegex.FindStringSubmatch(trimmedLine); len(matches) == 2 {
				currentEnum.Values = append(currentEnum.Values, matches[1])
			}
		}

		// Service start
		if matches := serviceRegex.FindStringSubmatch(trimmedLine); len(matches) == 2 {
			currentService = &ServiceInfo{Name: matches[1]}
			inService = true
			continue
		}

		// Service end
		if inService && strings.Contains(trimmedLine, "}") && !strings.Contains(trimmedLine, "rpc") {
			if currentService != nil {
				info.Services = append(info.Services, *currentService)
			}
			currentService = nil
			inService = false
			continue
		}

		// RPC method
		if inService && currentService != nil {
			if matches := rpcRegex.FindStringSubmatch(trimmedLine); len(matches) == 4 {
				method := MethodInfo{
					Name:         matches[1],
					RequestType:  matches[2],
					ResponseType: matches[3],
				}
				currentService.Methods = append(currentService.Methods, method)
			}
		}
	}

	return info, scanner.Err()
}
