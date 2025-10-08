package parser

import (
	"bufio"
	"os"
	"regexp"
	"strings"

	"gen_skeleton/types"
	"gen_skeleton/utils"
)

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

// ParseEnumsFromProto extracts enum definitions from proto file
func ParseEnumsFromProto(filename string) (map[string][]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	enums := make(map[string][]string)
	scanner := bufio.NewScanner(file)

	enumRegex := regexp.MustCompile(`enum\s+(\w+)\s*\{`)
	enumValueRegex := regexp.MustCompile(`^\s*(\w+)\s*=\s*\d+;`)

	var currentEnum string
	inEnum := false

	for scanner.Scan() {
		line := scanner.Text()

		// Check if starting an enum
		if matches := enumRegex.FindStringSubmatch(line); len(matches) == 2 {
			currentEnum = matches[1]
			inEnum = true
			enums[currentEnum] = []string{}
			continue
		}

		// Check if inside enum
		if inEnum {
			if strings.Contains(line, "}") {
				inEnum = false
				currentEnum = ""
			} else if matches := enumValueRegex.FindStringSubmatch(line); len(matches) == 2 {
				enums[currentEnum] = append(enums[currentEnum], matches[1])
			}
		}
	}

	return enums, scanner.Err()
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
	fieldRegex := regexp.MustCompile(`^\s*(optional\s+)?(\w+(?:\.\w+\.\w+)?)\s+(\w+)\s*=\s*\d+;`)

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

				// Skip system fields
				if fieldName == "created_by" || fieldName == "updated_by" {
					continue
				}

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

// ParseEntityFields extracts field information from entity messages
func ParseEntityFields(filename string, enums map[string][]string) (map[string][]types.Field, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	entityFields := make(map[string][]types.Field)
	scanner := bufio.NewScanner(file)

	messageRegex := regexp.MustCompile(`message\s+(\w+)\s*\{`)
	fieldRegex := regexp.MustCompile(`^\s*(optional\s+)?(\w+(?:\.\w+\.\w+)?)\s+(\w+)\s*=\s*\d+;`)

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

				// Skip system fields
				if fieldName == "id" || fieldName == "created_at" || fieldName == "updated_at" ||
					fieldName == "created_by" || fieldName == "updated_by" {
					continue
				}

				field := types.Field{
					Name:       fieldName,
					ProtoName:  utils.ToSnakeCase(fieldName),
					Type:       fieldType,
					GoName:     utils.ToCamelCase(fieldName),
					DBField:    utils.ToSnakeCase(fieldName),
					IsOptional: isOptional,
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
				} else if fieldType == "string" {
					field.DefaultValue = `""`
				} else if fieldType == "int32" {
					field.DefaultValue = "int32(0)"
				} else if fieldType == "int64" {
					field.DefaultValue = "int64(0)"
				} else if fieldType == "bool" {
					field.DefaultValue = "false"
				} else if strings.Contains(fieldType, "Timestamp") {
					field.IsTimestamp = true
				}

				entityFields[currentMessage] = append(entityFields[currentMessage], field)
			}
		}
	}

	return entityFields, scanner.Err()
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

// IsCRUDEntity checks if methods represent a full CRUD entity
func IsCRUDEntity(methods []types.Method) bool {
	hasCreate := false
	hasGet := false
	hasUpdate := false
	hasDelete := false
	hasList := false

	for _, m := range methods {
		if strings.HasPrefix(m.Name, "Create") {
			hasCreate = true
		} else if strings.HasPrefix(m.Name, "Get") && !strings.HasPrefix(m.Name, "List") {
			hasGet = true
		} else if strings.HasPrefix(m.Name, "Update") {
			hasUpdate = true
		} else if strings.HasPrefix(m.Name, "Delete") {
			hasDelete = true
		} else if strings.HasPrefix(m.Name, "List") {
			hasList = true
		}
	}

	return hasCreate && hasGet && hasUpdate && hasDelete && hasList
}
