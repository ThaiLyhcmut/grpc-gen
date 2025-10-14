package generator

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gen_skeleton/types"
)

// GenerateMain creates main.go from template
func GenerateMain(serviceDir string, data types.Data) {
	tmpl, err := template.ParseFiles("template/main.tmpl")
	if err != nil {
		log.Fatal(err)
	}

	out, err := os.Create(filepath.Join(serviceDir, "main.go"))
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	if err := tmpl.Execute(out, data); err != nil {
		log.Fatal(err)
	}

	log.Printf("Generated %s/main.go\n", serviceDir)
}

// GenerateHandlerRoot creates handler/handler.go from template
func GenerateHandlerRoot(handlerDir string, data types.HandlerData) {
	tmpl, err := template.ParseFiles("template/handler.tmpl")
	if err != nil {
		log.Fatal(err)
	}

	out, err := os.Create(filepath.Join(handlerDir, "handler.go"))
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	if err := tmpl.Execute(out, data); err != nil {
		log.Fatal(err)
	}

	log.Printf("Generated %s/handler.go\n", handlerDir)
}

// GenerateEntityHandler creates a simple entity handler from template
func GenerateEntityHandler(handlerDir string, data types.EntityHandlerData) {
	tmpl, err := template.ParseFiles("template/entity_handler.tmpl")
	if err != nil {
		log.Fatal(err)
	}

	filename := strings.ToLower(data.EntityName) + ".go"
	out, err := os.Create(filepath.Join(handlerDir, filename))
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	if err := tmpl.Execute(out, data); err != nil {
		log.Fatal(err)
	}

	log.Printf("Generated %s/%s\n", handlerDir, filename)
}

// GenerateCRUDHandler creates a full CRUD handler from template
func GenerateCRUDHandler(handlerDir, packagePath, entityName string, methods []types.Method, fields []types.Field, enums map[string][]string, requiredFieldsMap map[string][]string, optionalFieldsMap map[string][]string, optionalEntityFieldsMap map[string][]string, optionalUpdateFieldsMap map[string][]string, modulePath string) {
	// Prepare data for template
	requiredFields := []types.Field{}
	optionalFields := []types.Field{}
	enumFields := []types.Field{}
	createFields := []types.Field{}
	updateFields := []types.Field{}
	filterableFields := []string{}
	scanFields := []string{}

	var enumType string

	// Get required/optional field names from CreateRequest
	requiredFieldNames := make(map[string]bool)
	optionalFieldNames := make(map[string]bool)

	if reqFields, ok := requiredFieldsMap[entityName]; ok {
		for _, fieldName := range reqFields {
			requiredFieldNames[fieldName] = true
		}
	}
	if optFields, ok := optionalFieldsMap[entityName]; ok {
		for _, fieldName := range optFields {
			optionalFieldNames[fieldName] = true
		}
	}

	for i := range fields {
		field := &fields[i] // Use pointer to modify in place

		if field.IsEnum && enumType == "" {
			enumType = field.EnumType
		}

		if !field.IsTimestamp {
			filterableFields = append(filterableFields, field.DBField)
		}

		// Mark field as optional if it's in optionalFieldsMap (MUST DO THIS FIRST)
		if optionalFieldNames[field.DBField] {
			field.IsOptional = true
		}

		// Now append to category lists (AFTER marking optional)
		if field.IsEnum {
			enumFields = append(enumFields, *field)
		}

		// Check if field is required based on CreateRequest
		if requiredFieldNames[field.DBField] && !field.IsEnum {
			requiredFields = append(requiredFields, *field)
		} else if optionalFieldNames[field.DBField] && !field.IsTimestamp {
			optionalFields = append(optionalFields, *field)
		}

		// All non-timestamp fields can be updated and created
		if !field.IsTimestamp {
			updateFields = append(updateFields, *field)
			createFields = append(createFields, *field)
		}
	}

	// Build scan fields list (start with id)
	scanFields = append(scanFields, "entity.Id")
	for _, field := range fields {
		if field.IsEnum {
			scanFields = append(scanFields, field.GoName+"Str")
		} else if field.IsTimestamp {
			continue
		} else {
			scanFields = append(scanFields, "entity."+field.GoName)
		}
	}

	// Build SQL field strings
	createFieldNames := []string{}
	createPlaceholders := []string{}
	selectFields := []string{}

	for _, field := range fields {
		if !field.IsTimestamp {
			createFieldNames = append(createFieldNames, field.DBField)
			createPlaceholders = append(createPlaceholders, "?")
		}
	}

	selectFields = append([]string{"id"}, createFieldNames...)
	selectFields = append(selectFields, "created_at", "updated_at", "created_by", "updated_by")

	// Get optional entity fields
	optionalEntityFields := []string{}
	if optFields, ok := optionalEntityFieldsMap[entityName]; ok {
		optionalEntityFields = optFields
	}

	// Get optional update fields
	optionalUpdateFields := []string{}
	if optFields, ok := optionalUpdateFieldsMap[entityName]; ok {
		optionalUpdateFields = optFields
	}

	// Check if created_by and updated_by are optional
	isCreatedByOptional := false
	isUpdatedByOptional := false

	if optFields, ok := optionalFieldsMap[entityName]; ok {
		for _, field := range optFields {
			if field == "created_by" {
				isCreatedByOptional = true
			}
		}
	}

	if optFields, ok := optionalUpdateFieldsMap[entityName]; ok {
		for _, field := range optFields {
			if field == "updated_by" {
				isUpdatedByOptional = true
			}
		}
	}

	data := types.CRUDHandlerData{
		ModulePath:           modulePath,
		PackagePath:          packagePath,
		EntityName:           entityName,
		TableName:            entityName,
		Methods:              methods,
		EnumType:             enumType,
		RequiredFields:       requiredFields,
		OptionalFields:       optionalFields,
		EnumFields:           enumFields,
		FilterableFields:     filterableFields,
		CreateFields:         createFields,
		CreateFieldsSQL:      strings.Join(createFieldNames, ", "),
		CreatePlaceholders:   strings.Join(createPlaceholders, ", "),
		UpdateFields:         updateFields,
		SelectFieldsSQL:      strings.Join(selectFields, ", "),
		ScanFields:           scanFields,
		OptionalEntityFields: optionalEntityFields,
		OptionalUpdateFields: optionalUpdateFields,
		IsCreatedByOptional:  isCreatedByOptional,
		IsUpdatedByOptional:  isUpdatedByOptional,
	}

	// Create template with custom functions
	funcMap := template.FuncMap{
		"lower":     strings.ToLower,
		"hasPrefix": strings.HasPrefix,
		"pluralize": func(s string) string {
			// Pluralize keeping first letter uppercase (Go proto convention)
			if len(s) == 0 {
				return s
			}
			// Check if ends with consonant + y
			if strings.HasSuffix(s, "y") && len(s) > 1 {
				// Faculty -> Faculties
				return s[:len(s)-1] + "ies"
			}
			// Default: just add s
			return s + "s"
		},
		"isOptionalEntity": func(fieldName string, optionalFields []string) bool {
			for _, opt := range optionalFields {
				if opt == fieldName {
					return true
				}
			}
			return false
		},
		"isOptionalUpdate": func(fieldName string, optionalFields []string) bool {
			for _, opt := range optionalFields {
				if opt == fieldName {
					return true
				}
			}
			return false
		},
	}

	tmpl, err := template.New("crud_handler.tmpl").Funcs(funcMap).ParseFiles("template/crud_handler.tmpl")
	if err != nil {
		log.Fatal(err)
	}

	filename := strings.ToLower(entityName) + ".go"
	out, err := os.Create(filepath.Join(handlerDir, filename))
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	if err := tmpl.Execute(out, data); err != nil {
		log.Fatal(err)
	}

	log.Printf("Generated CRUD handler %s/%s\n", handlerDir, filename)
}

// GenerateEnvFile creates .env file from template
func GenerateEnvFile(protoName string, data types.Data) {
	tmpl, err := template.ParseFiles("template/env.tmpl")
	if err != nil {
		log.Fatal(err)
	}

	filename := filepath.Join("env", protoName+".env")
	out, err := os.Create(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	if err := tmpl.Execute(out, data); err != nil {
		log.Fatal(err)
	}

	log.Printf("Generated %s\n", filename)
}

// GenerateDockerfile creates Dockerfile from template
func GenerateDockerfile(protoName string, data types.Data) {
	tmpl, err := template.ParseFiles("template/dockerfile.tmpl")
	if err != nil {
		log.Fatal(err)
	}

	filename := filepath.Join("docker", protoName+".Dockerfile")
	out, err := os.Create(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	if err := tmpl.Execute(out, data); err != nil {
		log.Fatal(err)
	}

	log.Printf("Generated %s\n", filename)
}
