#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

PROTO_ROOT="api/resource/proto"
ENTRY_PROTO="${PROTO_ROOT}/root.proto"
MODULE_GLOB="${PROTO_ROOT}/modules/*.proto"
OUT_DIR="${PROTO_ROOT}/gen"
PROTO_INCLUDE="/usr/local/include"
THIRD_PARTY_ROOT="${PROTO_ROOT}/third_party"
BUF_DIR="${PROTO_ROOT}"

log() {
  printf '[genproto] %s\n' "$*"
}

die() {
  printf '[genproto] error: %s\n' "$*" >&2
  exit 1
}

require_file() {
  local path=$1
  [[ -f "${path}" ]] || die "required file not found: ${path}"
}

require_bin() {
  local bin=$1
  command -v "${bin}" >/dev/null 2>&1 || die "required command not found: ${bin}"
}

generate_with_protoc() {
  mkdir -p "${OUT_DIR}"
  rm -f "${OUT_DIR}"/*.pb.go "${OUT_DIR}"/*_grpc.pb.go
  rm -rf "${OUT_DIR}/modules"

  log "generating Go protobuf files to ${OUT_DIR}"

  # Generate service/root definitions (root.proto imports modules/*.proto)
  protoc \
    -I="${PROTO_ROOT}" \
    -I="${THIRD_PARTY_ROOT}" \
    -I="${PROTO_INCLUDE}" \
    --go_out="${OUT_DIR}" --go_opt=paths=source_relative \
    --go-grpc_out="${OUT_DIR}" --go-grpc_opt=paths=source_relative --go-grpc_opt=require_unimplemented_servers=false \
    "${ENTRY_PROTO}"

  # Generate module messages into OUT_DIR root.
  protoc \
    -I="${PROTO_ROOT}" \
    -I="${THIRD_PARTY_ROOT}" \
    -I="${PROTO_INCLUDE}" \
    --go_out="${OUT_DIR}" --go_opt=paths=source_relative \
    --go-grpc_out="${OUT_DIR}" --go-grpc_opt=paths=source_relative --go-grpc_opt=require_unimplemented_servers=false \
    ${MODULE_GLOB}

  # protoc-gen-go may write under OUT_DIR/<go_package import path>/...; flatten into OUT_DIR.
  local nested="${OUT_DIR}/github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
  if [[ -d "${nested}" ]]; then
    mv "${nested}/"*.go "${OUT_DIR}/" || true
    rm -rf "${OUT_DIR}/github.com"
  fi

  # Move module outputs into OUT_DIR so the generated code is a single Go package.
  if [[ -d "${OUT_DIR}/modules" ]]; then
    mv "${OUT_DIR}/modules/"*.pb.go "${OUT_DIR}/" || true
    rmdir "${OUT_DIR}/modules" 2>/dev/null || true
  fi
}

require_file "${ENTRY_PROTO}"
require_file "${PROTO_INCLUDE}/google/protobuf/empty.proto"

log "using entry proto: ${ENTRY_PROTO}"
log "writing generated files to: ${OUT_DIR}"

if command -v buf >/dev/null 2>&1; then
  log "buf detected, generating via buf generate"
  (
    cd "${BUF_DIR}"
    if [[ ! -f buf.lock ]]; then
      log "buf.lock missing, running buf dep update"
      buf dep update
    fi
    buf generate
  )
else
  log "buf not found, falling back to protoc"
  require_bin protoc
  require_bin protoc-gen-go
  require_bin protoc-gen-go-grpc
  generate_with_protoc
fi

log "proto generation succeeded"