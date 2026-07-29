#!/usr/bin/env bash
# Idempotent APISIX Admin API bootstrap.
#
# Phase 0: transparent proxy for gateway + resource
# Phase 1: key-auth + limit-req on /gateway/* (EMS / simulator)
#
# Requires APISIX Admin API (default host :9181 → container :9180).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APISIX_ADMIN_URL="${APISIX_ADMIN_URL:-http://127.0.0.1:9181}"
APISIX_ADMIN_KEY="${APISIX_ADMIN_KEY:-edd1c9f034335f136f87ad84b625c8f1}"

# Dev EMS/simulator API key (see deploy/apisix/conf/consumers.yaml).
SIMULATOR_CONSUMER="${SIMULATOR_CONSUMER:-simulator_default}"
SIMULATOR_API_KEY="${SIMULATOR_API_KEY:-vpp-dev-simulator-key}"

if [[ -z "${APISIX_ADMIN_KEY}" && -f "${SCRIPT_DIR}/conf/config.yaml" ]]; then
  APISIX_ADMIN_KEY="$(awk '/key:/{print $2; exit}' "${SCRIPT_DIR}/conf/config.yaml")"
fi

# Bypass local HTTP proxies (common on WSL/dev machines).
CURL=(curl --noproxy '*')

admin_curl() {
  "${CURL[@]}" -sfS \
    -H "X-API-KEY: ${APISIX_ADMIN_KEY}" \
    -H "Content-Type: application/json" \
    "$@"
}

wait_for_admin() {
  local attempts="${1:-30}"
  local i code
  for ((i = 1; i <= attempts; i++)); do
    code="$("${CURL[@]}" -s -o /dev/null -w '%{http_code}' \
      "${APISIX_ADMIN_URL}/apisix/admin/routes" \
      -H "X-API-KEY: ${APISIX_ADMIN_KEY}" \
      --connect-timeout 2 || true)"
    if [[ "${code}" == "200" ]]; then
      echo "APISIX Admin API is ready."
      return 0
    fi
    echo "Waiting for APISIX Admin API (${i}/${attempts}, last_http=${code:-none})..."
    sleep 2
  done
  echo "ERROR: APISIX Admin API not reachable at ${APISIX_ADMIN_URL}" >&2
  return 1
}

put_upstream() {
  local id=$1
  local host_port=$2
  admin_curl -X PUT "${APISIX_ADMIN_URL}/apisix/admin/upstreams/${id}" -d "{
    \"type\": \"roundrobin\",
    \"nodes\": {
      \"${host_port}\": 1
    },
    \"timeout\": {
      \"connect\": 5,
      \"send\": 30,
      \"read\": 30
    }
  }"
  echo
  echo "Upstream ${id} -> ${host_port}"
}

put_consumer_key_auth() {
  local username=$1
  local api_key=$2
  admin_curl -X PUT "${APISIX_ADMIN_URL}/apisix/admin/consumers/${username}" -d "{
    \"username\": \"${username}\",
    \"plugins\": {
      \"key-auth\": {
        \"key\": \"${api_key}\"
      }
    }
  }"
  echo
  echo "Consumer ${username} (key-auth)"
}

put_resource_route() {
  admin_curl -X PUT "${APISIX_ADMIN_URL}/apisix/admin/routes/resource-proxy" -d '{
    "uri": "/resource/*",
    "methods": ["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"],
    "upstream_id": "resource-backend",
    "plugins": {
      "proxy-rewrite": {
        "regex_uri": ["^/resource/(.*)", "/$1"]
      }
    }
  }'
  echo
  echo "Route resource-proxy: /resource/* -> resource-backend (no auth)"
}

put_gateway_route() {
  admin_curl -X PUT "${APISIX_ADMIN_URL}/apisix/admin/routes/gateway-proxy" -d '{
    "uri": "/gateway/*",
    "methods": ["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"],
    "upstream_id": "gateway-backend",
    "plugins": {
      "proxy-rewrite": {
        "regex_uri": ["^/gateway/(.*)", "/$1"]
      },
      "key-auth": {
        "header": "X-API-KEY"
      },
      "limit-req": {
        "rate": 100,
        "burst": 200,
        "key": "remote_addr",
        "rejected_code": 429
      }
    }
  }'
  echo
  echo "Route gateway-proxy: /gateway/* -> gateway-backend (key-auth + limit-req)"
}

main() {
  wait_for_admin

  put_upstream "gateway-backend" "host.docker.internal:8083"
  put_upstream "resource-backend" "host.docker.internal:8082"

  put_consumer_key_auth "${SIMULATOR_CONSUMER}" "${SIMULATOR_API_KEY}"

  put_gateway_route
  put_resource_route

  echo ""
  echo "Phase 0+1 routes installed."
  echo ""
  echo "Smoke test (services running):"
  echo "  # 401 without key"
  echo "  curl --noproxy '*' -s -o /dev/null -w '%{http_code}\n' \\"
  echo "    http://127.0.0.1:9080/gateway/api/v1/tenants/default/mappings"
  echo "  # 200 with key"
  echo "  curl --noproxy '*' -s -o /dev/null -w '%{http_code}\n' \\"
  echo "    -H 'X-API-KEY: ${SIMULATOR_API_KEY}' \\"
  echo "    http://127.0.0.1:9080/gateway/api/v1/tenants/default/mappings"
  echo "  # resource still open (Phase 2 adds JWT)"
  echo "  curl --noproxy '*' -s http://127.0.0.1:9080/resource/api/tenants/default/sites"
}

main "$@"
