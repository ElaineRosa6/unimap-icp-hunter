#!/usr/bin/env bash
set -euo pipefail

CONFIG_PATH="${1:-configs/config.yaml}"
SERVER_BASE="${2:-http://127.0.0.1:8448}"

if [[ ! -f "$CONFIG_PATH" ]]; then
  echo "Config file not found: $CONFIG_PATH" >&2
  exit 1
fi

tmp_file="$(mktemp)"

awk '
BEGIN {
  in_screenshot=0
  in_extension=0
  updated_engine=0
  updated_ext_enabled=0
  updated_fallback=0
}
{
  line=$0

  if (line ~ /^screenshot:[[:space:]]*$/) {
    in_screenshot=1
    in_extension=0
    print line
    next
  }

  if (in_screenshot && line ~ /^[^[:space:]]/) {
    in_screenshot=0
    in_extension=0
  }

  if (!in_screenshot) {
    print line
    next
  }

  if (line ~ /^    extension:[[:space:]]*$/) {
    in_extension=1
    print line
    next
  }

  if (in_extension && line ~ /^    [a-zA-Z_]/) {
    in_extension=0
  }

  if (line ~ /^    engine:[[:space:]]*/) {
    print "    engine: cdp"
    updated_engine=1
    next
  }

  if (in_extension && line ~ /^        enabled:[[:space:]]*/) {
    print "        enabled: false"
    updated_ext_enabled=1
    next
  }

  if (in_extension && line ~ /^        fallback_to_cdp:[[:space:]]*/) {
    print "        fallback_to_cdp: true"
    updated_fallback=1
    next
  }

  print line
}
END {
  if (!updated_engine) {
    print "Failed to locate screenshot.engine" > "/dev/stderr"
    exit 2
  }
  if (!updated_ext_enabled) {
    print "Failed to locate screenshot.extension.enabled" > "/dev/stderr"
    exit 3
  }
  if (!updated_fallback) {
    print "Failed to locate screenshot.extension.fallback_to_cdp" > "/dev/stderr"
    exit 4
  }
}
' "$CONFIG_PATH" > "$tmp_file"

mv "$tmp_file" "$CONFIG_PATH"

echo "Updated $CONFIG_PATH -> engine=cdp, extension.enabled=false, fallback_to_cdp=true"
echo "Next step: restart service (recommended: ./scripts/stop.sh ; ./scripts/start.sh)"

echo "Bridge health check:"
if command -v curl >/dev/null 2>&1; then
  curl -fsS "$SERVER_BASE/api/screenshot/bridge/health" || echo "Health endpoint not reachable. Restart service and retry."
else
  echo "curl not found. Verify manually: $SERVER_BASE/api/screenshot/bridge/health"
fi
