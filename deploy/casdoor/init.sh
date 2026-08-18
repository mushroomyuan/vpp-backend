#!/usr/bin/env bash
# Idempotent Casdoor bootstrap helpers for C0.
#
# - Ensures Postgres database `casdoor` exists (existing volumes skip initdb/).
# - Waits for Casdoor HTTP readiness.
# - Verifies seed via Admin API (session login as built-in admin) and/or SQL.
#
# First-boot identity seed comes from conf/init_data.json (initDataNewOnly=true).
# Re-running does not wipe the DB. It verifies seed and upserts vpp-resource
# tokenFormat=JWT-Custom (slim C3 claims) so existing volumes pick up the
# access_token size fix without drop/reimport.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CASDOOR_URL="${CASDOOR_URL:-http://127.0.0.1:8000}"
POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-}"
# Casdoor auto-creates built-in admin for management API / console.
ADMIN_ORG="${CASDOOR_ADMIN_ORG:-built-in}"
ADMIN_USER="${CASDOOR_ADMIN_USER:-admin}"
ADMIN_PASS="${CASDOOR_ADMIN_PASS:-123}"
ADMIN_APP="${CASDOOR_ADMIN_APP:-app-built-in}"
COOKIE_JAR="$(mktemp)"
trap 'rm -f "${COOKIE_JAR}"' EXIT

CURL=(curl --noproxy '*')

resolve_postgres() {
  if [[ -n "${POSTGRES_CONTAINER}" ]]; then
    echo "${POSTGRES_CONTAINER}"
    return
  fi
  docker ps --format '{{.Names}}' | grep -E 'postgres' | head -1
}

ensure_db() {
  local c
  c="$(resolve_postgres)"
  if [[ -z "${c}" ]]; then
    echo "ERROR: Postgres container is not running. Run: make infra-up" >&2
    return 1
  fi
  POSTGRES_CONTAINER="${c}"
  local exists
  exists="$(docker exec "${c}" psql -U postgres -Atc \
    "SELECT 1 FROM pg_database WHERE datname='casdoor'" || true)"
  if [[ "${exists}" != "1" ]]; then
    echo "Creating database casdoor..."
    docker exec "${c}" psql -U postgres -c "CREATE DATABASE casdoor;"
  else
    echo "Database casdoor already exists."
  fi
}

wait_for_casdoor() {
  local attempts="${1:-45}"
  local i code
  for ((i = 1; i <= attempts; i++)); do
    code="$("${CURL[@]}" -s -o /dev/null -w '%{http_code}' \
      "${CASDOOR_URL}/api/health" --connect-timeout 2 || true)"
    if [[ "${code}" == "200" ]]; then
      echo "Casdoor is ready at ${CASDOOR_URL}"
      return 0
    fi
    code="$("${CURL[@]}" -s -o /dev/null -w '%{http_code}' \
      "${CASDOOR_URL}/.well-known/openid-configuration" --connect-timeout 2 || true)"
    if [[ "${code}" == "200" ]]; then
      echo "Casdoor OIDC discovery is ready at ${CASDOOR_URL}"
      return 0
    fi
    echo "Waiting for Casdoor (${i}/${attempts}, last_http=${code:-none})..."
    sleep 2
  done
  echo "ERROR: Casdoor not reachable at ${CASDOOR_URL}" >&2
  return 1
}

login_admin() {
  local body status
  body="$("${CURL[@]}" -sS -c "${COOKIE_JAR}" -b "${COOKIE_JAR}" \
    -X POST "${CASDOOR_URL}/api/login" \
    -H 'Content-Type: application/json' \
    -d "{\"application\":\"${ADMIN_APP}\",\"organization\":\"${ADMIN_ORG}\",\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASS}\",\"type\":\"login\",\"autoSignin\":true}")"
  status="$(echo "${body}" | python3 -c 'import sys,json; print(json.load(sys.stdin).get("status",""))' 2>/dev/null || true)"
  if [[ "${status}" != "ok" ]]; then
    echo "ERROR: built-in admin login failed: ${body}" >&2
    return 1
  fi
  echo "OK logged in as ${ADMIN_ORG}/${ADMIN_USER}"
}

api_get() {
  local path=$1
  "${CURL[@]}" -sfS -b "${COOKIE_JAR}" -c "${COOKIE_JAR}" \
    "${CASDOOR_URL}${path}"
}

api_post_json() {
  local path=$1
  "${CURL[@]}" -sfS -b "${COOKIE_JAR}" -c "${COOKIE_JAR}" \
    -X POST "${CASDOOR_URL}${path}" \
    -H 'Content-Type: application/json' \
    --data-binary @-
}

# initDataNewOnly=true will not reimport init_data.json. Patch the live
# application so existing Postgres volumes get slim access_tokens.
ensure_jwt_custom() {
  local wrap payload status
  echo "Ensuring vpp-resource tokenFormat=JWT-Custom..."
  wrap="$(api_get "/api/get-application?id=admin/vpp-resource")"
  payload="$(WRAP="${wrap}" python3 - <<'PY'
import json, os, sys
wrap = json.loads(os.environ["WRAP"])
app = wrap.get("data") if isinstance(wrap, dict) and "data" in wrap else wrap
if not isinstance(app, dict) or not app.get("name"):
    sys.stderr.write("ERROR: unexpected get-application body\n")
    sys.exit(1)
wanted_format = "JWT-Custom"
wanted_fields = ["Owner", "Name", "Id", "IsAdmin", "Roles"]
if app.get("tokenFormat") == wanted_format and app.get("tokenFields") == wanted_fields:
    print("UNCHANGED")
    raise SystemExit(0)
app["tokenFormat"] = wanted_format
app["tokenFields"] = wanted_fields
json.dump(app, sys.stdout)
PY
)"
  if [[ "${payload}" == "UNCHANGED" ]]; then
    echo "OK application already JWT-Custom (Owner/Name/Id/IsAdmin/Roles)"
    return 0
  fi
  status="$(echo "${payload}" | api_post_json "/api/update-application?id=admin/vpp-resource" \
    | python3 -c 'import sys,json; print(json.load(sys.stdin).get("status",""))' 2>/dev/null || true)"
  if [[ "${status}" != "ok" ]]; then
    echo "ERROR: update-application failed (status=${status:-none})" >&2
    return 1
  fi
  echo "OK patched application tokenFormat=JWT-Custom"
}

verify_seed_api() {
  local body
  echo "Verifying seed via Admin API..."
  body="$(api_get "/api/get-organization?id=admin/default")"
  if echo "${body}" | grep -q '"name"[ ]*:[ ]*"default"'; then
    echo "OK organization default"
  else
    echo "ERROR: organization default missing: $(echo "${body}" | head -c 240)" >&2
    return 1
  fi

  body="$(api_get "/api/get-application?id=admin/vpp-resource")"
  if echo "${body}" | grep -q 'vpp-resource-dev-client'; then
    echo "OK application vpp-resource (client_id)"
  else
    echo "ERROR: application vpp-resource missing/unexpected: $(echo "${body}" | head -c 240)" >&2
    return 1
  fi
  if echo "${body}" | grep -q '"password"'; then
    echo "OK grantTypes includes password"
  else
    echo "ERROR: grantTypes missing password — Password Grant will fail." >&2
    return 1
  fi
  if echo "${body}" | grep -q '"JWT-Custom"'; then
    echo "OK application tokenFormat=JWT-Custom"
  else
    echo "ERROR: application tokenFormat is not JWT-Custom (admin JWT will blow APISIX header buffers)." >&2
    return 1
  fi

  body="$(api_get "/api/get-user?id=default/admin")"
  if echo "${body}" | grep -q '"name"[ ]*:[ ]*"admin"'; then
    echo "OK user default/admin"
  else
    echo "ERROR: user default/admin missing: $(echo "${body}" | head -c 240)" >&2
    return 1
  fi

  body="$(api_get "/api/get-cert?id=admin/cert-vpp")"
  if echo "${body}" | grep -q 'cert-vpp'; then
    echo "OK cert cert-vpp"
  else
    echo "WARN: cert cert-vpp not visible via API (may still be in DB)"
  fi
}

verify_seed_sql() {
  local c
  c="$(resolve_postgres)"
  [[ -n "${c}" ]] || return 0
  echo "Verifying seed via SQL..."
  docker exec "${c}" psql -U postgres -d casdoor -Atc \
    "SELECT name FROM organization WHERE name='default'" | grep -qx default
  docker exec "${c}" psql -U postgres -d casdoor -Atc \
    "SELECT name FROM application WHERE name='vpp-resource'" | grep -qx vpp-resource
  docker exec "${c}" psql -U postgres -d casdoor -Atc \
    "SELECT count(*) FROM \"user\" WHERE owner='default'" | grep -Eq '^[3-9]'
  echo "OK SQL seed (org + app + >=3 default users)"
}

smoke_password_grant() {
  local body token
  echo "Smoke: Password Grant for default/admin..."
  body="$("${CURL[@]}" -sS -X POST "${CASDOOR_URL}/api/login/oauth/access_token" \
    -d 'grant_type=password' \
    -d 'client_id=vpp-resource-dev-client' \
    -d 'client_secret=vpp-resource-dev-secret' \
    -d 'username=admin' \
    -d 'password=vpp-admin-dev')"
  if ! echo "${body}" | grep -q 'access_token'; then
    # Casdoor sometimes requires org-qualified username.
    body="$("${CURL[@]}" -sS -X POST "${CASDOOR_URL}/api/login/oauth/access_token" \
      -d 'grant_type=password' \
      -d 'client_id=vpp-resource-dev-client' \
      -d 'client_secret=vpp-resource-dev-secret' \
      -d 'username=admin' \
      -d 'password=vpp-admin-dev' \
      -d 'organization=default')"
  fi
  if ! echo "${body}" | grep -q 'access_token'; then
    echo "WARN: Password Grant failed (C1 will harden make casdoor-token): $(echo "${body}" | head -c 300)"
    return 0
  fi
  echo "OK Password Grant returned access_token"
  token="$(echo "${body}" | python3 -c '
import json,sys
d=json.load(sys.stdin)
print(d.get("access_token") or d.get("accessToken") or "")
')"
  TOKEN="${token}" python3 - <<'PY'
import base64, json, os, sys

token = os.environ["TOKEN"]
limit = 4096
if len(token) > limit:
    print(f"ERROR: admin access_token is {len(token)} bytes (limit {limit}). JWT-Custom not applied?", file=sys.stderr)
    sys.exit(1)

def b64url(s: str):
    s += "=" * ((4 - len(s) % 4) % 4)
    return json.loads(base64.urlsafe_b64decode(s.encode()))

parts = token.split(".")
if len(parts) < 2:
    print("ERROR: access_token is not a JWT", file=sys.stderr)
    sys.exit(1)
payload = b64url(parts[1])
if "permissions" in payload:
    print("ERROR: access_token still embeds permissions (fat JWT)", file=sys.stderr)
    sys.exit(1)
if not payload.get("owner") or not payload.get("name") or payload.get("roles") is None:
    print("ERROR: slim JWT missing owner/name/roles needed by C3", file=sys.stderr)
    sys.exit(1)
print(f"OK slim access_token ({len(token)} bytes, no permissions claim)")
PY
}

print_next() {
  cat <<EOF

C0 ready.
  UI:        ${CASDOOR_URL}
  Discovery: ${CASDOOR_URL}/.well-known/openid-configuration
  Creds:     ${SCRIPT_DIR}/conf/credentials.yaml
  Next:      C1 — make casdoor-token + JWT claim decode
EOF
}

main() {
  ensure_db
  wait_for_casdoor
  verify_seed_sql
  login_admin
  ensure_jwt_custom
  verify_seed_api
  smoke_password_grant
  print_next
}

main "$@"
