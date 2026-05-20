package parser

import (
	"bufio"
	"os"
	"regexp"
	"strings"

	"gen_skeleton/types"
	"gen_skeleton/utils"
)

// normalizeProtoType maps a proto3 scalar type to the Go type protoc emits.
// Keeps the template-side branches simple: it only needs to know Go types
// like "int32"/"float64", not every proto variant (sint32, sfixed64, ...).
func normalizeProtoType(protoType string) string {
	switch protoType {
	case "int32", "sint32", "sfixed32":
		return "int32"
	case "int64", "sint64", "sfixed64":
		return "int64"
	case "uint32", "fixed32":
		return "uint32"
	case "uint64", "fixed64":
		return "uint64"
	case "float":
		return "float32"
	case "double":
		return "float64"
	case "bytes":
		return "[]byte"
	default:
		return protoType
	}
}

// defaultValueFor returns the Go literal used to initialize a local var for an
// optional field in the Create handler before the request value is copied in.
func defaultValueFor(goType string) string {
	switch goType {
	case "string":
		return `""`
	case "int32":
		return "int32(0)"
	case "int64":
		return "int64(0)"
	case "uint32":
		return "uint32(0)"
	case "uint64":
		return "uint64(0)"
	case "float32":
		return "float32(0)"
	case "float64":
		return "float64(0)"
	case "bool":
		return "false"
	case "[]byte":
		return "[]byte(nil)"
	}
	return ""
}

// ParseProtoFile extracts RPC methods from proto file
func ParseProtoFile(filename string) ([]types.Method, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var methods []types.Method
	scanner := bufio.NewScanner(file)
	rpcRegex := regexp.MustCompile(`rpc\s+(\w+)\s*\(\s*(\w+)\s*\)\s*returns\s*\(\s*(\w+)\s*\)`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		matches := rpcRegex.FindStringSubmatch(line)
		if len(matches) == 4 {
			methods = append(methods, types.Method{
				Name:         matches[1],
				RequestType:  matches[2],
				ResponseType: matches[3],
			})
		}
	}

	return methods, scanner.Err()
}

// ParseEnumsFromProto extracts enum definitions. Works for both multi-line
// (`enum X {\n  A = 0;\n}`) and single-line (`enum X { A = 0; B = 1; }`)
// formats by parsing the whole file body with a DOTALL regex.
func ParseEnumsFromProto(filename string) (map[string][]string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	enums := make(map[string][]string)
	enumBlockRegex := regexp.MustCompile(`(?s)enum\s+(\w+)\s*\{(.*?)\}`)
	valueRegex := regexp.MustCompile(`(\w+)\s*=\s*\d+\s*;`)

	for _, m := range enumBlockRegex.FindAllStringSubmatch(string(data), -1) {
		name := m[1]
		body := m[2]
		values := []string{}
		for _, vm := range valueRegex.FindAllStringSubmatch(body, -1) {
			values = append(values, vm[1])
		}
		enums[name] = values
	}

	return enums, nil
}

// ParseFieldsFromUpdateRequests extracts all fields from UpdateXRequest messages
// Returns: optionalUpdateFields (fields marked optional), allUpdateFields (all fields in UpdateRequest)
func ParseFieldsFromUpdateRequests(filename string) (map[string][]string, map[string][]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	optionalUpdateFields := make(map[string][]string)
	allUpdateFields := make(map[string][]string)
	scanner := bufio.NewScanner(file)

	messageRegex := regexp.MustCompile(`message\s+Update(\w+)Request\s*\{`)
	fieldRegex := regexp.MustCompile(`^\s*(optional\s+)?(\w+(?:\.\w+\.\w+)?)\s+(\w+)\s*=\s*\d+\s*(?:\[.*?\])?\s*;`)

	var currentEntity string
	inMessage := false

	for scanner.Scan() {
		line := scanner.Text()

		// Check if starting an UpdateRequest message
		if matches := messageRegex.FindStringSubmatch(line); len(matches) == 2 {
			currentEntity = matches[1]
			inMessage = true
			optionalUpdateFields[currentEntity] = []string{}
			allUpdateFields[currentEntity] = []string{}
			continue
		}

		// Check if inside message
		if inMessage {
			if strings.Contains(line, "}") {
				inMessage = false
				currentEntity = ""
			} else if matches := fieldRegex.FindStringSubmatch(line); len(matches) >= 4 {
				isOptional := matches[1] != ""
				fieldName := matches[3]

				// Skip id only (not other fields)
				if fieldName == "id" {
					continue
				}

				dbFieldName := utils.ToSnakeCase(fieldName)

				// Track all fields in UpdateRequest
				allUpdateFields[currentEntity] = append(allUpdateFields[currentEntity], dbFieldName)

				// Track optional fields separately
				if isOptional {
					optionalUpdateFields[currentEntity] = append(optionalUpdateFields[currentEntity], dbFieldName)
				}
			}
		}
	}

	return optionalUpdateFields, allUpdateFields, scanner.Err()
}

// ParseFieldsFromCreateRequests extracts required and optional fields from CreateXRequest messages
func ParseFieldsFromCreateRequests(filename string) (map[string][]string, map[string][]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	requiredFields := make(map[string][]string)
	optionalFields := make(map[string][]string)
	scanner := bufio.NewScanner(file)

	messageRegex := regexp.MustCompile(`message\s+Create(\w+)Request\s*\{`)
	fieldRegex := regexp.MustCompile(`^\s*(optional\s+)?(\w+(?:\.\w+\.\w+)?)\s+(\w+)\s*=\s*\d+\s*(?:\[.*?\])?\s*;`)

	var currentEntity string
	inMessage := false

	for scanner.Scan() {
		line := scanner.Text()

		// Check if starting a CreateRequest message
		if matches := messageRegex.FindStringSubmatch(line); len(matches) == 2 {
			currentEntity = matches[1]
			inMessage = true
			requiredFields[currentEntity] = []string{}
			optionalFields[currentEntity] = []string{}
			continue
		}

		// Check if inside message
		if inMessage {
			if strings.Contains(line, "}") {
				inMessage = false
				currentEntity = ""
			} else if matches := fieldRegex.FindStringSubmatch(line); len(matches) >= 4 {
				isOptional := matches[1] != ""
				fieldName := matches[3]

				// Mark system fields as always optional
				// but don't skip them - we need to track them

				dbFieldName := utils.ToSnakeCase(fieldName)
				if isOptional {
					optionalFields[currentEntity] = append(optionalFields[currentEntity], dbFieldName)
				} else {
					requiredFields[currentEntity] = append(requiredFields[currentEntity], dbFieldName)
				}
			}
		}
	}

	return requiredFields, optionalFields, scanner.Err()
}

// ParseEntityOptionalFields extracts optional field names from entity messages
func ParseEntityOptionalFields(filename string) (map[string][]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	optionalEntityFields := make(map[string][]string)
	scanner := bufio.NewScanner(file)

	messageRegex := regexp.MustCompile(`message\s+(\w+)\s*\{`)
	fieldRegex := regexp.MustCompile(`^\s*(optional\s+)?(\w+(?:\.\w+\.\w+)?)\s+(\w+)\s*=\s*\d+\s*(?:\[.*?\])?\s*;`)

	var currentEntity string
	inMessage := false

	for scanner.Scan() {
		line := scanner.Text()

		// Check if starting an entity message (not Request/Response)
		if matches := messageRegex.FindStringSubmatch(line); len(matches) == 2 {
			msgName := matches[1]
			if !strings.HasSuffix(msgName, "Request") && !strings.HasSuffix(msgName, "Response") {
				currentEntity = msgName
				inMessage = true
				optionalEntityFields[currentEntity] = []string{}
			}
			continue
		}

		// Check if inside message
		if inMessage {
			if strings.Contains(line, "}") {
				inMessage = false
				currentEntity = ""
			} else if matches := fieldRegex.FindStringSubmatch(line); len(matches) >= 4 {
				isOptional := matches[1] != ""
				fieldName := matches[3]

				// Track optional fields (including system fields)
				if isOptional {
					dbFieldName := utils.ToSnakeCase(fieldName)
					optionalEntityFields[currentEntity] = append(optionalEntityFields[currentEntity], dbFieldName)
				}
			}
		}
	}

	return optionalEntityFields, scanner.Err()
}

// ParseEntityFields extracts field information from entity messages. Returns:
//   - entityFields: per-entity list of non-system fields (used for scan/CRUD)
//   - blockedSystemFields: per-entity set of system fields explicitly marked
//     `[(common.filterable) = false]` so the generator can subtract them from
//     the system-field filter defaults. System fields are still skipped from
//     scan logic, but their filterability is now configurable.
func ParseEntityFields(filename string, enums map[string][]string) (map[string][]types.Field, map[string]map[string]bool, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	entityFields := make(map[string][]types.Field)
	blockedSystemFields := make(map[string]map[string]bool)
	scanner := bufio.NewScanner(file)

	messageRegex := regexp.MustCompile(`message\s+(\w+)\s*\{`)
	// Field regex captures: optional?, type, name, tag, optional [annotations]
	fieldRegex := regexp.MustCompile(`^\s*(optional\s+)?(\w+(?:\.\w+\.\w+)?)\s+(\w+)\s*=\s*\d+\s*(?:\[(.*?)\])?\s*;`)
	// Matches (common.filterable) = true|false inside the annotation block.
	filterableAnnotRegex := regexp.MustCompile(`\(\s*common\.filterable\s*\)\s*=\s*(true|false)`)

	systemFieldNames := map[string]bool{
		"id": true, "created_at": true, "updated_at": true,
		"created_by": true, "updated_by": true,
	}

	var currentMessage string
	inMessage := false

	for scanner.Scan() {
		line := scanner.Text()

		// Check if starting a message (entity)
		if matches := messageRegex.FindStringSubmatch(line); len(matches) == 2 {
			msgName := matches[1]
			// Only track entity messages (not Request/Response)
			if !strings.HasSuffix(msgName, "Request") && !strings.HasSuffix(msgName, "Response") {
				currentMessage = msgName
				inMessage = true
				entityFields[currentMessage] = []types.Field{}
				blockedSystemFields[currentMessage] = map[string]bool{}
			}
			continue
		}

		// Check if inside message
		if inMessage {
			if strings.Contains(line, "}") {
				inMessage = false
				currentMessage = ""
			} else if matches := fieldRegex.FindStringSubmatch(line); len(matches) >= 4 {
				isOptional := matches[1] != ""
				fieldType := matches[2]
				fieldName := matches[3]
				annotations := ""
				if len(matches) >= 5 {
					annotations = matches[4]
				}

				// System fields are skipped from regular field iteration (scan logic
				// handles them separately) BUT we still parse the annotation so the
				// generator can know if the user wants to block them from filter.
				if systemFieldNames[fieldName] {
					if m := filterableAnnotRegex.FindStringSubmatch(annotations); len(m) == 2 && m[1] == "false" {
						blockedSystemFields[currentMessage][fieldName] = true
					}
					continue
				}

				// Default allow: every field is filterable unless explicitly marked
				// with [(common.filterable) = false] in the proto. This means you
				// only need to annotate sensitive fields (password, secret, ...).
				isFilterable := true
				if m := filterableAnnotRegex.FindStringSubmatch(annotations); len(m) == 2 {
					isFilterable = m[1] == "true"
				}

				field := types.Field{
					Name:         fieldName,
					ProtoName:    utils.ToSnakeCase(fieldName),
					Type:         normalizeProtoType(fieldType),
					GoName:       utils.ToCamelCase(fieldName),
					DBField:      utils.ToSnakeCase(fieldName),
					IsOptional:   isOptional,
					IsFilterable: isFilterable,
				}

				// Check if enum
				if _, isEnum := enums[fieldType]; isEnum {
					field.IsEnum = true
					field.EnumType = fieldType
					field.EnumValues = enums[fieldType]
					if len(field.EnumValues) > 0 {
						field.DefaultValue = field.EnumValues[0]
						field.DefaultDBValue = strings.ToLower(field.EnumValues[0])
					}
				} else if strings.Contains(fieldType, "Timestamp") {
					field.IsTimestamp = true
					// No DefaultValue: the template has a dedicated IsTimestamp branch
					// for Create/Update that uses interface{} + AsTime() instead.
				} else {
					field.DefaultValue = defaultValueFor(field.Type)
				}

				entityFields[currentMessage] = append(entityFields[currentMessage], field)
			}
		}
	}

	return entityFields, blockedSystemFields, scanner.Err()
}

// GroupMethodsByEntity groups RPC methods by their entity name
func GroupMethodsByEntity(methods []types.Method) map[string][]types.Method {
	entityMethods := make(map[string][]types.Method)

	// First pass: get entity names from Create methods (canonical names)
	entityNames := make(map[string]string) // normalized -> canonical
	for _, method := range methods {
		if strings.HasPrefix(method.Name, "Create") {
			canonicalName := strings.TrimPrefix(method.Name, "Create")
			canonicalName = strings.TrimSuffix(canonicalName, "Request")

			// Normalize for matching
			normalized := utils.NormalizePlural(canonicalName)
			entityNames[normalized] = canonicalName
		}
	}

	// Second pass: group methods by entity using canonical names
	for _, method := range methods {
		// Extract entity name from method name
		var entityName string
		for _, prefix := range []string{"Create", "Get", "Update", "Delete", "List"} {
			if strings.HasPrefix(method.Name, prefix) {
				entityName = strings.TrimPrefix(method.Name, prefix)
				break
			}
		}

		// Normalize to find canonical name
		normalized := utils.NormalizePlural(entityName)

		// Use canonical name if found, otherwise use normalized
		canonicalName := entityNames[normalized]
		if canonicalName == "" {
			canonicalName = normalized
		}

		if canonicalName != "" {
			entityMethods[canonicalName] = append(entityMethods[canonicalName], method)
		}
	}

	return entityMethods
}

// IsCRUDEntity checks if methods represent a full CRUD entity.
// Get is no longer required: detail-by-id is just List with an id filter,
// and Create inlines its read-back SELECT instead of calling Get.
func IsCRUDEntity(methods []types.Method) bool {
	hasCreate := false
	hasUpdate := false
	hasDelete := false
	hasList := false

	for _, m := range methods {
		if strings.HasPrefix(m.Name, "Create") {
			hasCreate = true
		} else if strings.HasPrefix(m.Name, "Update") {
			hasUpdate = true
		} else if strings.HasPrefix(m.Name, "Delete") {
			hasDelete = true
		} else if strings.HasPrefix(m.Name, "List") {
			hasList = true
		}
	}

	return hasCreate && hasUpdate && hasDelete && hasList
}
