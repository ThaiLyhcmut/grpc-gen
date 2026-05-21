#!/bin/bash
# Build + start the full LMS: 8 gRPC services (50051-50058) + GraphQL
# gateway (8080). Each service runs with its own dir as cwd so the
# relative ./<svc>.env and certs/ paths resolve. PIDs are recorded in
# run/pids for stop-lms.sh.
set -euo pipefail
cd "$(dirname "$0")/.."          # -> project root (lms/)
ROOT="$(pwd)"
mkdir -p bin logs run

SERVICES=(user class lesson material exam submission notification audit)

echo "🔨 building 8 services + gateway..."
for s in "${SERVICES[@]}"; do
  go build -o "bin/$s" "./src/service/$s"
done
go build -o bin/gateway ./src/server
echo "   build OK"

echo "🚀 starting gRPC services..."
: > run/pids
port=50051
for s in "${SERVICES[@]}"; do
  # setsid + </dev/null: fully detach into a new session so the launcher
  # returns cleanly (no orphan children tied to the caller's session).
  ( cd "src/service/$s" && exec setsid "$ROOT/bin/$s" > "$ROOT/logs/$s.log" 2>&1 < /dev/null ) &
  echo "$! $s" >> run/pids
  printf '   %-13s :%d  pid=%s\n' "$s" "$port" "$!"
  port=$((port + 1))
done

# Give services time to bind their port + open the DB pool before the
# gateway starts dialing them.
sleep 5

echo "🚀 starting GraphQL gateway..."
CERTS="$ROOT/src/server/certs"
USER_SERVICE_ADDR=localhost:50051 \
CLASS_SERVICE_ADDR=localhost:50052 \
LESSON_SERVICE_ADDR=localhost:50053 \
MATERIAL_SERVICE_ADDR=localhost:50054 \
EXAM_SERVICE_ADDR=localhost:50055 \
SUBMISSION_SERVICE_ADDR=localhost:50056 \
NOTIFICATION_SERVICE_ADDR=localhost:50057 \
AUDIT_SERVICE_ADDR=localhost:50058 \
TLS_CA_FILE="$CERTS/ca.crt" \
TLS_CERT_FILE="$CERTS/gateway-client.crt" \
TLS_KEY_FILE="$CERTS/gateway-client.key" \
CACHE_REDIS_ADDR=localhost:10002 \
CACHE_REDIS_PASSWORD='Th@i2004' \
CACHE_REDIS_DB=0 \
CACHE_TTL_SECONDS=300 \
JWT_HMAC_SECRET='lms-dev-secret' \
POLICY_MONGO_URI='mongodb://thaily:Th%40i2004@localhost:10000/?authSource=admin&replicaSet=rs0&directConnection=true' \
POLICY_MONGO_DB=lms_policy \
POLICY_MONGO_COLLECTION=casbin_rule \
GATEWAY_LISTEN=:8080 \
  setsid "$ROOT/bin/gateway" > logs/gateway.log 2>&1 < /dev/null &
echo "$! gateway" >> run/pids
printf '   %-13s :8080 pid=%s\n' "gateway" "$!"

sleep 3
echo ""
echo "✅ LMS is up — GraphQL playground: http://localhost:8080/"
echo "   per-process logs in logs/   |   stop with ./scripts/stop-lms.sh"
