#!/bin/sh
# Standalone bootstrap generator for the egress quality guard.
# Builds bootstrap.json from environment variables so the guard can run
# against a remote grok2api (e.g. Render) without a shared compose volume.
set -eu

mkdir -p /var/lib/grok2api-quality-guard
chmod 0700 /var/lib/grok2api-quality-guard

python3 - <<'PYEOF'
import json
import os

path = "/var/lib/grok2api-quality-guard/bootstrap.json"


def env(name, default):
    return os.environ.get(name, default)


payload = {
    "version": 1,
    "enabled": True,
    "internal_token": env("GROK2API_INTERNAL_TOKEN", ""),
    "config": {
        "model": env("GROK2API_QG_MODEL", "grok-4.5"),
        "prompt": "Write exactly 16 numbered lines about reliable distributed systems. Each line must be one complete English sentence, with no markdown heading. The final line must end with the exact marker QUALITY_OK.",
        "expected": "QUALITY_OK",
        "node_ids": json.loads(env("GROK2API_QG_NODE_IDS", "[]")),
        "mode": env("GROK2API_QG_MODE", "hybrid"),
        "active_interval_seconds": int(env("GROK2API_QG_ACTIVE_INTERVAL", "1800")),
        "passive_poll_seconds": int(env("GROK2API_QG_PASSIVE_POLL", "5")),
        "soft_tps": float(env("GROK2API_QG_SOFT_TPS", "500")),
        "hard_tps": float(env("GROK2API_QG_HARD_TPS", "1000")),
        "consecutive_soft": int(env("GROK2API_QG_CONSECUTIVE_SOFT", "2")),
        "consecutive_errors": int(env("GROK2API_QG_CONSECUTIVE_ERRORS", "2")),
        "quarantine_seconds": int(env("GROK2API_QG_QUARANTINE", "300")),
        "no_account_backoff_seconds": int(env("GROK2API_QG_NO_ACCOUNT_BACKOFF", "300")),
        "min_healthy_nodes": int(env("GROK2API_QG_MIN_HEALTHY", "3")),
        "max_output_tokens": int(env("GROK2API_QG_MAX_OUTPUT_TOKENS", "384")),
        "fail_closed": env("GROK2API_QG_FAIL_CLOSED", "false").lower() == "true",
        "min_generation_ms": int(env("GROK2API_QG_MIN_GENERATION_MS", "1000")),
        "rotation_url": env("GROK2API_QG_ROTATION_URL", ""),
        "rotation_token": env("GROK2API_QG_ROTATION_TOKEN", ""),
        "rotation_timeout_seconds": int(env("GROK2API_QG_ROTATION_TIMEOUT", "45")),
        "rotatable_node_ids": json.loads(env("GROK2API_QG_ROTATABLE_NODE_IDS", "[]")),
    },
}

temporary = path + ".tmp"
with open(temporary, "w", encoding="utf-8") as handle:
    json.dump(payload, handle)
os.chmod(temporary, 0o600)
os.replace(temporary, path)
PYEOF

# Tiny HTTP liveness server so a Web Service deployment (Render free plan
# only allows web services) stays awake: Render pings /healthz periodically.
python3 -c '
import http.server, socketserver
class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Length", "2")
        self.end_headers()
        self.wfile.write(b"ok")
    def log_message(self, *args):
        pass
socketserver.TCPServer(("0.0.0.0", 8000), Handler).serve_forever()
' &
LIVENESS_PID=$!
trap 'kill $LIVENESS_PID 2>/dev/null || true' EXIT TERM INT

exec /usr/local/bin/grok2api-egress-quality-guard "$@"
