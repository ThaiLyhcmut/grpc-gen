# Logger - Function Tracer

Simple function tracing package for Go gRPC services.

## Features

- **Automatic function tracing**: Track function call hierarchy
- **Database query tracking**: Log all SQL queries with duration
- **Request ID**: Unique ID for each request
- **JSON output**: Easy to parse trace logs

## Quick Start

### 1. Add interceptor to gRPC server

```go
import "thaily/src/pkg/logger"

grpcServer := grpc.NewServer(
    grpc.UnaryInterceptor(logger.UnaryServerInterceptor()),
)
```

### 2. Add tracing to handlers

```go
func (h *Handler) CreateStudent(ctx context.Context, req *pb.CreateStudentRequest) (*pb.CreateStudentResponse, error) {
    defer logger.TraceFunction(ctx)()

    // Your code here...
}
```

### 3. Track database queries

In your handler helper methods:

```go
func (h *Handler) execQuery(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
    start := time.Now()
    result, err := h.db.ExecContext(ctx, query, args...)
    duration := time.Since(start)

    // Add query to trace
    logger.AddQueryToTrace(ctx, query, duration.Milliseconds())

    return result, err
}
```

## Output Example

```json
{
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "method": "/user.UserService/CreateStudent",
  "duration_ms": 156,
  "success": true,
  "trace": {
    "function_name": "CreateStudent",
    "duration_ms": 155,
    "queries": [
      {
        "query": "INSERT INTO Student (id, email, ...) VALUES (?, ?, ...)",
        "duration_ms": 45
      },
      {
        "query": "SELECT id, email, ... FROM Student WHERE id = ?",
        "duration_ms": 12
      }
    ],
    "children": []
  },
  "queries": [
    {
      "query": "INSERT INTO Student (id, email, ...) VALUES (?, ?, ...)",
      "duration_ms": 45
    },
    {
      "query": "SELECT id, email, ... FROM Student WHERE id = ?",
      "duration_ms": 12
    }
  ]
}
```

## API Reference

### Context Functions

- `WithRequestID(ctx)` - Add request ID to context
- `GetRequestID(ctx)` - Get request ID from context
- `WithTraceStack(ctx)` - Add trace stack to context
- `GetTraceStack(ctx)` - Get trace stack from context

### Tracing Functions

- `TraceFunction(ctx)` - Trace current function (auto-detect name)
- `TraceFunctionWithName(ctx, name)` - Trace with explicit name
- `AddQueryToTrace(ctx, query, durationMs)` - Add SQL query to trace

### Helper Functions

- `GetAllTraces(trace)` - Flatten trace tree to list
- `FlattenQueries(trace)` - Get all queries from trace tree

## Files

- `tracer.go` - Core tracing logic
- `context.go` - Context helpers (request ID)
- `helper.go` - Helper functions
- `interceptor.go` - gRPC interceptor
