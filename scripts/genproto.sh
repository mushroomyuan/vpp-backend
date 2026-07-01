#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

PROTO_INCLUDE="/usr/local/include"

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

generate_with_buf() {
  local dir=$1
  log "buf generate in ${dir}"
  (
    cd "${dir}"
    if [[ ! -f buf.lock ]]; then
      log "buf.lock missing in ${dir}, running buf dep update"
      buf dep update
    fi
    buf generate
  )
}

flatten_resource_gen() {
  "${ROOT_DIR}/scripts/flatten_resource_gen.sh"
}

generate_resource_with_protoc() {
  local proto_root="api/resource/proto"
  local entry_proto="${proto_root}/resource_service.proto"
  local module_glob="${proto_root}/modules/*.proto"
  local out_dir="${proto_root}/gen"
  local third_party_root="${proto_root}/third_party"

  mkdir -p "${out_dir}"
  rm -f "${out_dir}"/*.pb.go "${out_dir}"/*_grpc.pb.go "${out_dir}"/*.pb.gw.go
  rm -rf "${out_dir}/modules"

  log "generating resource Go protobuf files to ${out_dir}"

  protoc \
    -I="${proto_root}" \
    -I="${third_party_root}" \
    -I="${PROTO_INCLUDE}" \
    --go_out="${out_dir}" --go_opt=paths=source_relative \
    --go-grpc_out="${out_dir}" --go-grpc_opt=paths=source_relative --go-grpc_opt=require_unimplemented_servers=false \
    "${entry_proto}" ${module_glob}

  local nested="${out_dir}/github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
  if [[ -d "${nested}" ]]; then
    mv "${nested}/"*.go "${out_dir}/" || true
    rm -rf "${out_dir}/github.com"
  fi

  if [[ -d "${out_dir}/modules" ]]; then
    mv "${out_dir}/modules/"*.pb.go "${out_dir}/" || true
    rmdir "${out_dir}/modules" 2>/dev/null || true
  fi
}

require_file "${PROTO_INCLUDE}/google/protobuf/empty.proto"

if command -v buf >/dev/null 2>&1; then
  generate_with_buf "api/resource/proto"
  flatten_resource_gen
  generate_with_buf "api/telemetry/proto"
  generate_with_buf "api/gateway/proto"
else
  log "buf not found, falling back to protoc for resource only"
  require_bin protoc
  require_bin protoc-gen-go
  require_bin protoc-gen-go-grpc
  require_file "api/resource/proto/resource_service.proto"
  generate_resource_with_protoc
  die "telemetry and gateway proto generation requires buf; install buf or run buf generate manually"
fi

log "proto generation succeeded"
