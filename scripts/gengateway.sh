#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

PROTO_ROOT="api/resource/proto"
ENTRY_PROTO="${PROTO_ROOT}/root.proto"
MODULE_GLOB="${PROTO_ROOT}/modules/*.proto"
THIRD_PARTY_ROOT="${PROTO_ROOT}/third_party"
PROTO_INCLUDE="/usr/local/include"

GO_OUT_DIR="${PROTO_ROOT}/gen"
OPENAPI_OUT_DIR="${GO_OUT_DIR}/openapi"

BIN_DIR="${ROOT_DIR}/.bin"

log() { printf '[gengateway] %s\n' "$*"; }
die() { printf '[gengateway] error: %s\n' "$*" >&2; exit 1; }

require_bin() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

require_file() {
  [[ -f "$1" ]] || die "required file not found: $1"
}

install_plugins() {
  mkdir -p "${BIN_DIR}"
  export GOBIN="${BIN_DIR}"

  log "installing protoc plugins into ${BIN_DIR}"
  log "note: if your Go uses snap packaging and fails, install these plugins via your system package manager or a non-snap Go toolchain"
  go install "github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@v2.27.1"
  go install "github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@v2.27.1"
}

generate() {
  mkdir -p "${GO_OUT_DIR}" "${OPENAPI_OUT_DIR}"

  local proto_files=("${ENTRY_PROTO}" ${MODULE_GLOB})

  log "generating grpc-gateway reverse proxy stubs"
  protoc \
    -I="${PROTO_ROOT}" \
    -I="${PROTO_ROOT}/modules" \
    -I="${THIRD_PARTY_ROOT}" \
    -I="${PROTO_INCLUDE}" \
    --grpc-gateway_out="${GO_OUT_DIR}" \
    --grpc-gateway_opt=paths=source_relative \
    "${ENTRY_PROTO}"

  # Flatten possible nested output from go_package import path.
  local nested="${GO_OUT_DIR}/github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
  if [[ -d "${nested}" ]]; then
    mv "${nested}/"*.go "${GO_OUT_DIR}/" || true
    rm -rf "${GO_OUT_DIR}/github.com"
  fi

  log "generating OpenAPI (swagger) from proto annotations"
  protoc \
    -I="${PROTO_ROOT}" \
    -I="${PROTO_ROOT}/modules" \
    -I="${THIRD_PARTY_ROOT}" \
    -I="${PROTO_INCLUDE}" \
    --openapiv2_out="${OPENAPI_OUT_DIR}" \
    --openapiv2_opt=logtostderr=true,allow_merge=true,merge_file_name=resource \
    "${ENTRY_PROTO}"

  log "gateway/openapi generation succeeded"
}

require_bin protoc
require_bin go
require_file "${ENTRY_PROTO}"
require_file "${PROTO_INCLUDE}/google/protobuf/empty.proto"
require_file "${THIRD_PARTY_ROOT}/google/api/annotations.proto"

install_plugins
export PATH="${BIN_DIR}:${PATH}"

require_bin protoc-gen-grpc-gateway
require_bin protoc-gen-openapiv2

generate

