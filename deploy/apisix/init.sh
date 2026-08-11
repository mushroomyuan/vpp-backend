#!/usr/bin/env bash
# Idempotent APISIX Admin API bootstrap.
#
# Phase 0: transparent proxy for gateway + resource
# Phase 1: key-auth + limit-req on /gateway/* (EMS / simulator)
# Phase 2: openid-connect bearer_only on /resource/* (Casdoor OIDC)
# Phase 2b / C10b: openid-connect on /gateway/.../mappings* (human-managed mappings)
# Gate 0: plaintext HTTP/2 :9081 + dispatch/telemetry-read gRPC + openid-connect
#
# Requires APISIX Admin API (default host :9181 → container :9180).
# Casdoor should be reachable from the APISIX container at host.docker.internal:8000.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APISIX_ADMIN_URL="${APISIX_ADMIN_URL:-http://127.0.0.1:9181}"
APISIX_ADMIN_KEY="${APISIX_ADMIN_KEY:-edd1c9f034335f136f87ad84b625c8f1}"

# Dev EMS/simulator API key (see deploy/apisix/conf/consumers.yaml).
SIMULATOR_CONSUMER="${SIMULATOR_CONSUMER:-simulator_default}"
SIMULATOR_API_KEY="${SIMULATOR_API_KEY:-vpp-dev-simulator-key}"

# Casdoor OIDC (see deploy/casdoor/conf/credentials.yaml).
OIDC_CLIENT_ID="${OIDC_CLIENT_ID:-vpp-resource-dev-client}"
OIDC_CLIENT_SECRET="${OIDC_CLIENT_SECRET:-vpp-resource-dev-secret}"
# Must be reachable FROM the APISIX container (not 127.0.0.1).
OIDC_DISCOVERY="${OIDC_DISCOVERY:-http://host.docker.internal:8000/.well-known/openid-configuration}"

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
  local scheme=${3:-http}
  admin_curl -X PUT "${APISIX_ADMIN_URL}/apisix/admin/upstreams/${id}" -d "{
    \"type\": \"roundrobin\",
    \"scheme\": \"${scheme}\",
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
  echo "Upstream ${id} -> ${host_port} (scheme=${scheme})"
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
  # Path C: bearer_only + JWKS verify + X-Userinfo only (no Lua / no X-Roles split).
  local body
  body="$(OIDC_CLIENT_ID="${OIDC_CLIENT_ID}" \
    OIDC_CLIENT_SECRET="${OIDC_CLIENT_SECRET}" \
    OIDC_DISCOVERY="${OIDC_DISCOVERY}" \
    python3 - <<'PY'
import json, os
doc = {
  "uri": "/resource/*",
  "methods": ["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"],
  "upstream_id": "resource-backend",
  "plugins": {
    "proxy-rewrite": {
      "regex_uri": ["^/resource/(.*)", "/$1"]
    },
    "openid-connect": {
      "client_id": os.environ["OIDC_CLIENT_ID"],
      "client_secret": os.environ["OIDC_CLIENT_SECRET"],
      "discovery": os.environ["OIDC_DISCOVERY"],
      "bearer_only": True,
      "realm": "vpp",
      "use_jwks": True,
      "ssl_verify": False,
      "set_userinfo_header": True,
      "set_access_token_header": True,
      "access_token_in_authorization_header": True,
      "set_id_token_header": False,
    },
    "limit-req": {
      "rate": 30,
      "burst": 50,
      "key": "remote_addr",
      "rejected_code": 429,
    },
  },
}
print(json.dumps(doc))
PY
)"
  admin_curl -X PUT "${APISIX_ADMIN_URL}/apisix/admin/routes/resource-proxy" -d "${body}"
  echo
  echo "Route resource-proxy: /resource/* -> resource-backend (openid-connect bearer_only + limit-req)"
}

put_gateway_route() {
  admin_curl -X PUT "${APISIX_ADMIN_URL}/apisix/admin/routes/gateway-proxy" -d '{
    "uri": "/gateway/*",
    "priority": 0,
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
  echo "Route gateway-proxy: /gateway/* -> gateway-backend (key-auth + limit-req, priority 0)"
}

put_gateway_mappings_route() {
  # C10b: mappings use OIDC + X-Userinfo; higher priority than key-auth catch-all.
  local body
  body="$(OIDC_CLIENT_ID="${OIDC_CLIENT_ID}" \
    OIDC_CLIENT_SECRET="${OIDC_CLIENT_SECRET}" \
    OIDC_DISCOVERY="${OIDC_DISCOVERY}" \
    python3 - <<'PY'
import json, os
doc = {
  "uri": "/gateway/api/v1/tenants/*/mappings*",
  "priority": 100,
  "methods": ["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"],
  "upstream_id": "gateway-backend",
  "plugins": {
    "proxy-rewrite": {
      "regex_uri": ["^/gateway/(.*)", "/$1"]
    },
    "openid-connect": {
      "client_id": os.environ["OIDC_CLIENT_ID"],
      "client_secret": os.environ["OIDC_CLIENT_SECRET"],
      "discovery": os.environ["OIDC_DISCOVERY"],
      "bearer_only": True,
      "realm": "vpp",
      "use_jwks": True,
      "ssl_verify": False,
      "set_userinfo_header": True,
      "set_access_token_header": True,
      "access_token_in_authorization_header": True,
      "set_id_token_header": False,
    },
    "limit-req": {
      "rate": 30,
      "burst": 50,
      "key": "remote_addr",
      "rejected_code": 429,
    },
  },
}
print(json.dumps(doc))
PY
)"
  admin_curl -X PUT "${APISIX_ADMIN_URL}/apisix/admin/routes/gateway-mappings" -d "${body}"
  echo
  echo "Route gateway-mappings: /gateway/.../mappings* -> gateway-backend (openid-connect, priority 100)"
}

put_oidc_plugins_json() {
  # Shared Path C plugin block (no proxy-rewrite x-userinfo remove — see plan).
  OIDC_CLIENT_ID="${OIDC_CLIENT_ID}" \
    OIDC_CLIENT_SECRET="${OIDC_CLIENT_SECRET}" \
    OIDC_DISCOVERY="${OIDC_DISCOVERY}" \
    python3 - <<'PY'
import json, os
print(json.dumps({
  "openid-connect": {
    "client_id": os.environ["OIDC_CLIENT_ID"],
    "client_secret": os.environ["OIDC_CLIENT_SECRET"],
    "discovery": os.environ["OIDC_DISCOVERY"],
    "bearer_only": True,
    "realm": "vpp",
    "use_jwks": True,
    "ssl_verify": False,
    "set_userinfo_header": True,
    "set_access_token_header": True,
    "access_token_in_authorization_header": True,
    "set_id_token_header": False,
  },
  "limit-req": {
    "rate": 30,
    "burst": 50,
    "key": "remote_addr",
    "rejected_code": 429,
  },
}))
PY
}

put_dispatch_grpc_route() {
  # Northbound DispatchService via plaintext HTTP/2 :9081.
  # Relies on openid-connect set_userinfo_header overwrite (no proxy-rewrite remove).
  local plugins body
  plugins="$(put_oidc_plugins_json)"
  body="$(PLUGINS="${plugins}" python3 - <<'PY'
import json, os
doc = {
  "uri": "/dispatchpb.DispatchService/*",
  "methods": ["POST", "GET"],
  "upstream_id": "dispatch-grpc",
  "plugins": json.loads(os.environ["PLUGINS"]),
}
print(json.dumps(doc))
PY
)"
  admin_curl -X PUT "${APISIX_ADMIN_URL}/apisix/admin/routes/dispatch-grpc" -d "${body}"
  echo
  echo "Route dispatch-grpc: /dispatchpb.DispatchService/* -> dispatch-grpc (OIDC)"
}

put_telemetry_grpc_read_route() {
  # User-facing read RPCs only. IngestTelemetry stays machine→:5003 (no OIDC route).
  local plugins body
  plugins="$(put_oidc_plugins_json)"
  body="$(PLUGINS="${plugins}" python3 - <<'PY'
import json, os
doc = {
  "uris": [
    "/telemetrypb.TelemetryService/QueryTelemetry",
    "/telemetrypb.TelemetryService/GetSnapshot",
    "/telemetrypb.TelemetryService/GetFleetSnapshot",
    "/telemetrypb.TelemetryService/QueryAggregation",
  ],
  "methods": ["POST", "GET"],
  "upstream_id": "telemetry-grpc",
  "plugins": json.loads(os.environ["PLUGINS"]),
}
print(json.dumps(doc))
PY
)"
  admin_curl -X PUT "${APISIX_ADMIN_URL}/apisix/admin/routes/telemetry-grpc-read" -d "${body}"
  echo
  echo "Route telemetry-grpc-read: TelemetryService read RPCs -> telemetry-grpc (OIDC; Ingest excluded)"
}

main() {
  wait_for_admin

  put_upstream "gateway-backend" "host.docker.internal:8083"
  put_upstream "resource-backend" "host.docker.internal:8082"
  put_upstream "dispatch-grpc" "host.docker.internal:5006" "grpc"
  put_upstream "telemetry-grpc" "host.docker.internal:5003" "grpc"

  put_consumer_key_auth "${SIMULATOR_CONSUMER}" "${SIMULATOR_API_KEY}"

  put_gateway_route
  put_gateway_mappings_route
  put_resource_route
  put_dispatch_grpc_route
  put_telemetry_grpc_read_route

  # Drop Gate 0 temporary route id if it still exists from earlier init.
  "${CURL[@]}" -s -o /dev/null -w '' \
    -X DELETE "${APISIX_ADMIN_URL}/apisix/admin/routes/dispatch-grpc-gate0" \
    -H "X-API-KEY: ${APISIX_ADMIN_KEY}" || true

  echo ""
  echo "Phase 0+1+2(+C10b)+gRPC-OIDC routes installed."
  echo ""
  echo "Smoke test:"
  echo "  # EMS ingest still needs API key (not OIDC)"
  echo "  curl --noproxy '*' -s -o /dev/null -w '%{http_code}\\n' \\"
  echo "    http://127.0.0.1:9080/gateway/api/v1/tenants/default/telemetry:ingest"
  echo "  # mappings without Bearer → 401 (OIDC)"
  echo "  curl --noproxy '*' -s -o /dev/null -w '%{http_code}\\n' \\"
  echo "    http://127.0.0.1:9080/gateway/api/v1/tenants/default/mappings"
  echo "  # mappings with Casdoor token"
  echo "  curl --noproxy '*' -s -o /dev/null -w '%{http_code}\\n' \\"
  echo "    -H \"Authorization: Bearer \$(make -s casdoor-token)\" \\"
  echo "    http://127.0.0.1:9080/gateway/api/v1/tenants/default/mappings"
  echo "  # resource 401 without Bearer"
  echo "  curl --noproxy '*' -s -o /dev/null -w '%{http_code}\\n' \\"
  echo "    http://127.0.0.1:9080/resource/api/tenants/default/sites"
  echo "  # gRPC GetTask without Bearer (expect reject on :9081)"
  echo "  grpcurl -plaintext 127.0.0.1:9081 dispatchpb.DispatchService/GetTask"
  echo "  # Gate 0 / gRPC OIDC probe: make apisix-gate0-probe"
}

main "$@"
