package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// generateConvertFunctions generates convert functions for each service
func generateConvertFunctions(serverDir string, protos []ProtoInfo, modulePath string) error {
	for _, proto := range protos {
		if len(proto.Messages) == 0 {
			continue
		}

		if err := generateConvertFile(serverDir, proto, modulePath); err != nil {
			return err
		}
	}
	return nil
}

// generateConvertFile generates a single convert file
func generateConvertFile(serverDir string, proto ProtoInfo, modulePath string) error {
	var sb strings.Builder

	packageName := proto.PackageName
	protoImportPath := fmt.Sprintf("%s/proto/%s", modulePath, packageName)
	modelImportPath := fmt.Sprintf("%s/src/server/graph/model", modulePath)

	sb.WriteString("package convert\n\n")
	sb.WriteString("import (\n")
	sb.WriteString(fmt.Sprintf("\tpb \"%s\"\n", protoImportPath))
	sb.WriteString(fmt.Sprintf("\t\"%s\"\n", modelImportPath))
	sb.WriteString(")\n\n")

	// Generate convert functions for each message
	for _, msg := range proto.Messages {
		// pb -> model
		generatePbToModel(&sb, msg, packageName)

		// model -> pb (for input types)
		generateModelToPb(&sb, msg, packageName)

		// Slice conversion
		generateSliceConversions(&sb, msg)
	}

	// Generate enum conversions
	for _, enum := range proto.Enums {
		generateEnumConversions(&sb, enum, packageName)
	}

	// Write to file
	convertFile := filepath.Join(serverDir, "graph", "convert", packageName+".go")
	return os.WriteFile(convertFile, []byte(sb.String()), 0644)
}

// generatePbToModel generates function to convert proto message to GraphQL model
func generatePbToModel(sb *strings.Builder, msg MessageInfo, packageName string) {
	msgName := msg.Name
	funcName := fmt.Sprintf("Pb%sToModel", msgName)

	sb.WriteString(fmt.Sprintf("// %s converts proto %s to GraphQL model\n", funcName, msgName))
	sb.WriteString(fmt.Sprintf("func %s(pb *pb.%s) *model.%s {\n", funcName, msgName, msgName))
	sb.WriteString("\tif pb == nil {\n")
	sb.WriteString("\t\treturn nil\n")
	sb.WriteString("\t}\n\n")
	sb.WriteString(fmt.Sprintf("\tresult := &model.%s{\n", msgName))

	for _, field := range msg.Fields {
		modelFieldName := toGoFieldName(field.Name)
		pbFieldName := toGoFieldName(field.Name)

		if field.Type == "google.protobuf.Timestamp" {
			// Handle timestamp
			if field.IsOptional {
				sb.WriteString(fmt.Sprintf("\t\t%s: pbTimestampToTime(pb.%s),\n", modelFieldName, pbFieldName))
			} else {
				sb.WriteString(fmt.Sprintf("\t\t%s: pbTimestampToTime(pb.%s),\n", modelFieldName, pbFieldName))
			}
		} else if isEnumType(field.Type) {
			// Handle enum
			enumName := field.Type
			if field.IsOptional {
				sb.WriteString(fmt.Sprintf("\t\t%s: pb%sToModel(pb.%s),\n", modelFieldName, enumName, pbFieldName))
			} else {
				sb.WriteString(fmt.Sprintf("\t\t%s: pb%sToModel(pb.%s),\n", modelFieldName, enumName, pbFieldName))
			}
		} else if field.IsOptional {
			// Handle optional fields
			switch field.Type {
			case "string":
				sb.WriteString(fmt.Sprintf("\t\t%s: pb.%s,\n", modelFieldName, pbFieldName))
			case "int32", "int64":
				sb.WriteString(fmt.Sprintf("\t\t%s: intPtr(int(pb.Get%s())),\n", modelFieldName, pbFieldName))
			case "bool":
				sb.WriteString(fmt.Sprintf("\t\t%s: pb.%s,\n", modelFieldName, pbFieldName))
			default:
				sb.WriteString(fmt.Sprintf("\t\t%s: pb.%s,\n", modelFieldName, pbFieldName))
			}
		} else {
			// Handle required fields
			sb.WriteString(fmt.Sprintf("\t\t%s: pb.%s,\n", modelFieldName, pbFieldName))
		}
	}

	sb.WriteString("\t}\n")
	sb.WriteString("\treturn result\n")
	sb.WriteString("}\n\n")
}

// generateModelToPb generates function to convert GraphQL model to proto message
func generateModelToPb(sb *strings.Builder, msg MessageInfo, packageName string) {
	msgName := msg.Name
	funcName := fmt.Sprintf("ModelTo Pb%s", msgName)

	// For Create input
	sb.WriteString(fmt.Sprintf("// Create%sInputToPb converts GraphQL Create%sInput to proto request\n", msgName, msgName))
	sb.WriteString(fmt.Sprintf("func Create%sInputToPb(input model.Create%sInput, createdBy string) *pb.Create%sRequest {\n", msgName, msgName, msgName))
	sb.WriteString(fmt.Sprintf("\treq := &pb.Create%sRequest{\n", msgName))
	sb.WriteString("\t\tCreatedBy: createdBy,\n")

	for _, field := range msg.Fields {
		if isAutoField(field.Name) {
			continue
		}
		modelFieldName := toGoFieldName(field.Name)
		pbFieldName := toGoFieldName(field.Name)

		if isEnumType(field.Type) {
			sb.WriteString(fmt.Sprintf("\t\t%s: model%sEnumToPb(input.%s),\n", pbFieldName, field.Type, modelFieldName))
		} else if field.IsOptional {
			sb.WriteString(fmt.Sprintf("\t\t%s: input.%s,\n", pbFieldName, modelFieldName))
		} else {
			sb.WriteString(fmt.Sprintf("\t\t%s: input.%s,\n", pbFieldName, modelFieldName))
		}
	}

	sb.WriteString("\t}\n")
	sb.WriteString("\treturn req\n")
	sb.WriteString("}\n\n")

	// For Update input
	sb.WriteString(fmt.Sprintf("// Update%sInputToPb converts GraphQL Update%sInput to proto request\n", msgName, msgName))
	sb.WriteString(fmt.Sprintf("func Update%sInputToPb(input model.Update%sInput, updatedBy string) *pb.Update%sRequest {\n", msgName, msgName, msgName))
	sb.WriteString(fmt.Sprintf("\treq := &pb.Update%sRequest{\n", msgName))
	sb.WriteString("\t\tId:        input.ID,\n")
	sb.WriteString("\t\tUpdatedBy: updatedBy,\n")

	for _, field := range msg.Fields {
		if isAutoField(field.Name) {
			continue
		}
		modelFieldName := toGoFieldName(field.Name)
		pbFieldName := toGoFieldName(field.Name)

		if isEnumType(field.Type) {
			sb.WriteString(fmt.Sprintf("\t\t%s: model%sEnumToPbOptional(input.%s),\n", pbFieldName, field.Type, modelFieldName))
		} else {
			sb.WriteString(fmt.Sprintf("\t\t%s: input.%s,\n", pbFieldName, modelFieldName))
		}
	}

	sb.WriteString("\t}\n")
	sb.WriteString("\treturn req\n")
	sb.WriteString("}\n\n")

	_ = funcName // suppress unused warning
}

// generateSliceConversions generates slice conversion functions
func generateSliceConversions(sb *strings.Builder, msg MessageInfo) {
	msgName := msg.Name

	sb.WriteString(fmt.Sprintf("// Pb%sSliceToModel converts slice of proto %s to GraphQL models\n", msgName, msgName))
	sb.WriteString(fmt.Sprintf("func Pb%sSliceToModel(pbs []*pb.%s) []*model.%s {\n", msgName, msgName, msgName))
	sb.WriteString("\tif pbs == nil {\n")
	sb.WriteString("\t\treturn nil\n")
	sb.WriteString("\t}\n")
	sb.WriteString(fmt.Sprintf("\tresult := make([]*model.%s, len(pbs))\n", msgName))
	sb.WriteString("\tfor i, p := range pbs {\n")
	sb.WriteString(fmt.Sprintf("\t\tresult[i] = Pb%sToModel(p)\n", msgName))
	sb.WriteString("\t}\n")
	sb.WriteString("\treturn result\n")
	sb.WriteString("}\n\n")
}

// generateEnumConversions generates enum conversion functions
func generateEnumConversions(sb *strings.Builder, enum EnumInfo, packageName string) {
	enumName := enum.Name

	// pb enum -> model enum
	sb.WriteString(fmt.Sprintf("// pb%sToModel converts proto %s to GraphQL model enum\n", enumName, enumName))
	sb.WriteString(fmt.Sprintf("func pb%sToModel(pb pb.%s) model.%s {\n", enumName, enumName, enumName))
	sb.WriteString("\tswitch pb {\n")
	for _, value := range enum.Values {
		sb.WriteString(fmt.Sprintf("\tcase pb.%s_%s:\n", enumName, value))
		sb.WriteString(fmt.Sprintf("\t\treturn model.%s%s\n", enumName, toPascalCase(value)))
	}
	sb.WriteString("\tdefault:\n")
	if len(enum.Values) > 0 {
		sb.WriteString(fmt.Sprintf("\t\treturn model.%s%s\n", enumName, toPascalCase(enum.Values[0])))
	} else {
		sb.WriteString("\t\treturn \"\"\n")
	}
	sb.WriteString("\t}\n")
	sb.WriteString("}\n\n")

	// model enum -> pb enum
	sb.WriteString(fmt.Sprintf("// model%sEnumToPb converts GraphQL model enum to proto %s\n", enumName, enumName))
	sb.WriteString(fmt.Sprintf("func model%sEnumToPb(m model.%s) pb.%s {\n", enumName, enumName, enumName))
	sb.WriteString("\tswitch m {\n")
	for _, value := range enum.Values {
		sb.WriteString(fmt.Sprintf("\tcase model.%s%s:\n", enumName, toPascalCase(value)))
		sb.WriteString(fmt.Sprintf("\t\treturn pb.%s_%s\n", enumName, value))
	}
	sb.WriteString("\tdefault:\n")
	if len(enum.Values) > 0 {
		sb.WriteString(fmt.Sprintf("\t\treturn pb.%s_%s\n", enumName, enum.Values[0]))
	} else {
		sb.WriteString("\t\treturn 0\n")
	}
	sb.WriteString("\t}\n")
	sb.WriteString("}\n\n")

	// Optional version for update
	sb.WriteString(fmt.Sprintf("// model%sEnumToPbOptional converts optional GraphQL model enum to proto %s pointer\n", enumName, enumName))
	sb.WriteString(fmt.Sprintf("func model%sEnumToPbOptional(m *model.%s) *pb.%s {\n", enumName, enumName, enumName))
	sb.WriteString("\tif m == nil {\n")
	sb.WriteString("\t\treturn nil\n")
	sb.WriteString("\t}\n")
	sb.WriteString(fmt.Sprintf("\tresult := model%sEnumToPb(*m)\n", enumName))
	sb.WriteString("\treturn &result\n")
	sb.WriteString("}\n\n")
}

// Helper functions

// toGoFieldName converts snake_case to PascalCase
func toGoFieldName(name string) string {
	parts := strings.Split(name, "_")
	for i := range parts {
		parts[i] = strings.Title(parts[i])
	}
	return strings.Join(parts, "")
}

// toPascalCase converts UPPER_SNAKE to PascalCase
func toPascalCase(name string) string {
	parts := strings.Split(strings.ToLower(name), "_")
	for i := range parts {
		parts[i] = strings.Title(parts[i])
	}
	return strings.Join(parts, "")
}

// isEnumType checks if type is likely an enum
func isEnumType(typeName string) bool {
	// Enums typically have names ending in Status, Type, Kind, etc.
	// or don't start with lowercase (not primitive types)
	if strings.HasSuffix(typeName, "Status") ||
		strings.HasSuffix(typeName, "Type") ||
		strings.HasSuffix(typeName, "Kind") ||
		strings.HasSuffix(typeName, "Stage") ||
		strings.HasSuffix(typeName, "Position") {
		return true
	}
	// Check if first letter is uppercase (not primitive)
	if len(typeName) > 0 && typeName[0] >= 'A' && typeName[0] <= 'Z' {
		primitives := []string{"String", "Int", "Float", "Bool", "Bytes"}
		for _, p := range primitives {
			if typeName == p {
				return false
			}
		}
		return true
	}
	return false
}
