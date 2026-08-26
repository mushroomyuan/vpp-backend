#!/usr/bin/env bash
# ROADMAP / kind 混合拓扑：业务进程不再听在 host 的 :808x / :500x。
# 外部入口是 APISIX :9080/:9081；APISIX→kind 仍走 extraPortMappings 的 NodePort。
set -euo pipefail

pass=0
fail=0

listening() {
  local port=$1
  ss -ltn 2>/dev/null | grep -qE ":${port}[[:space:]]"
}

expect_closed() {
  local port=$1 name=$2
  if listening "${port}"; then
    echo "FAIL  host :${port} LISTEN  (${name} should have left the host)"
    fail=$((fail + 1))
  else
    echo "OK    host :${port} closed  (${name})"
    pass=$((pass + 1))
  fi
}

expect_listen() {
  local port=$1 name=$2
  if listening "${port}"; then
    echo "OK    host :${port} LISTEN  (${name})"
    pass=$((pass + 1))
  else
    echo "FAIL  host :${port} closed  (${name} should be up)"
    fail=$((fail + 1))
  fi
}

http_code() {
  local c
  c="$(curl --noproxy '*' -s -o /dev/null -w '%{http_code}' --connect-timeout 2 "$1" 2>/dev/null || true)"
  echo "${c:-000}"
}

echo "== old make run-* ports must not be on the host =="
expect_closed 8082 "resource HTTP"
expect_closed 8083 "gateway HTTP"
expect_closed 8084 "simulator HTTP"
expect_closed 8087 "alarm HTTP"
expect_closed 5002 "resource gRPC"
expect_closed 5003 "telemetry gRPC"
expect_closed 5005 "gateway gRPC"
expect_closed 5006 "dispatch gRPC"

echo
echo "== unused NodePorts must not be extraPortMapped =="
expect_closed 30502 "resource gRPC nodePort (in-cluster only)"
expect_closed 30505 "gateway gRPC nodePort (in-cluster only)"

echo
echo "== northbound still on the host (APISIX + kind hairpin) =="
expect_listen 9080 "APISIX HTTP"
expect_listen 9081 "APISIX gRPC h2c"
expect_listen 30082 "kind NodePort resource HTTP"
expect_listen 30083 "kind NodePort gateway HTTP"
expect_listen 30003 "kind NodePort telemetry gRPC"
expect_listen 30006 "kind NodePort dispatch gRPC"

echo
echo "== traffic =="
code=$(http_code http://127.0.0.1:8082/healthz)
if [[ "${code}" == "000" ]]; then
  echo "OK    curl :8082/healthz → connection failed"
  pass=$((pass + 1))
else
  echo "FAIL  curl :8082/healthz → HTTP ${code} (host process still there?)"
  fail=$((fail + 1))
fi

code=$(http_code http://127.0.0.1:30082/healthz)
if [[ "${code}" == "200" ]]; then
  echo "OK    curl :30082/healthz → 200 (kind NodePort; bypasses APISIX)"
  pass=$((pass + 1))
else
  echo "FAIL  curl :30082/healthz → ${code} (expected 200 from resource Pod)"
  fail=$((fail + 1))
fi

code=$(http_code http://127.0.0.1:9080/resource/api/tenants/default/sites)
if [[ "${code}" == "401" ]]; then
  echo "OK    curl APISIX /resource → 401 (OIDC gate, no Bearer)"
  pass=$((pass + 1))
else
  echo "FAIL  curl APISIX /resource → ${code} (expected 401 without Bearer)"
  fail=$((fail + 1))
fi

echo
echo "passed=${pass} failed=${fail}"
echo
echo "Remaining surface: host :300xx is kind extraPortMappings for compose APISIX."
echo "Direct :30082 still reaches the Pod and skips APISIX plugins."
echo "That goes away when APISIX moves in-cluster and extraPortMappings are removed."

exit "${fail}"
