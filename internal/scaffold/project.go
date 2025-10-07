package scaffold

import (
	"fmt"
	"os"
)

// CreateProject initializes a new gRPC project with complete scaffolding
func CreateProject(projectName, modulePath string) error {
	fmt.Println("🔨 Creating project structure...")

	// Create directories
	dirs := []string{
		"proto/common",
		"src/service",
		"src/pkg/database",
		"src/pkg/logger",
		"src/pkg/helper",
		"env",
		"docker",
		"scripts/types",
		"scripts/parser",
		"scripts/generator",
		"scripts/utils",
		"template",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	fmt.Println("  ✓ Created directory structure")

	// Create go.mod
	if err := createGoMod(modulePath); err != nil {
		return err
	}
	fmt.Println("  ✓ Created go.mod")

	// Create common proto
	if err := createCommonProto(); err != nil {
		return err
	}
	fmt.Println("  ✓ Created common.proto")

	// Create pkg files
	if err := createPkgFiles(modulePath); err != nil {
		return err
	}
	fmt.Println("  ✓ Created pkg utilities")

	// Copy templates
	if err := copyTemplates(); err != nil {
		return err
	}
	fmt.Println("  ✓ Created templates")

	// Copy generator scripts
	if err := copyGeneratorScripts(); err != nil {
		return err
	}
	fmt.Println("  ✓ Created generator scripts")

	// Create Makefile
	if err := createMakefile(modulePath); err != nil {
		return err
	}
	fmt.Println("  ✓ Created Makefile")

	// Create README
	if err := createReadme(projectName, modulePath); err != nil {
		return err
	}
	fmt.Println("  ✓ Created README.md")

	// Create .gitignore
	if err := createGitignore(); err != nil {
		return err
	}
	fmt.Println("  ✓ Created .gitignore")

	return nil
}

func createGoMod(modulePath string) error {
	content := fmt.Sprintf(`module %s

go 1.24

require (
	github.com/google/uuid v1.6.0
	github.com/joho/godotenv v1.5.1
	google.golang.org/grpc v1.62.0
	google.golang.org/protobuf v1.32.0
	github.com/go-sql-driver/mysql v1.7.1
)
`, modulePath)

	return os.WriteFile("go.mod", []byte(content), 0644)
}

func createGitignore() error {
	content := `# Binaries
*.exe
*.exe~
*.dll
*.so
*.dylib
gen_skeleton
/src/service/*/main

# Test binary
*.test
*.out

# Build
/bin/
/build/

# IDE
.idea/
.vscode/
*.swp
*.swo
*~

# Env files
.env
*.env

# OS
.DS_Store
Thumbs.db
`
	return os.WriteFile(".gitignore", []byte(content), 0644)
}

func createReadme(projectName, modulePath string) error {
	content := fmt.Sprintf(`# %s

Generated gRPC microservices project with CRUD operations.

## Prerequisites

- Go 1.24+
- Protocol Buffers compiler (protoc)
- MySQL 8.0+

## Project Structure

`+"```"+`
.
├── proto/              # Protocol buffer definitions
│   ├── common/        # Common messages
│   └── {service}/     # Service-specific protos
├── src/
│   ├── service/       # Generated services
│   └── pkg/          # Shared packages
│       ├── database/ # Database utilities
│       ├── logger/   # Logger utilities
│       └── helper/   # Helper functions
├── scripts/          # Code generation scripts
├── template/         # Code templates
├── env/             # Environment files
└── docker/          # Dockerfiles

`+"```"+`

## Getting Started

### 1. Add a new service

`+"```bash"+`
grpc-generator add-service user 50051
`+"```"+`

### 2. Define entities in proto file

Edit `+"`proto/user/user.proto`"+`:

`+"```protobuf"+`
message User {
  string id = 1;
  string name = 2;
  string email = 3;
  UserStatus status = 4;
  google.protobuf.Timestamp created_at = 5;
  google.protobuf.Timestamp updated_at = 6;
  string created_by = 7;
  string updated_by = 8;
}

enum UserStatus {
  ACTIVE = 0;
  INACTIVE = 1;
  DELETED = 2;
}

message CreateUserRequest {
  string name = 1;
  string email = 2;
  UserStatus status = 3;
  string created_by = 4;
}

// Add other CRUD messages...
`+"```"+`

### 3. Generate service code

`+"```bash"+`
make gen-user
`+"```"+`

### 4. Build and run

`+"```bash"+`
go build ./src/service/user
./user
`+"```"+`

## Available Make Commands

- `+"`make proto-{service}`"+` - Generate protobuf code
- `+"`make gen-{service}`"+` - Generate service handlers
- `+"`make gen-all`"+` - Generate all services
- `+"`make build-{service}`"+` - Build service binary

## Features

- ✅ Full CRUD operations (Create, Read, Update, Delete, List)
- ✅ Pagination and filtering
- ✅ Enum support with database conversion
- ✅ Optional fields handling
- ✅ Logger with function tracing
- ✅ Database connection pooling
- ✅ Docker support

## License

MIT
`, projectName)

	return os.WriteFile("README.md", []byte(content), 0644)
}
