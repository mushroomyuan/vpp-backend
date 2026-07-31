#!/usr/bin/env bash
# Password Grant → print access_token (default) or decode JWT payload.
#
# Usage:
#   ./deploy/casdoor/token.sh                       # token only (admin)
#   CASDOOR_USER=operator ./deploy/casdoor/token.sh
#   ./deploy/casdoor/token.sh --decode              # JWT payload + C3 mapping
#   make casdoor-token USER=viewer DECODE=1
#
# Env (optional):
#   CASDOOR_URL, CLIENT_ID, CLIENT_SECRET, CASDOOR_USER, PASSWORD
# Note: do NOT use shell USER= (clashes with OS login name).
set -euo pipefail

CASDOOR_URL="${CASDOOR_URL:-http://127.0.0.1:8000}"
CLIENT_ID="${CLIENT_ID:-vpp-resource-dev-client}"
CLIENT_SECRET="${CLIENT_SECRET:-vpp-resource-dev-secret}"
CASDOOR_USER="${CASDOOR_USER:-admin}"
DECODE="${DECODE:-0}"

# Map CASDOOR_USER → password from credentials.yaml defaults when PASSWORD unset.
if [[ -z "${PASSWORD:-}" ]]; then
  case "${CASDOOR_USER}" in
    admin) PASSWORD="vpp-admin-dev" ;;
    operator) PASSWORD="vpp-operator-dev" ;;
    viewer) PASSWORD="vpp-viewer-dev" ;;
    *)
      echo "ERROR: unknown CASDOOR_USER=${CASDOOR_USER}; set PASSWORD=... explicitly" >&2
      exit 1
      ;;
  esac
fi

for arg in "$@"; do
  case "${arg}" in
    --decode|-d) DECODE=1 ;;
    --help|-h)
      sed -n '2,14p' "$0" | sed 's/^# \?//'
      exit 0
      ;;
  esac
done

CURL=(curl --noproxy '*')

body="$("${CURL[@]}" -sS -X POST "${CASDOOR_URL}/api/login/oauth/access_token" \
  -d "grant_type=password" \
  -d "client_id=${CLIENT_ID}" \
  -d "client_secret=${CLIENT_SECRET}" \
  -d "username=${CASDOOR_USER}" \
  -d "password=${PASSWORD}")"

token="$(echo "${body}" | python3 -c '
import sys, json
d = json.load(sys.stdin)
t = d.get("access_token") or d.get("accessToken") or ""
if not t:
    sys.stderr.write("ERROR: no access_token in response: %s\n" % (d,))
    sys.exit(1)
print(t)
')"

if [[ "${DECODE}" != "1" ]]; then
  # stdout: token only (safe for $(make -s casdoor-token))
  printf '%s' "${token}"
  exit 0
fi

python3 - "${token}" <<'PY'
import base64, json, sys

tok = sys.argv[1]
parts = tok.split(".")
if len(parts) < 2:
    print("ERROR: not a JWT", file=sys.stderr)
    sys.exit(1)

def b64url(s: str):
    s += "=" * ((4 - len(s) % 4) % 4)
    return json.loads(base64.urlsafe_b64decode(s.encode()))

header = b64url(parts[0])
payload = b64url(parts[1])

# Compact identity summary for C3 middleware mapping.
roles = payload.get("roles") or []
role_names = []
if isinstance(roles, list):
    for r in roles:
        if isinstance(r, dict) and r.get("name"):
            role_names.append(r["name"])
        elif isinstance(r, str):
            role_names.append(r)

summary = {
    "mapping_for_c3": {
        "user_id": payload.get("sub") or payload.get("id"),
        "tenant_id": payload.get("owner"),
        "username": payload.get("name"),
        "roles": role_names,
        "roles_raw_type": type(roles).__name__,
        "is_admin": payload.get("isAdmin"),
    },
    "header": header,
    "payload": payload,
}
print(json.dumps(summary, indent=2, ensure_ascii=False))
PY
