FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the service
RUN CGO_ENABLED=0 GOOS=linux go build -o /user-service ./src/service/user

# Final stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Copy binary from builder
COPY --from=builder /user-service .

# Create env directory and copy env file
RUN mkdir -p env
COPY env/user.env env/user.env

# Expose port
EXPOSE 50051

# Run the service
CMD ["./user-service"]
