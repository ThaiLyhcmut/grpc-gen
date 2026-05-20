#!/bin/bash

# Generate TLS certificates for gRPC service
# Usage: ./generate-certs.sh <service-name>

set -e

SERVICE_NAME=$1

if [ -z "$SERVICE_NAME" ]; then
  echo "❌ Error: Service name is required"
  echo "Usage: ./generate-certs.sh <service-name>"
  echo "Example: ./generate-certs.sh user"
  exit 1
fi

CERTS_DIR="src/service/$SERVICE_NAME/certs"

if [ ! -d "proto/$SERVICE_NAME" ]; then
  echo "❌ Error: Service '$SERVICE_NAME' does not exist"
  echo "Please run 'grpc-gen add-service $SERVICE_NAME <port>' first"
  exit 1
fi

mkdir -p "$CERTS_DIR"
cd "$CERTS_DIR"

echo "🔐 Generating TLS certificates for '$SERVICE_NAME' service..."

# 1. Generate CA private key and certificate
echo "  ➜ Generating CA certificate..."
openssl req -x509 -newkey rsa:4096 -days 365 -nodes \
  -keyout ca-key.pem -out ca.crt \
  -subj "/C=VN/ST=HCM/L=HCM/O=Dev/OU=IT/CN=localhost" 2>/dev/null

# 2. Generate server private key
echo "  ➜ Generating server private key..."
openssl genrsa -out "$SERVICE_NAME-server.key" 4096 2>/dev/null

# 3. Generate server certificate signing request (CSR)
echo "  ➜ Creating certificate signing request..."
openssl req -new -key "$SERVICE_NAME-server.key" \
  -out "$SERVICE_NAME-server.csr" \
  -subj "/C=VN/ST=HCM/L=HCM/O=Dev/OU=IT/CN=localhost" 2>/dev/null

# 4. Sign server certificate with CA
echo "  ➜ Signing server certificate..."
openssl x509 -req -in "$SERVICE_NAME-server.csr" \
  -CA ca.crt -CAkey ca-key.pem -CAcreateserial \
  -out "$SERVICE_NAME-server.crt" -days 365 2>/dev/null

# 5. Clean up temporary files
rm "$SERVICE_NAME-server.csr" ca-key.pem ca.srl 2>/dev/null || true

echo ""
echo "✅ Certificates generated successfully!"
echo ""
echo "📁 Certificate files created in: $CERTS_DIR/"
echo "   ├── ca.crt                      # CA certificate"
echo "   ├── $SERVICE_NAME-server.crt    # Server certificate"
echo "   └── $SERVICE_NAME-server.key    # Server private key"
echo ""
echo "⚠️  Security Note:"
echo "   - These are self-signed certificates for DEVELOPMENT only"
echo "   - Do NOT use in production"
echo "   - Private keys are in .gitignore by default"
echo ""
