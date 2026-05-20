# Package Documentation

This directory contains shared packages used across all services.

## Packages

### database
Database connection pooling and management with MySQL support.

Features:
- Connection pooling configuration
- Environment-based configuration
- Thread-safe global DB instance

### logger
Structured logging with file output and function tracing.

Features:
- File-based logging
- Function execution tracing
- gRPC interceptor for request/response logging
- Query logging support

### helper
Helper utilities for building SQL queries from proto filter conditions.

Features:
- Filter condition builder
- Nested filter group support
- Field whitelist validation
- Safe SQL query generation

### tls
TLS/mTLS credential management for secure gRPC communication.

Features:
- Server TLS credentials loading
- Client TLS credentials loading
- Certificate verification
- Support for both development and production environments

### config
Configuration management (if present).

### container
Dependency injection container (if present).

## Usage

Import packages in your service handlers:

```go
import (
    "yourmodule/src/service/pkg/database"
    "yourmodule/src/service/pkg/logger"
    "yourmodule/src/service/pkg/helper"
    "yourmodule/src/service/pkg/tls"
)
```
