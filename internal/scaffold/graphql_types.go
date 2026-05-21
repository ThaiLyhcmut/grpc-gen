package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// generateGraphQLTypes generates types.graphqls from proto files
func generateGraphQLTypes(serverDir string, protos []ProtoInfo) error {
	var sb strings.Builder

	sb.WriteString("# Auto-generated GraphQL types from proto files\n")
	sb.WriteString("# DO NOT EDIT - regenerate with 'grpc-gen add-server'\n\n")

	// Add scalar types
	sb.WriteString("scalar Time\n")
	sb.WriteString("scalar Upload\n\n")

	// Generate enums
	for _, proto := range protos {
		if len(proto.Enums) > 0 {
			sb.WriteString(fmt.Sprintf("# ============= %s Enums =============\n", strings.Title(proto.PackageName)))
			for _, enum := range proto.Enums {
				sb.WriteString(fmt.Sprintf("enum %s {\n", enum.Name))
				for _, value := range enum.Values {
					sb.WriteString(fmt.Sprintf("  %s\n", value))
				}
				sb.WriteString("}\n\n")
			}
		}
	}

	// Generate types
	for _, proto := range protos {
		if len(proto.Messages) > 0 {
			sb.WriteString(fmt.Sprintf("# ============= %s Types =============\n", strings.Title(proto.PackageName)))
			for _, msg := range proto.Messages {
				sb.WriteString(fmt.Sprintf("type %s {\n", msg.Name))
				for _, field := range msg.Fields {
					graphqlType := protoTypeToGraphQL(field.Type, field.IsOptional, field.IsRepeated)
					graphqlName := toGraphQLFieldName(field.Name)
					sb.WriteString(fmt.Sprintf("  %s: %s\n", graphqlName, graphqlType))
				}
				sb.WriteString("}\n\n")
			}
		}
	}

	// Generate input types for mutations
	for _, proto := range protos {
		if len(proto.Messages) > 0 {
			sb.WriteString(fmt.Sprintf("# ============= %s Inputs =============\n", strings.Title(proto.PackageName)))
			for _, msg := range proto.Messages {
				// Create input type
				sb.WriteString(fmt.Sprintf("input Create%sInput {\n", msg.Name))
				for _, field := range msg.Fields {
					// Skip id, created_at, updated_at, created_by, updated_by
					if isAutoField(field.Name) {
						continue
					}
					graphqlType := protoTypeToGraphQL(field.Type, true, field.IsRepeated)
					graphqlName := toGraphQLFieldName(field.Name)
					sb.WriteString(fmt.Sprintf("  %s: %s\n", graphqlName, graphqlType))
				}
				sb.WriteString("}\n\n")

				// Update input type
				sb.WriteString(fmt.Sprintf("input Update%sInput {\n", msg.Name))
				sb.WriteString("  id: ID!\n")
				for _, field := range msg.Fields {
					// Skip auto fields
					if isAutoField(field.Name) {
						continue
					}
					graphqlType := protoTypeToGraphQL(field.Type, true, field.IsRepeated)
					graphqlName := toGraphQLFieldName(field.Name)
					sb.WriteString(fmt.Sprintf("  %s: %s\n", graphqlName, graphqlType))
				}
				sb.WriteString("}\n\n")
			}
		}
	}

	// Generate list response types
	for _, proto := range protos {
		for _, msg := range proto.Messages {
			sb.WriteString(fmt.Sprintf("type %sListResponse {\n", msg.Name))
			sb.WriteString(fmt.Sprintf("  %s: [%s!]!\n", toLowerFirst(msg.Name)+"s", msg.Name))
			sb.WriteString("  total: Int!\n")
			sb.WriteString("  page: Int!\n")
			sb.WriteString("  pageSize: Int!\n")
			sb.WriteString("}\n\n")
		}
	}

	// Generate common input types
	sb.WriteString("# ============= Common Inputs =============\n")
	sb.WriteString(`input PaginationInput {
  page: Int = 1
  pageSize: Int = 20
  sortBy: String
  descending: Boolean = false
}

input FilterConditionInput {
  field: String!
  operator: FilterOperator!
  values: [String!]
}

input FilterGroupInput {
  logic: LogicalCondition = AND
  filters: [FilterCriteriaInput!]!
}

input FilterCriteriaInput {
  condition: FilterConditionInput
  group: FilterGroupInput
}

input SearchRequestInput {
  pagination: PaginationInput
  filters: [FilterCriteriaInput!]
}

enum FilterOperator {
  EQUAL
  NOT_EQUAL
  GREATER_THAN
  GREATER_THAN_EQUAL
  LESS_THAN
  LESS_THAN_EQUAL
  LIKE
  IN
  NOT_IN
  IS_NULL
  IS_NOT_NULL
  BETWEEN
}

enum LogicalCondition {
  AND
  OR
}
`)

	// Write to file
	typesFile := filepath.Join(serverDir, "graph", "schema", "types.graphqls")
	return os.WriteFile(typesFile, []byte(sb.String()), 0644)
}

// generateRootSchema generates schema.graphqls
func generateRootSchema(serverDir string) error {
	content := `# Root GraphQL Schema
# This file defines the root Query and Mutation types

type Query

type Mutation

# Uncomment to enable subscriptions
# type Subscription
`
	schemaFile := filepath.Join(serverDir, "graph", "schema", "schema.graphqls")
	return os.WriteFile(schemaFile, []byte(content), 0644)
}

// generateQuerySchema generates query.graphqls template
func generateQuerySchema(serverDir string, protos []ProtoInfo) error {
	var sb strings.Builder

	sb.WriteString("# Query definitions\n")
	sb.WriteString("# Add your query fields here\n\n")
	sb.WriteString("extend type Query {\n")

	for _, proto := range protos {
		sb.WriteString(fmt.Sprintf("  # ============= %s Queries =============\n", strings.Title(proto.PackageName)))
		for _, msg := range proto.Messages {
			lowerName := toLowerFirst(msg.Name)
			// Get by ID
			sb.WriteString(fmt.Sprintf("  %s(id: ID!): %s\n", lowerName, msg.Name))
			// List with search
			sb.WriteString(fmt.Sprintf("  %ss(search: SearchRequestInput): %sListResponse!\n", lowerName, msg.Name))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("}\n")

	queryFile := filepath.Join(serverDir, "graph", "schema", "query.graphqls")
	return os.WriteFile(queryFile, []byte(sb.String()), 0644)
}

// generateMutationSchema generates mutation.graphqls template
func generateMutationSchema(serverDir string, protos []ProtoInfo) error {
	var sb strings.Builder

	sb.WriteString("# Mutation definitions\n")
	sb.WriteString("# Add your mutation fields here\n\n")
	sb.WriteString("extend type Mutation {\n")

	for _, proto := range protos {
		sb.WriteString(fmt.Sprintf("  # ============= %s Mutations =============\n", strings.Title(proto.PackageName)))
		for _, msg := range proto.Messages {
			lowerName := toLowerFirst(msg.Name)
			// Create
			sb.WriteString(fmt.Sprintf("  create%s(input: Create%sInput!): %s!\n", msg.Name, msg.Name, msg.Name))
			// Update
			sb.WriteString(fmt.Sprintf("  update%s(input: Update%sInput!): %s!\n", msg.Name, msg.Name, msg.Name))
			// Delete
			sb.WriteString(fmt.Sprintf("  delete%s(id: ID!): Boolean!\n", msg.Name))
			_ = lowerName // suppress unused warning
		}
		sb.WriteString("\n")
	}

	sb.WriteString("}\n")

	mutationFile := filepath.Join(serverDir, "graph", "schema", "mutation.graphqls")
	return os.WriteFile(mutationFile, []byte(sb.String()), 0644)
}

// protoTypeToGraphQL converts proto type to GraphQL type
func protoTypeToGraphQL(protoType string, isOptional, isRepeated bool) string {
	var graphqlType string

	switch protoType {
	case "string":
		graphqlType = "String"
	case "int32", "int64", "uint32", "uint64", "sint32", "sint64":
		graphqlType = "Int"
	case "float", "double":
		graphqlType = "Float"
	case "bool":
		graphqlType = "Boolean"
	case "google.protobuf.Timestamp":
		graphqlType = "Time"
	case "bytes":
		graphqlType = "String"
	default:
		// Could be an enum or custom type
		if strings.Contains(protoType, ".") {
			parts := strings.Split(protoType, ".")
			graphqlType = parts[len(parts)-1]
		} else {
			graphqlType = protoType
		}
	}

	// Handle id field specially
	if strings.ToLower(protoType) == "string" && strings.Contains(strings.ToLower(protoType), "id") {
		graphqlType = "ID"
	}

	if isRepeated {
		graphqlType = "[" + graphqlType + "!]"
	}

	if !isOptional {
		graphqlType = graphqlType + "!"
	}

	return graphqlType
}

// toGraphQLFieldName converts proto field name to GraphQL field name (camelCase)
func toGraphQLFieldName(name string) string {
	// Convert snake_case to camelCase
	parts := strings.Split(name, "_")
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.Title(parts[i])
	}
	result := strings.Join(parts, "")

	// Special case for id
	if result == "id" {
		return "id"
	}

	return result
}

// isAutoField checks if field should be auto-generated
func isAutoField(name string) bool {
	autoFields := []string{"id", "created_at", "updated_at", "created_by", "updated_by"}
	for _, f := range autoFields {
		if name == f {
			return true
		}
	}
	return false
}

// toLowerFirst converts first character to lowercase
func toLowerFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
