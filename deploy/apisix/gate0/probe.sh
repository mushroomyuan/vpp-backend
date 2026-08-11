#!/usr/bin/env bash
# Gate 0 probe: APISIX plaintext HTTP/2 + dispatch gRPC + openid-connect.
#
# Prerequisites:
#   make casdoor-up && make casdoor-init
#   make apisix-up && make apisix-init
#   DISPATCH_AUTH_TRUST_PROXY_HEADERS=true make run-dispatch
#     (or a secured config; PEP must be on to assert x-userinfo injection)
#
# Usage (repo root):
#   bash deploy/apisix/gate0/probe.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "${ROOT}"

APISIX_GRPC="${APISIX_GRPC:-127.0.0.1:9081}"
PROTO_DIR="${ROOT}/api/dispatch/proto"
PROTO_FILE="dispatch.proto"
GRPCURL=(grpcurl -plaintext -import-path "${PROTO_DIR}" -proto "${PROTO_FILE}")

# Forged admin identity (must NOT win when a viewer Bearer is presented).
FORGED_ADMIN_B64="$(python3 - <<'PY'
import base64, json
print(base64.b64encode(json.dumps({
  "owner": "default",
  "name": "forged",
  "roles": [{"name": "admin"}],
}).encode()).decode())
PY
)"

pass() { echo "PASS: $*"; }
fail() { echo "FAIL: $*" >&2; exit 1; }
info() { echo "==> $*"; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing command: $1"
}

need_cmd grpcurl
need_cmd make
need_cmd python3
need_cmd go

info "fetch Casdoor tokens"
TOKEN_ADMIN="$(make -s casdoor-token USER=admin)"
TOKEN_VIEWER="$(make -s casdoor-token USER=viewer)"
[[ -n "${TOKEN_ADMIN}" ]] || fail "empty admin token"
[[ -n "${TOKEN_VIEWER}" ]] || fail "empty viewer token"

info "1) grpcurl: no Bearer → must be rejected by APISIX"
set +e
OUT="$("${GRPCURL[@]}" -d '{"TenantID":"default","TaskID":"gate0"}' \
  "${APISIX_GRPC}" dispatchpb.DispatchService/GetTask 2>&1)"
RC=$?
set -e
if [[ ${RC} -eq 0 ]]; then
  fail "expected rejection without Bearer; got success: ${OUT}"
fi
pass "grpcurl without Bearer rejected (rc=${RC})"
echo "     ${OUT}" | head -n 5 | sed 's/^/     /'

info "2) grpcurl: admin Bearer + GetTask → must pass APISIX (backend may NotFound)"
set +e
OUT="$("${GRPCURL[@]}" \
  -H "authorization: Bearer ${TOKEN_ADMIN}" \
  -d '{"TenantID":"default","TaskID":"gate0-missing"}' \
  "${APISIX_GRPC}" dispatchpb.DispatchService/GetTask 2>&1)"
RC=$?
set -e
# Success OR a gRPC status from backend both mean OIDC let the call through.
if echo "${OUT}" | grep -qiE 'WWW-Authenticate|oidc|unauthorized|401'; then
  fail "looks like APISIX OIDC still rejecting with valid token: ${OUT}"
fi
if echo "${OUT}" | grep -qiE 'Unavailable|connection refused|reset by peer'; then
  fail "upstream/APISIX transport problem: ${OUT}"
fi
pass "grpcurl with admin Bearer reached auth path (rc=${RC})"
echo "     ${OUT}" | head -n 8 | sed 's/^/     /'

info "3) grpcurl: viewer Bearer + forged admin x-userinfo + SubmitTask → expect PermissionDenied (overwrite)"
set +e
OUT="$("${GRPCURL[@]}" \
  -H "authorization: Bearer ${TOKEN_VIEWER}" \
  -H "x-userinfo: ${FORGED_ADMIN_B64}" \
  -d '{"TenantID":"default","Name":"gate0","TaskType":"control","TriggerType":"manual"}' \
  "${APISIX_GRPC}" dispatchpb.DispatchService/SubmitTask 2>&1)"
RC=$?
set -e
if echo "${OUT}" | grep -qiE 'PermissionDenied|permission denied|forbidden|role cannot'; then
  pass "forged x-userinfo did not elevate viewer (PermissionDenied)"
elif [[ ${RC} -eq 0 ]]; then
  fail "SubmitTask succeeded for viewer+forged-admin — overwrite likely broken: ${OUT}"
else
  # Missing x-userinfo would be Unauthenticated — that means injection failed.
  if echo "${OUT}" | grep -qiE 'Unauthenticated|missing or invalid x-userinfo'; then
    fail "backend missing x-userinfo — OIDC injection failed: ${OUT}"
  fi
  echo "WARN: unexpected SubmitTask error (inspect manually): ${OUT}"
  fail "could not confirm overwrite semantics"
fi

info "4) grpc-go: no Bearer rejection semantics"
(
  cd "${ROOT}/internal/dispatch"
  go run ./cmd/gate0client -addr "${APISIX_GRPC}" -mode no-auth
) || fail "grpc-go no-auth probe failed"
pass "grpc-go no-auth probe completed"

info "5) grpc-go: viewer + forged admin SubmitTask"
(
  cd "${ROOT}/internal/dispatch"
  go run ./cmd/gate0client \
    -addr "${APISIX_GRPC}" \
    -mode submit \
    -token "${TOKEN_VIEWER}" \
    -forge-admin-userinfo
) || fail "grpc-go submit probe failed"
pass "grpc-go submit probe completed"

echo ""
echo "Gate 0 probe finished. Review outputs above before enabling trust-proxy by default."
echo "Note: :9081 is plaintext h2c — localhost only; use :9443 TLS for cross-host."
