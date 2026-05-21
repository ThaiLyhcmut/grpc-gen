#!/bin/bash
# Regenerate every service cert + the gateway client cert from ONE shared CA.
#
# Why: each service originally got its own self-signed CA, so a single
# gateway client cert could never satisfy all 8 services' mTLS check
# (ClientAuth: RequireAndVerifyClientCert). With one shared root CA every
# service trusts the same gateway client cert, and the gateway trusts every
# service's server cert.
#
# Idempotent: rerun any time. Overwrites src/service/<svc>/certs/* and
# src/server/certs/*.
set -euo pipefail
cd "$(dirname "$0")/.."   # -> project root (lms/)

SERVICES=(user class lesson material exam submission notification audit)
CA_DIR=".shared-ca"
DAYS=3650

echo "🔐 Shared-CA cert regeneration"
mkdir -p "$CA_DIR"

# ── 1. Root CA ───────────────────────────────────────────────────────────
echo "  ➜ root CA"
openssl genrsa -out "$CA_DIR/ca.key" 4096 2>/dev/null
openssl req -x509 -new -nodes -key "$CA_DIR/ca.key" -sha256 -days "$DAYS" \
  -out "$CA_DIR/ca.crt" \
  -subj "/C=VN/ST=HCM/L=HCM/O=LMS-Dev/OU=Platform/CN=LMS Root CA" 2>/dev/null

# Extensions applied to every leaf cert: usable as BOTH server and client
# (the gateway presents its leaf as a client; services present theirs as
# servers — and the gateway leaf only needs clientAuth, but keeping both
# keeps the helper uniform).
EXT_FILE="$CA_DIR/leaf.ext"
cat > "$EXT_FILE" <<'EOF'
subjectAltName = DNS:localhost, IP:127.0.0.1
extendedKeyUsage = serverAuth, clientAuth
keyUsage = digitalSignature, keyEncipherment
EOF

# ── helper: sign one leaf cert ───────────────────────────────────────────
# gen_leaf <out_dir> <basename>
gen_leaf() {
  local dir="$1" base="$2"
  mkdir -p "$dir"
  openssl genrsa -out "$dir/$base.key" 4096 2>/dev/null
  openssl req -new -key "$dir/$base.key" -out "$dir/$base.csr" \
    -subj "/C=VN/ST=HCM/L=HCM/O=LMS-Dev/OU=Platform/CN=localhost" 2>/dev/null
  openssl x509 -req -in "$dir/$base.csr" \
    -CA "$CA_DIR/ca.crt" -CAkey "$CA_DIR/ca.key" -CAcreateserial \
    -out "$dir/$base.crt" -days "$DAYS" -sha256 -extfile "$EXT_FILE" 2>/dev/null
  cp "$CA_DIR/ca.crt" "$dir/ca.crt"
  rm -f "$dir/$base.csr"
}

# ── 2. Per-service server certs ──────────────────────────────────────────
for s in "${SERVICES[@]}"; do
  echo "  ➜ service: $s"
  gen_leaf "src/service/$s/certs" "$s-server"
done

# ── 3. Gateway client cert ───────────────────────────────────────────────
echo "  ➜ gateway client cert"
gen_leaf "src/server/certs" "gateway-client"

echo ""
echo "✅ Done. All certs share CA: $CA_DIR/ca.crt"
echo "   Gateway dials with: src/server/certs/{gateway-client.crt,gateway-client.key,ca.crt}"
echo "   Restart services + gateway to pick up the new certs."
