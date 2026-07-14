#!/bin/sh
set -eu

umask 077

if [ -f "${GROK2API_CONFIG_SOURCE}" ]; then
  cp "${GROK2API_CONFIG_SOURCE}" /app/config.yaml
elif [ -n "${GROK2API_CONFIG_YAML:-}" ]; then
  echo "${GROK2API_CONFIG_YAML}" > /app/config.yaml
elif [ -n "${GROK2API_JWT_SECRET:-}" ] && [ -n "${GROK2API_ENCRYPTION_KEY:-}" ] && [ -n "${GROK2API_ADMIN_PASSWORD:-}" ]; then
  PUBLIC_URL="${GROK2API_PUBLIC_URL:-${RENDER_EXTERNAL_URL:-http://localhost:8000}}"
  ADMIN_USER="${GROK2API_ADMIN_USER:-admin}"
  cat > /app/config.yaml << EOFCONFIG
server:
  listen: "0.0.0.0:8000"
  maxBodyBytes: 33554432
  readTimeout: 15m
  requestTimeout: 2h
  swaggerEnabled: false
secrets:
  jwtSecret: "${GROK2API_JWT_SECRET}"
  credentialEncryptionKey: "${GROK2API_ENCRYPTION_KEY}"
bootstrapAdmin:
  username: "${ADMIN_USER}"
  password: "${GROK2API_ADMIN_PASSWORD}"
frontend:
  publicApiBaseURL: "${PUBLIC_URL}"
  staticPath: "./frontend/dist"
database:
  driver: sqlite
  sqlite:
    path: "./data/backend.db"
runtimeStore:
  driver: memory
auth:
  accessTokenTTL: 15m
  refreshTokenTTL: 720h
  secureCookies: false
provider:
  console:
    baseURL: "https://console.x.ai"
    userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
    chatTimeout: 5m
media:
  driver: local
  local:
    path: "./data/media"
EOFCONFIG
else
  echo "missing config: ${GROK2API_CONFIG_SOURCE}" >&2
  echo "Provide config via GROK2API_CONFIG_SOURCE (file mount), GROK2API_CONFIG_YAML (full yaml), or set GROK2API_JWT_SECRET, GROK2API_ENCRYPTION_KEY, GROK2API_ADMIN_PASSWORD env vars" >&2
  exit 1
fi

chown grok2api:grok2api /app/config.yaml
chmod 0600 /app/config.yaml

exec su-exec grok2api:grok2api "$@"
