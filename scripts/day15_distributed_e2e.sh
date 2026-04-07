#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${1:-http://127.0.0.1:8448}"
NODE_ID="${2:-node-e2e-a}"
NODE_TOKEN="${3:-token-e2e-a}"
TASK_ID="${4:-task-e2e-1}"
ADMIN_TOKEN="${5:-$NODE_TOKEN}"

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required for this script" >&2
  exit 1
fi

auth_header=( -H "Authorization: Bearer ${NODE_TOKEN}" )
admin_header=( -H "Authorization: Bearer ${ADMIN_TOKEN}" )

echo "[1/7] Register node..."
register_resp="$(curl -fsS -X POST "${BASE_URL}/api/nodes/register" "${auth_header[@]}" -H "Content-Type: application/json" -d "{\"node_id\":\"${NODE_ID}\",\"hostname\":\"worker-e2e\",\"region\":\"local\",\"max_concurrency\":2,\"capabilities\":[\"port_scan\",\"screenshot\"],\"egress_ip\":\"127.0.0.1\"}")"
echo "${register_resp}" | jq -e '.success == true' >/dev/null

echo "[2/7] Heartbeat..."
heartbeat_resp="$(curl -fsS -X POST "${BASE_URL}/api/nodes/heartbeat" "${auth_header[@]}" -H "Content-Type: application/json" -d "{\"node_id\":\"${NODE_ID}\",\"current_load\":0,\"max_concurrency\":2,\"avg_latency_ms\":9.6,\"success_rate_5m\":99.9,\"egress_ip\":\"127.0.0.1\"}")"
echo "${heartbeat_resp}" | jq -e '.success == true' >/dev/null

echo "[3/7] Enqueue task..."
enqueue_resp="$(curl -fsS -X POST "${BASE_URL}/api/nodes/task/enqueue" "${admin_header[@]}" -H "Content-Type: application/json" -d "{\"task_id\":\"${TASK_ID}\",\"task_type\":\"port_scan\",\"priority\":10,\"required_caps\":[\"port_scan\"],\"payload\":{\"url\":\"https://example.com\"}}")"
echo "${enqueue_resp}" | jq -e '.success == true' >/dev/null

echo "[4/7] Claim task..."
claim_resp="$(curl -fsS -X POST "${BASE_URL}/api/nodes/task/claim" "${auth_header[@]}" -H "Content-Type: application/json" -d "{\"node_id\":\"${NODE_ID}\",\"caps\":[\"port_scan\"]}")"
echo "${claim_resp}" | jq -e '.success == true and .task != null' >/dev/null

echo "[5/7] Submit task result..."
result_resp="$(curl -fsS -X POST "${BASE_URL}/api/nodes/task/result" "${auth_header[@]}" -H "Content-Type: application/json" -d "{\"task_id\":\"${TASK_ID}\",\"node_id\":\"${NODE_ID}\",\"status\":\"completed\",\"duration_ms\":18,\"output\":{\"ok\":true,\"source\":\"day15_distributed_e2e.sh\"}}")"
echo "${result_resp}" | jq -e '.success == true' >/dev/null

echo "[6/7] Check task snapshot..."
task_status_resp="$(curl -fsS "${BASE_URL}/api/nodes/task/status" "${admin_header[@]}")"
echo "${task_status_resp}" | jq -e '.success == true' >/dev/null

echo "[7/7] Check network profile..."
profile_resp="$(curl -fsS "${BASE_URL}/api/nodes/network/profile" "${admin_header[@]}")"
echo "${profile_resp}" | jq -e '.success == true' >/dev/null

echo "\n=== REGISTER ==="
echo "${register_resp}" | jq .
echo "\n=== CLAIM ==="
echo "${claim_resp}" | jq .
echo "\n=== RESULT ==="
echo "${result_resp}" | jq .
echo "\n=== TASK STATUS ==="
echo "${task_status_resp}" | jq .
echo "\n=== NETWORK PROFILE ==="
echo "${profile_resp}" | jq .
