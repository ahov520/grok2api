#!/bin/sh
set -eu

umask 077

quality_guard_dir=/var/lib/grok2api-quality-guard
mkdir -p "${quality_guard_dir}"
chown grok2api:grok2api "${quality_guard_dir}"
chmod 0700 "${quality_guard_dir}"

if [ ! -f "${GROK2API_CONFIG_SOURCE}" ]; then
  echo "missing config: ${GROK2API_CONFIG_SOURCE}" >&2
  echo "mount config.yaml to /run/grok2api/config.yaml" >&2
  exit 1
fi

cp "${GROK2API_CONFIG_SOURCE}" /app/config.yaml
chown grok2api:grok2api /app/config.yaml
chmod 0600 /app/config.yaml

# Optional qualityGuard injection for platforms that cannot mount config files
# (e.g. Render secret files). Set GROK2API_QG_ENABLED=true to append a
# qualityGuard section generated from GROK2API_QG_* env vars.
if [ "${GROK2API_QG_ENABLED:-false}" = "true" ]; then
  {
    printf '\nqualityGuard:\n'
    printf '  enabled: true\n'
    printf '  model: "%s"\n' "${GROK2API_QG_MODEL:-grok-4.5}"
    printf '  mode: %s\n' "${GROK2API_QG_MODE:-hybrid}"
    printf '  activeInterval: %s\n' "${GROK2API_QG_ACTIVE_INTERVAL:-30m}"
    printf '  passivePollInterval: %s\n' "${GROK2API_QG_PASSIVE_POLL_INTERVAL:-5s}"
    printf '  softTPS: %s\n' "${GROK2API_QG_SOFT_TPS:-500}"
    printf '  hardTPS: %s\n' "${GROK2API_QG_HARD_TPS:-1000}"
    printf '  consecutiveSoft: %s\n' "${GROK2API_QG_CONSECUTIVE_SOFT:-2}"
    printf '  consecutiveErrors: %s\n' "${GROK2API_QG_CONSECUTIVE_ERRORS:-2}"
    printf '  quarantineDuration: %s\n' "${GROK2API_QG_QUARANTINE_DURATION:-5m}"
    printf '  noAccountBackoff: %s\n' "${GROK2API_QG_NO_ACCOUNT_BACKOFF:-5m}"
    printf '  minimumHealthyNodes: %s\n' "${GROK2API_QG_MIN_HEALTHY_NODES:-3}"
    printf '  maxOutputTokens: %s\n' "${GROK2API_QG_MAX_OUTPUT_TOKENS:-384}"
    printf '  failClosed: %s\n' "${GROK2API_QG_FAIL_CLOSED:-false}"
    printf '  minimumGenerationWindow: %s\n' "${GROK2API_QG_MIN_GENERATION_WINDOW:-1s}"
    printf '  rotationURL: "%s"\n' "${GROK2API_QG_ROTATION_URL:-}"
    printf '  rotationToken: "%s"\n' "${GROK2API_QG_ROTATION_TOKEN:-}"
    printf '  rotationTimeout: %s\n' "${GROK2API_QG_ROTATION_TIMEOUT:-45s}"
    printf '  nodeIDs: []\n'
    printf '  rotatableNodeIDs: []\n'
  } >> /app/config.yaml
fi

exec su-exec grok2api:grok2api "$@"
