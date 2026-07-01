#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
gen_dir="${ROOT_DIR}/api/resource/proto/gen"

if [[ ! -d "${gen_dir}/modules" ]]; then
  exit 0
fi

printf '[flatten_resource_gen] flattening %s/modules into %s\n' "${gen_dir}" "${gen_dir}"

# Remove stale module message files left from older flattened outputs.
rm -f \
  "${gen_dir}/asset.pb.go" \
  "${gen_dir}/common.pb.go" \
  "${gen_dir}/cu.pb.go" \
  "${gen_dir}/error.pb.go" \
  "${gen_dir}/job.pb.go" \
  "${gen_dir}/point.pb.go" \
  "${gen_dir}/resource.pb.go" \
  "${gen_dir}/runtime.pb.go" \
  "${gen_dir}/site.pb.go"

mv "${gen_dir}/modules/"*.pb.go "${gen_dir}/"
rmdir "${gen_dir}/modules"
