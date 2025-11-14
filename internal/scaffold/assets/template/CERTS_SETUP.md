# TLS Certificates Setup Guide

This guide explains how to generate TLS certificates for your gRPC services.

## Quick Start

Each service needs its own TLS certificates in `src/service/{service_name}/certs/`:

```
src/service/{service_name}/certs/
├── {service_name}-server.crt    # Server certificate
├── {service_name}-server.key    # Server private key
└── ca.crt                        # Certificate Authority
```

## Generate Certificates

### Option 1: Using OpenSSL (Development)

```bash
# Navigate to service directory
cd src/service/{service_name}/certs

# 1. Generate CA private key and certificate
openssl req -x509 -newkey rsa:4096 -days 365 -nodes \
  -keyout ca-key.pem -out ca.crt \
  -subj "/C=VN/ST=HCM/L=HCM/O=YourOrg/OU=IT/CN=*.yourdomain.com"

# 2. Generate server private key
openssl genrsa -out {service_name}-server.key 4096

# 3. Generate server certificate signing request (CSR)
openssl req -new -key {service_name}-server.key \
  -out {service_name}-server.csr \
  -subj "/C=VN/ST=HCM/L=HCM/O=YourOrg/OU=IT/CN={service_name}-service"

# 4. Sign server certificate with CA
openssl x509 -req -in {service_name}-server.csr \
  -CA ca.crt -CAkey ca-key.pem -CAcreateserial \
  -out {service_name}-server.crt -days 365

# 5. Clean up
rm {service_name}-server.csr ca-key.pem ca.srl
```

### Option 2: Using Script (Recommended)

Create a script `generate-certs.sh` in project root:

```bash
#!/bin/bash

SERVICE_NAME=$1

if [ -z "$SERVICE_NAME" ]; then
  echo "Usage: ./generate-certs.sh <service-name>"
  exit 1
fi

CERTS_DIR="src/service/$SERVICE_NAME/certs"
mkdir -p $CERTS_DIR
cd $CERTS_DIR

# Generate CA
openssl req -x509 -newkey rsa:4096 -days 365 -nodes \
  -keyout ca-key.pem -out ca.crt \
  -subj "/C=VN/ST=HCM/L=HCM/O=YourOrg/OU=IT/CN=*.yourdomain.com"

# Generate server key
openssl genrsa -out $SERVICE_NAME-server.key 4096

# Generate server CSR
openssl req -new -key $SERVICE_NAME-server.key \
  -out $SERVICE_NAME-server.csr \
  -subj "/C=VN/ST=HCM/L=HCM/O=YourOrg/OU=IT/CN=$SERVICE_NAME-service"

# Sign server certificate
openssl x509 -req -in $SERVICE_NAME-server.csr \
  -CA ca.crt -CAkey ca-key.pem -CAcreateserial \
  -out $SERVICE_NAME-server.crt -days 365

# Clean up
rm $SERVICE_NAME-server.csr ca-key.pem ca.srl

echo "✓ Certificates generated for $SERVICE_NAME service"
```

Then run:
```bash
chmod +x generate-certs.sh
./generate-certs.sh user
./generate-certs.sh academic
# ... for each service
```

## Development vs Production

### Development
Set environment variable in your `.env`:
```env
CODE=
SERVICE_CERT_PATH=/certs
```

Certificates are loaded from relative path: `../service-name/certs/`

### Production (Docker)
Set environment variable:
```env
CODE=PRODUCTION
```

Certificates are loaded from: `/app/service/` (mounted volume in Docker)

## Verify Certificates

```bash
# Check certificate details
openssl x509 -in {service_name}-server.crt -text -noout

# Verify certificate chain
openssl verify -CAfile ca.crt {service_name}-server.crt
```

## Security Notes

⚠️ **Important:**
- Never commit private keys (`.key` files) to git
- Use `.gitignore` to exclude certificate files
- Rotate certificates before expiration
- Use proper CN (Common Name) for production
- Consider using Let's Encrypt for production environments

## Troubleshooting

### Error: "certificate not found"
- Ensure certificates are in correct directory
- Check file permissions (readable by service)
- Verify paths in `.env` file

### Error: "certificate verification failed"
- Ensure CA certificate matches the one used to sign server cert
- Check certificate expiration dates
- Verify certificate chain

## mTLS (Mutual TLS)

For client authentication, you also need client certificates. See the full guide in your main server documentation.
