package generator

import (
	"bytes"
	"go/format"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gen_skeleton/types"
)

// renderAndWriteGo executes the given template into a buffer, runs gofmt on the
// result, and writes it to outPath. Falls back to unformatted output if gofmt
// fails (so a template typo surfaces as a useful Go compile error rather than
// being masked by go/format's complaint).
func renderAndWriteGo(tmpl *template.Template, data interface{}, outPath string) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		log.Fatal(err)
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		log.Printf("gofmt failed on %s (writing unformatted): %v", outPath, err)
		formatted = buf.Bytes()
	}
	if err := os.WriteFile(outPath, formatted, 0644); err != nil {
		log.Fatal(err)
	}
}

// GenerateMain creates main.go from template
func GenerateMain(serviceDir string, data types.Data) {
	tmpl, err := template.ParseFiles("template/main.tmpl")
	if err != nil {
		log.Fatal(err)
	}
	outPath := filepath.Join(serviceDir, "main.go")
	renderAndWriteGo(tmpl, data, outPath)
	log.Printf("Generated %s\n", outPath)
}

// GenerateHandlerRoot creates handler/handler.go from template
func GenerateHandlerRoot(handlerDir string, data types.HandlerData) {
	tmpl, err := template.ParseFiles("template/handler.tmpl")
	if err != nil {
		log.Fatal(err)
	}
	outPath := filepath.Join(handlerDir, "handler.go")
	renderAndWriteGo(tmpl, data, outPath)
	log.Printf("Generated %s\n", outPath)
}

// GenerateFilterableFile writes the per-entity filterable whitelist file derived
// from `[(common.filterable) = true]` annotations in the proto. Always
// overwritten on regen — the proto is the source of truth, not this file.
func GenerateFilterableFile(handlerDir, entityName string, filterableFields []string) {
	outPath := filepath.Join(handlerDir, strings.ToLower(entityName)+"_filterable.go")
	tmpl, err := template.New("filterable.tmpl").Funcs(template.FuncMap{
		"lower": strings.ToLower,
	}).ParseFiles("template/filterable.tmpl")
	if err != nil {
		log.Fatal(err)
	}
	data := struct {
		EntityName       string
		FilterableFields []string
	}{
		EntityName:       entityName,
		FilterableFields: filterableFields,
	}
	renderAndWriteGo(tmpl, data, outPath)
	log.Printf("Generated %s\n", outPath)
}

// GenerateEntityHandler creates a simple entity handler from template
func GenerateEntityHandler(handlerDir string, data types.EntityHandlerData) {
	tmpl, err := template.ParseFiles("template/entity_handler.tmpl")
	if err != nil {
		log.Fatal(err)
	}
	outPath := filepath.Join(handlerDir, strings.ToLower(data.EntityName)+".go")
	renderAndWriteGo(tmpl, data, outPath)
	log.Printf("Generated %s\n", outPath)
}

// GenerateCRUDHandler creates a full CRUD handler from template
func GenerateCRUDHandler(handlerDir, packagePath, entityName string, methods []types.Method, fields []types.Field, enums map[string][]string, requiredFieldsMap map[string][]string, optionalFieldsMap map[string][]string, optionalEntityFieldsMap map[string][]string, optionalUpdateFieldsMap map[string][]string, allUpdateFieldsMap map[string][]string, blockedSystemFields map[string]bool, modulePath string) {
	// Prepare data for template
	requiredFields := []types.Field{}
	optionalFields := []types.Field{}
	enumFields := []types.Field{}
	createFields := []types.Field{}
	updateFields := []types.Field{}
	// System fields are filterable by default (parser handles their scan
	// specially but client should still be able to filter by id, created_at,
	// etc.). User can block individually via `[(common.filterable) = false]`
	// in the proto — that gets surfaced via blockedSystemFields.
	systemFields := []string{"id", "created_at", "updated_at", "created_by", "updated_by"}
	filterableFields := []string{}
	for _, sf := range systemFields {
		if !blockedSystemFields[sf] {
			filterableFields = append(filterableFields, sf)
		}
	}
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

	// Get all fields from UpdateRequest
	allUpdateFieldNames := make(map[string]bool)
	if updateFieldNames, ok := allUpdateFieldsMap[entityName]; ok {
		for _, fieldName := range updateFieldNames {
			allUpdateFieldNames[fieldName] = true
		}
	}

	for i := range fields {
		field := &fields[i] // Use pointer to modify in place

		if field.IsEnum && enumType == "" {
			enumType = field.EnumType
		}

		// Default deny: only fields explicitly annotated with
		// `[(common.filterable) = true]` in the proto are added to the whitelist.
		if field.IsFilterable {
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
		} else if optionalFieldNames[field.DBField] {
			// Include timestamp fields in optionalFields too
			optionalFields = append(optionalFields, *field)
		}

		// Only add to updateFields if field exists in UpdateRequest
		if allUpdateFieldNames[field.DBField] {
			updateFields = append(updateFields, *field)
		}

		// All fields can be created (including custom timestamps like due_date)
		createFields = append(createFields, *field)
	}

	// Get optional entity fields
	optionalEntityFields := []string{}
	if optFields, ok := optionalEntityFieldsMap[entityName]; ok {
		optionalEntityFields = optFields
	}

	// Create a map for quick lookup of optional entity fields
	optionalEntityFieldsSet := make(map[string]bool)
	for _, fieldName := range optionalEntityFields {
		optionalEntityFieldsSet[fieldName] = true
	}

	// Build optional entity fields data for template
	optionalEntityFieldsData := []types.Field{}

	// Build scan fields list (start with id)
	scanFields = append(scanFields, "entity.Id")
	timestampFields := []types.Field{}
	for _, field := range fields {
		if field.IsEnum {
			scanFields = append(scanFields, field.GoName+"Str")
		} else if field.IsTimestamp {
			// Custom timestamp fields need to be scanned into sql.NullTime variable
			scanFields = append(scanFields, field.GoName+"Time")
			timestampFields = append(timestampFields, field)
		} else if field.Type == "[]byte" {
			// bytes: scan directly into the entity field. Go's []byte is
			// already a reference type; nil represents NULL — no NullXxx needed.
			scanFields = append(scanFields, "entity."+field.GoName)
		} else {
			// Check if this field is optional in the entity
			if optionalEntityFieldsSet[field.DBField] {
				// Use NullString/NullInt32 variable for optional entity fields
				scanFields = append(scanFields, field.GoName+"Null")
				optionalEntityFieldsData = append(optionalEntityFieldsData, field)
			} else {
				scanFields = append(scanFields, "entity."+field.GoName)
			}
		}
	}

	// Build SQL field strings
	createFieldNames := []string{}
	createPlaceholders := []string{}
	selectFields := []string{}

	for _, field := range fields {
		// Include all fields (including custom timestamps like due_date)
		createFieldNames = append(createFieldNames, field.DBField)
		createPlaceholders = append(createPlaceholders, "?")
	}

	selectFields = append([]string{"id"}, createFieldNames...)
	selectFields = append(selectFields, "created_at", "updated_at", "created_by", "updated_by")

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
		TableName:            strings.ToLower(entityName),
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
		SelectFieldsSQL:         strings.Join(selectFields, ", "),
		ScanFields:              scanFields,
		OptionalEntityFields:    optionalEntityFields,
		OptionalEntityFieldsData: optionalEntityFieldsData,
		OptionalUpdateFields:    optionalUpdateFields,
		TimestampFields:         timestampFields,
		IsCreatedByOptional:  isCreatedByOptional,
		IsUpdatedByOptional:  isUpdatedByOptional,
	}

	// Create template with custom functions
	funcMap := template.FuncMap{
		"lower":     strings.ToLower,
		"hasPrefix": strings.HasPrefix,
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
	outPath := filepath.Join(handlerDir, strings.ToLower(entityName)+".go")
	renderAndWriteGo(tmpl, data, outPath)
	log.Printf("Generated CRUD handler %s\n", outPath)

	// Scaffold the per-entity filterable whitelist file (once).
	GenerateFilterableFile(handlerDir, entityName, filterableFields)
}

// GenerateEnvFile creates .env file from template. Skips if the file already
// exists so user-edited DB credentials are preserved across regenerations.
func GenerateEnvFile(protoName string, data types.Data) {
	filename := filepath.Join("env", protoName+".env")
	if _, err := os.Stat(filename); err == nil {
		log.Printf("Skipped %s (already exists)\n", filename)
		return
	}

	tmpl, err := template.ParseFiles("template/env.tmpl")
	if err != nil {
		log.Fatal(err)
	}
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

	filename := filepath.Join("src", "service", protoName, "Dockerfile")
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

// GenerateDockerCompose creates docker-compose.yml from template
func GenerateDockerCompose(protoName string, data types.Data) {
	tmpl, err := template.ParseFiles("template/docker-compose.tmpl")
	if err != nil {
		log.Fatal(err)
	}

	filename := filepath.Join("src", "service", protoName, "docker-compose.yml")
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

// GenerateGitignore creates .gitignore from template
func GenerateGitignore(protoName string) {
	content := `log/
*.log
`
	filename := filepath.Join("src", "service", protoName, ".gitignore")
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		log.Fatal(err)
	}

	log.Printf("Generated %s\n", filename)
}

// GenerateServiceEnvFile creates service-level .env file. Skips if exists
// (preserves user-edited DB credentials).
func GenerateServiceEnvFile(protoName string, data types.Data) {
	filename := filepath.Join("src", "service", protoName, protoName+".env")
	if _, err := os.Stat(filename); err == nil {
		log.Printf("Skipped %s (already exists)\n", filename)
		return
	}

	tmpl, err := template.ParseFiles("template/env.tmpl")
	if err != nil {
		log.Fatal(err)
	}
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
