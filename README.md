# gRPC Generator

CLI để scaffold full-stack gRPC system: từ `.proto` → service handlers → GraphQL gateway → business-rule engine, một lệnh.

## Features

### Tier 1 — Service scaffold (`add-service`)
✅ Quick project init
✅ Full CRUD (Create/Update/Delete/List) handlers từ proto entity
✅ MySQL integration với connection pooling
✅ Function tracing + RPC logging
✅ Enum mapping proto ↔ DB strings
✅ Optional field handling cho Update
✅ Pagination + filter (whitelist annotation-driven)
✅ Filter operators: EQUAL, IN, GREATER_THAN, BETWEEN, IS_NULL, ...
✅ Dockerfile per service

### Tier 2 — GraphQL gateway (`add-gateway`)
✅ Auto-generate GraphQL schema từ proto + `gateway-config.yaml`
✅ Resolvers + gRPC client wrappers + DataLoader (N+1 protection)
✅ Per-entity `op_auth` + `field_auth` qua `@auth(role:)` directive
✅ Nested relations (belongsTo / hasOne / hasMany)
✅ gqlgen-backed, type-safe TypeScript codegen-friendly

### Tier 3 — Dynamic business engine (auto-generated)
✅ **Mongo-backed business rules** — `Create*`/`Update*`/`Delete*` chặn qua expr-lang condition, hot-reload
✅ **Read-scope RLS** — auto AND-prepend filter trên `List*`, direct + multi-hop chain
✅ **Action engine** — declarative fetch/compute/write pipeline (compute final grade, recompute totals, ...)
✅ **Action triggers** — auto-run action sau mutation thành công (`Triggers: ["CreateXxx"]`)
✅ **System context** — engine internal fetches bypass RLS để tránh recursion + false-deny
✅ Fail-open trên engine error (rule store down ≠ block writes)
✅ Fail-closed trên scope chain empty (sentinel `0` filter)

→ **Add features bằng cách edit Mongo doc, không rebuild.** Cold edit chỉ khi đổi schema.

📖 Doc chi tiết: [`docs/gateway.md`](docs/gateway.md)

## Installation

```bash
# Clone the repository
git clone https://github.com/thailyhcmut/grpc-gen.git
cd grpc-gen

# Build the tool
go build -o grpc-gen .

# Install globally (optional)
sudo mv grpc-gen /usr/local/bin/
```

Or install directly from source:

```bash
go install github.com/thailyhcmut/grpc-gen@latest
```

## Quick Start

### 1. Create a new project

```bash
grpc-gen init my-project
cd my-project
```

This creates:
```
my-project/
├── proto/              # Protocol buffer definitions
│   └── common/        # Common messages
├── src/
│   ├── service/       # Generated services
│   └── pkg/          # Shared packages
│       ├── database/ # Database utilities
│       ├── logger/   # Logger utilities
│       └── helper/   # Helper functions
├── scripts/          # Code generation scripts
├── template/         # Code templates
├── env/             # Environment files
├── docker/          # Dockerfiles
├── Makefile         # Build automation
└── go.mod
```

### 2. Add a service

```bash
grpc-gen add-service user 50051
```

This creates `proto/user/user.proto` with an example entity.

### 3. Define your entities

Edit `proto/user/user.proto`:

```protobuf
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

message CreateUserResponse {
  User user = 1;
}

// Define Get, Update, Delete, List messages...

service UserService {
  rpc CreateUser(CreateUserRequest) returns (CreateUserResponse);
  rpc GetUser(GetUserRequest) returns (GetUserResponse);
  rpc UpdateUser(UpdateUserRequest) returns (UpdateUserResponse);
  rpc DeleteUser(DeleteUserRequest) returns (DeleteUserResponse);
  rpc ListUsers(ListUsersRequest) returns (ListUsersResponse);
}
```

### 4. Generate service code

```bash
make gen-user
```

This generates:
- `src/service/user/main.go` - Service entry point
- `src/service/user/handler/handler.go` - Base handler
- `src/service/user/handler/user.go` - CRUD operations
- `env/user.env` - Environment variables
- `docker/user.Dockerfile` - Docker configuration

### 5. Configure database

Edit `env/user.env`:

```env
DB_HOST=localhost
DB_PORT=3306
DB_USER=root
DB_PASSWORD=yourpassword
DB_NAME=your_database

SERVICE_NAME=UserService
SERVICE_PORT=50051
```

### 6. Build and run

```bash
go mod download
go build ./src/service/user
./user
```

## Commands

### `grpc-gen init [project-name]`

Initialize a new gRPC project.

**Options:**
- `-m, --module` - Go module path (default: project-name)

**Example:**
```bash
grpc-gen init my-api
grpc-gen init my-api -m github.com/myorg/my-api
```

### `grpc-gen add-service [name] [port]`

Add a new service to the project.

**Arguments:**
- `name` - Service name (lowercase)
- `port` - Service port (1024-65535)

**Example:**
```bash
grpc-gen add-service order 50052
grpc-gen add-service payment 50053
```

## Project Structure

```
.
├── proto/                    # Protocol Buffers
│   ├── common/              # Common messages
│   │   └── common.proto     # Pagination, filters
│   └── [service]/           # Service-specific
│       └── [service].proto  # Service definition
│
├── src/
│   ├── service/             # Generated services
│   │   └── [service]/
│   │       ├── main.go      # Entry point
│   │       └── handler/     # Request handlers
│   │           ├── handler.go
│   │           └── [entity].go
│   │
│   └── pkg/                 # Shared packages
│       ├── database/        # DB connection
│       ├── logger/          # Logging utilities
│       └── helper/          # Filter builders
│
├── scripts/                 # Code generation
│   ├── gen_skeleton.go      # Main generator
│   ├── types/               # Type definitions
│   ├── parser/              # Proto parser
│   ├── generator/           # Code generator
│   └── utils/               # Utilities
│
├── template/                # Code templates
│   ├── main.tmpl           # Service main
│   ├── handler.tmpl        # Handler base
│   ├── crud_handler.tmpl   # CRUD operations
│   ├── env.tmpl            # Environment
│   └── dockerfile.tmpl     # Docker
│
├── env/                     # Environment files
├── docker/                  # Dockerfiles
├── Makefile                 # Build automation
└── go.mod                   # Go module
```

## Generated CRUD Operations

For each entity with full CRUD methods, the generator creates:

### Create
- UUID generation
- Required field validation (string fields only)
- Optional field handling
- Enum to string conversion for database
- Returns created entity

### Get
- Retrieves by ID
- String to enum conversion
- NULL-safe timestamp and optional fields
- Returns 404 if not found

### Update
- Dynamic UPDATE query (only updates provided fields)
- Optional field support
- Enum conversion
- Returns updated entity

### Delete
- Soft or hard delete
- Returns success boolean

### List
- Pagination (page, page_size, sort_by, descending)
- Filtering (eq, ne, gt, gte, lt, lte, like, in)
- Whitelist-based field filtering
- Returns entities with total count

## Makefile Targets

```bash
make proto-[service]    # Generate protobuf code
make gen-[service]      # Generate service handlers
make gen-all           # Generate all services
make clean             # Clean generated services
make clean-all         # Clean everything including proto files
```

## Examples

### Simple String Entity

```protobuf
message Product {
  string id = 1;
  string name = 2;
  string description = 3;
  double price = 4;
  google.protobuf.Timestamp created_at = 5;
  google.protobuf.Timestamp updated_at = 6;
  string created_by = 7;
  string updated_by = 8;
}

message CreateProductRequest {
  string name = 1;
  string description = 2;
  double price = 3;
  string created_by = 4;
}
```

### Entity with Enum

```protobuf
enum OrderStatus {
  PENDING = 0;
  PAID = 1;
  SHIPPED = 2;
  DELIVERED = 3;
  CANCELLED = 4;
}

message Order {
  string id = 1;
  string customer_id = 2;
  OrderStatus status = 3;
  double total = 4;
  google.protobuf.Timestamp created_at = 5;
  google.protobuf.Timestamp updated_at = 6;
  string created_by = 7;
  string updated_by = 8;
}

message CreateOrderRequest {
  string customer_id = 1;
  OrderStatus status = 2;
  double total = 3;
  string created_by = 4;
}
```

### Entity with Optional Fields

```protobuf
message Profile {
  string id = 1;
  string user_id = 2;
  string bio = 3;
  optional string avatar_url = 4;
  optional string website = 5;
  google.protobuf.Timestamp created_at = 6;
  google.protobuf.Timestamp updated_at = 7;
  string created_by = 8;
  string updated_by = 9;
}

message CreateProfileRequest {
  string user_id = 1;
  string bio = 2;
  optional string avatar_url = 3;
  optional string website = 4;
  string created_by = 5;
}
```

## Requirements

- Go 1.24+
- Protocol Buffers compiler (`protoc`)
- MySQL 8.0+

## License

MIT

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Support

If you encounter any issues, please file an issue on GitHub.
