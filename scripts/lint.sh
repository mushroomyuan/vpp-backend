#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

MODULES=(platform resource telemetry gateway dispatch simulator alarm)

log() {
  printf '[lint] %s\n' "$*"
}

die() {
  printf '[lint] error: %s\n' "$*" >&2
  exit 1
}

command -v golangci-lint >/dev/null 2>&1 || die "golangci-lint not found. Install: https://golangci-lint.run/welcome/install/"

failed=0
for module in "${MODULES[@]}"; do
  dir="internal/${module}"
  log "golangci-lint run (${dir})"
  if ! (cd "${dir}" && golangci-lint run --config "${ROOT_DIR}/.golangci.yml" --timeout=5m ./...); then
    log "FAILED: ${dir}"
    failed=1
  fi
done

if [[ ${failed} -ne 0 ]]; then
  die "lint failed for one or more modules"
fi

log "lint passed for all modules"
