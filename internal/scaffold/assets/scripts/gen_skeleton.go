package main

import (
	"log"
	"os"
	"path/filepath"

	"gen_skeleton/generator"
	"gen_skeleton/parser"
	"gen_skeleton/types"
	"gen_skeleton/utils"
)

func main() {
	if len(os.Args) < 4 {
		log.Fatalf("Usage: go run gen_skeleton.go <proto_name> <service_name> <port>")
	}

	protoName := os.Args[1]   // e.g.: academic
	serviceName := os.Args[2] // e.g.: AcademicService
	port := os.Args[3]        // e.g.: 50051

	// Get module path from go.mod
	modulePath, err := utils.GetModulePath()
	if err != nil {
		log.Fatalf("Failed to get module path: %v", err)
	}

	packagePath := modulePath + "/proto/" + protoName

	// Parse proto file to get RPC methods
	protoFile := filepath.Join("proto", protoName, protoName+".proto")
	methods, err := parser.ParseProtoFile(protoFile)
	if err != nil {
		log.Fatalf("Failed to parse proto file: %v", err)
	}

	// Parse enums from proto file
	enums, err := parser.ParseEnumsFromProto(protoFile)
	if err != nil {
		log.Fatalf("Failed to parse enums: %v", err)
	}

	// Group methods by entity
	entityMethods := parser.GroupMethodsByEntity(methods)

	// Parse entity messages to get field information
	entityFields, err := parser.ParseEntityFields(protoFile, enums)
	if err != nil {
		log.Fatalf("Failed to parse entity fields: %v", err)
	}

	// Parse request messages to get required/optional fields
	requiredFieldsMap, optionalFieldsMap, err := parser.ParseFieldsFromCreateRequests(protoFile)
	if err != nil {
		log.Fatalf("Failed to parse required/optional fields: %v", err)
	}

	// Create directories
	serviceDir := filepath.Join("src", "service", protoName)
	handlerDir := filepath.Join(serviceDir, "handler")
	os.MkdirAll(handlerDir, 0755)
	os.MkdirAll("env", 0755)
	os.MkdirAll("docker", 0755)

	data := types.Data{
		PackagePath: packagePath,
		ProtoName:   protoName,
		ServiceName: serviceName,
		Port:        port,
		ModulePath:  modulePath,
	}

	// Generate main.go
	generator.GenerateMain(serviceDir, data)

	// Generate handler/handler.go
	generator.GenerateHandlerRoot(handlerDir, types.HandlerData{
		PackagePath: packagePath,
		ServiceName: serviceName,
		ModulePath:  modulePath,
	})

	// Generate CRUD handler files for each entity
	for entityName, methods := range entityMethods {
		if parser.IsCRUDEntity(methods) {
			// Generate full CRUD handler
			generator.GenerateCRUDHandler(handlerDir, packagePath, entityName, methods, entityFields[entityName], enums, requiredFieldsMap, optionalFieldsMap)
		} else {
			// Generate simple entity handler
			generator.GenerateEntityHandler(handlerDir, types.EntityHandlerData{
				PackagePath: packagePath,
				EntityName:  entityName,
				Methods:     methods,
			})
		}
	}

	// Generate env file
	generator.GenerateEnvFile(protoName, data)

	// Generate Dockerfile
	generator.GenerateDockerfile(protoName, data)

	log.Printf("Generated skeleton for %s service\n", serviceName)
}
