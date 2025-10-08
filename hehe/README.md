# hehe

Generated gRPC microservices project with CRUD operations.

## Prerequisites

- Go 1.24+
- Protocol Buffers compiler (protoc)
- MySQL 8.0+

## Project Structure

```
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

```

## Getting Started

### 1. Add a new service

```bash
grpc-generator add-service user 50051
```

### 2. Define entities in proto file

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

// Add other CRUD messages...
```

### 3. Generate service code

```bash
make gen-user
```

### 4. Build and run

```bash
go build ./src/service/user
./user
```

## Available Make Commands

- `make proto-{service}` - Generate protobuf code
- `make gen-{service}` - Generate service handlers
- `make gen-all` - Generate all services
- `make build-{service}` - Build service binary

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
