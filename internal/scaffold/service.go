package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AddService adds a new service to an existing project
func AddService(serviceName string, port int) error {
	// Check if in a valid project
	if _, err := os.Stat("go.mod"); os.IsNotExist(err) {
		return fmt.Errorf("not in a project directory (go.mod not found)")
	}

	serviceLower := strings.ToLower(serviceName)
	serviceTitle := strings.Title(serviceLower)

	// Create proto directory
	protoDir := filepath.Join("proto", serviceLower)
	if err := os.MkdirAll(protoDir, 0755); err != nil {
		return fmt.Errorf("failed to create proto directory: %w", err)
	}

	// Create proto file
	protoFile := filepath.Join(protoDir, serviceLower+".proto")
	if err := createServiceProto(protoFile, serviceLower, serviceTitle); err != nil {
		return fmt.Errorf("failed to create proto file: %w", err)
	}
	fmt.Printf("  ✓ Created %s\n", protoFile)

	// Update Makefile
	if err := addServiceToMakefile(serviceLower, serviceTitle, port); err != nil {
		return fmt.Errorf("failed to update Makefile: %w", err)
	}
	fmt.Printf("  ✓ Updated Makefile\n")

	return nil
}

func createServiceProto(filename, serviceLower, serviceTitle string) error {
	// Read module path
	data, _ := os.ReadFile("go.mod")
	modulePath := "mymodule"
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "module ") {
			modulePath = strings.TrimSpace(strings.TrimPrefix(line, "module "))
			break
		}
	}

	// Get entity name with proper case (e.g., user -> User, post-type -> PostType)
	entityName := toEntityName(serviceLower)
	entityNamePlural := entityName + "s"
	// Get snake_case versions for field names (User -> user, PostType -> post_type)
	entityNameSnake := toSnakeCase(entityName)
	entityNameSnakePlural := entityNameSnake + "s"

	content := fmt.Sprintf(`syntax = "proto3";

package %s;

option go_package = "%s/proto/%s";

import "google/protobuf/timestamp.proto";
import "proto/common/common.proto";

// ============= %s Entity =============
// Example structure - uncomment and modify as needed:
//
// message %s {
//   string id = 1;
//   string name = 2;
//   %sStatus status = 3;
//   google.protobuf.Timestamp created_at = 4;
//   google.protobuf.Timestamp updated_at = 5;
//   string created_by = 6;
//   string updated_by = 7;
// }
//
// enum %sStatus {
//   ACTIVE = 0;
//   INACTIVE = 1;
// }
//
// message Create%sRequest {
//   string name = 1;
//   %sStatus status = 2;
//   string created_by = 3;
// }
//
// message Create%sResponse {
//   %s %s = 1;
// }
//
// message Get%sRequest {
//   string id = 1;
// }
//
// message Get%sResponse {
//   %s %s = 1;
// }
//
// message Update%sRequest {
//   string id = 1;
//   optional string name = 2;
//   optional %sStatus status = 3;
//   string updated_by = 4;
// }
//
// message Update%sResponse {
//   %s %s = 1;
// }
//
// message Delete%sRequest {
//   string id = 1;
// }
//
// message Delete%sResponse {
//   bool success = 1;
// }
//
// message List%sRequest {
//   common.SearchRequest search = 1;
// }
//
// message List%sResponse {
//   repeated %s %s = 1;
//   int32 total = 2;
//   int32 page = 3;
//   int32 page_size = 4;
// }

// ============= Service =============
service %sService {
  // Uncomment and modify these RPC methods as needed:
  // rpc Create%s(Create%sRequest) returns (Create%sResponse);
  // rpc Get%s(Get%sRequest) returns (Get%sResponse);
  // rpc Update%s(Update%sRequest) returns (Update%sResponse);
  // rpc Delete%s(Delete%sRequest) returns (Delete%sResponse);
  // rpc List%s(List%sRequest) returns (List%sResponse);
}
`, serviceLower, modulePath, serviceLower,
		entityName,
		entityName, entityName,
		entityName,
		entityName, entityName,
		entityName, entityName, entityNameSnake,
		entityName,
		entityName, entityName, entityNameSnake,
		entityName, entityName,
		entityName, entityName, entityNameSnake,
		entityName,
		entityName,
		entityNamePlural,
		entityNamePlural, entityName, entityNameSnakePlural,
		serviceTitle,
		entityName, entityName, entityName,
		entityName, entityName, entityName,
		entityName, entityName, entityName,
		entityName, entityName, entityName,
		entityNamePlural, entityNamePlural, entityNamePlural)

	return os.WriteFile(filename, []byte(content), 0644)
}

// toEntityName converts service name to entity name
// Examples: user -> User, post-type -> PostType, user-profile -> UserProfile
func toEntityName(serviceName string) string {
	parts := strings.Split(serviceName, "-")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

// toSnakeCase converts PascalCase to snake_case
// Examples: User -> user, PostType -> post_type, UserProfile -> user_profile
func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

func addServiceToMakefile(serviceLower, serviceTitle string, port int) error {
	// Read existing Makefile
	data, err := os.ReadFile("Makefile")
	if err != nil {
		return err
	}

	content := string(data)

	// Check if service already exists
	if strings.Contains(content, fmt.Sprintf("proto-%s:", serviceLower)) {
		return fmt.Errorf("service %s already exists in Makefile", serviceLower)
	}

	// Find insertion point for proto target
	protoInsert := strings.Index(content, "# Generate all protos")
	if protoInsert == -1 {
		return fmt.Errorf("could not find proto insertion point in Makefile")
	}

	// Add proto target
	protoTarget := fmt.Sprintf("\nproto-%s:\n\t$(PROTOC) proto/%s/%s.proto\n", serviceLower, serviceLower, serviceLower)
	content = content[:protoInsert] + protoTarget + "\n" + content[protoInsert:]

	// Update all: target
	allTarget := fmt.Sprintf("all: proto-common proto-%s", serviceLower)
	content = strings.Replace(content, "all: proto-common", allTarget, 1)

	// Find insertion point for gen target
	genInsert := strings.Index(content, "# Generate all service skeletons")
	if genInsert == -1 {
		return fmt.Errorf("could not find gen insertion point in Makefile")
	}

	// Add gen target
	genTarget := fmt.Sprintf("\ngen-%s: gen-tool proto-%s\n\t$(GEN) %s %sService %d\n",
		serviceLower, serviceLower, serviceLower, serviceTitle, port)
	content = content[:genInsert] + genTarget + "\n" + content[genInsert:]

	// Update gen-all target
	genAllTarget := fmt.Sprintf("gen-all: gen-tool all gen-%s", serviceLower)
	content = strings.Replace(content, "gen-all: gen-tool all", genAllTarget, 1)

	return os.WriteFile("Makefile", []byte(content), 0644)
}
