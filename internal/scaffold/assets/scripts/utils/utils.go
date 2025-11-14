package utils

import "strings"

// ToSnakeCase converts camelCase to snake_case
func ToSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

// ToCamelCase converts snake_case to CamelCase
// Special handling: numeric parts keep underscore (e.g., percent_stage_1 -> PercentStage_1)
func ToCamelCase(s string) string {
	parts := strings.Split(s, "_")
	var result strings.Builder

	for i := range parts {
		if len(parts[i]) > 0 {
			// Check if this part is numeric (starts with a digit)
			if parts[i][0] >= '0' && parts[i][0] <= '9' {
				// Keep underscore before numeric parts
				result.WriteRune('_')
				result.WriteString(parts[i])
			} else {
				// Capitalize non-numeric parts
				result.WriteString(strings.ToUpper(parts[i][:1]) + parts[i][1:])
			}
		}
	}
	return result.String()
}

// NormalizePlural converts plural entity names to singular
// Examples: Faculties -> Faculty, Semesters -> Semester
func NormalizePlural(name string) string {
	if strings.HasSuffix(name, "ies") && len(name) > 3 {
		return name[:len(name)-3] + "y"
	}
	if strings.HasSuffix(name, "ses") && len(name) > 3 {
		return name[:len(name)-2]
	}
	if strings.HasSuffix(name, "s") && len(name) > 1 {
		return name[:len(name)-1]
	}
	return name
}

// IsCRUDEntity checks if an entity has all CRUD methods
func IsCRUDEntity(methods []interface{}) bool {
	hasCreate := false
	hasGet := false
	hasUpdate := false
	hasDelete := false
	hasList := false

	for _, m := range methods {
		if method, ok := m.(interface{ GetName() string }); ok {
			name := method.GetName()
			if strings.HasPrefix(name, "Create") {
				hasCreate = true
			} else if strings.HasPrefix(name, "Get") && !strings.HasPrefix(name, "List") {
				hasGet = true
			} else if strings.HasPrefix(name, "Update") {
				hasUpdate = true
			} else if strings.HasPrefix(name, "Delete") {
				hasDelete = true
			} else if strings.HasPrefix(name, "List") {
				hasList = true
			}
		}
	}

	return hasCreate && hasGet && hasUpdate && hasDelete && hasList
}
