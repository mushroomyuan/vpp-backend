#!/usr/bin/env bash
# Seed a demo Site/Asset/CU/Point tree in Resource and matching Gateway mappings
# for vpp-simulator closed-loop testing.
#
# Prerequisites: resource (:8082) and gateway (:8083) are running.
#
# Resource HTTP (grpc-gateway) uses proto JSON field names (PascalCase), e.g. Name / CUID.
# Gateway HTTP uses snake_case JSON.
#
# Usage:
#   ./scripts/seed_simulator_demo.sh
#   TENANT_ID=demo RESOURCE_HTTP=http://127.0.0.1:8082 ./scripts/seed_simulator_demo.sh

set -euo pipefail

TENANT_ID="${TENANT_ID:-default}"
RESOURCE_HTTP="${RESOURCE_HTTP:-http://127.0.0.1:8082}"
GATEWAY_HTTP="${GATEWAY_HTTP:-http://127.0.0.1:8083}"

json_field() {
  local body="$1" field="$2"
  python3 -c "import json,sys; d=json.loads(sys.argv[1]); print(d.get('$field','') or '')" "$body"
}

post_json() {
  local url="$1" payload="$2"
  curl -sS -X POST "$url" \
    -H "Content-Type: application/json" \
    -d "$payload"
}

die() { echo "ERROR: $*" >&2; exit 1; }

echo "==> Creating Site"
SITE_RESP=$(post_json "$RESOURCE_HTTP/api/tenants/$TENANT_ID/sites" \
  '{"Name":"Sim Demo Site","Description":"vpp-simulator demo"}')
SITE_ID=$(json_field "$SITE_RESP" "SiteID")
[[ -n "$SITE_ID" ]] || die "CreateSite failed: $SITE_RESP"
echo "    SiteID=$SITE_ID"

echo "==> Creating Asset (ESS)"
ASSET_RESP=$(post_json "$RESOURCE_HTTP/api/tenants/$TENANT_ID/sites/$SITE_ID/resources" \
  '{"Name":"Sim ESS","RatedCapacityKW":100,"EnergyType":"battery","SubType":"ESS","DispatchMode":"centralized","OwnerType":"self","MarketEnabled":true}')
ASSET_ID=$(json_field "$ASSET_RESP" "AssetID")
[[ -n "$ASSET_ID" ]] || die "CreateAsset failed: $ASSET_RESP"
echo "    AssetID=$ASSET_ID"

create_cu() {
  local name="$1" type="$2" ext="$3"
  post_json "$RESOURCE_HTTP/api/tenants/$TENANT_ID/resources/$ASSET_ID/cus" \
    "{\"Name\":\"$name\",\"Type\":\"$type\",\"Provider\":\"simulator\",\"ExternalID\":\"$ext\"}"
}

create_point() {
  local cu_id="$1" metric_id="$2" access_mode="$3"
  post_json "$RESOURCE_HTTP/api/tenants/$TENANT_ID/cus/$cu_id/points" \
    "{\"AssetID\":\"$ASSET_ID\",\"MetricID\":\"$metric_id\",\"ExternalAddress\":\"$metric_id\",\"AccessMode\":\"$access_mode\",\"Scale\":1,\"Offset\":0,\"Enabled\":true}" >/dev/null
}

create_capability() {
  local cu_id="$1" capability_id="$2" spec="$3"
  post_json "$RESOURCE_HTTP/api/tenants/$TENANT_ID/cus/$cu_id/capabilities" \
    "{\"CapabilityID\":\"$capability_id\",\"SchemaVersion\":1,\"Spec\":$spec,\"Enabled\":true}" >/dev/null
}

create_mapping() {
  local ext="$1" cu="$2"
  local resp
  resp=$(post_json "$GATEWAY_HTTP/api/v1/tenants/$TENANT_ID/mappings" \
    "{\"external_system\":\"simulator\",\"external_id\":\"$ext\",\"cu_code\":\"$cu\"}")
  echo "    mapping: $resp"
}

echo "==> Creating Battery CU + points + mapping"
BAT_RESP=$(create_cu "Sim Battery" "Battery" "sim-battery-001")
BAT_ID=$(json_field "$BAT_RESP" "CUID")
[[ -n "$BAT_ID" ]] || die "CreateCU Battery failed: $BAT_RESP"
echo "    CUID=$BAT_ID ExternalID=sim-battery-001"
create_capability "$BAT_ID" "energy.storage.v1" \
  '{"usable_energy_kwh":200,"max_charge_power_kw":50,"max_discharge_power_kw":50}'
create_point "$BAT_ID" "energy_storage.state_of_charge.v1" "POINT_ACCESS_MODE_READ"
create_point "$BAT_ID" "electrical.active_power.v1" "POINT_ACCESS_MODE_READ"
create_point "$BAT_ID" "electrical.active_power_setpoint.v1" "POINT_ACCESS_MODE_WRITE"
create_mapping "sim-battery-001" "$BAT_ID"

echo "==> Creating PCS CU + points + mapping"
PCS_RESP=$(create_cu "Sim PCS" "PCS" "sim-pcs-001")
PCS_ID=$(json_field "$PCS_RESP" "CUID")
[[ -n "$PCS_ID" ]] || die "CreateCU PCS failed: $PCS_RESP"
echo "    CUID=$PCS_ID ExternalID=sim-pcs-001"
create_point "$PCS_ID" "electrical.active_power.v1" "POINT_ACCESS_MODE_READ"
create_point "$PCS_ID" "electrical.reactive_power.v1" "POINT_ACCESS_MODE_READ"
create_point "$PCS_ID" "electrical.active_power_setpoint.v1" "POINT_ACCESS_MODE_WRITE"
create_mapping "sim-pcs-001" "$PCS_ID"

echo "==> Creating PV CU + points + mapping"
PV_RESP=$(create_cu "Sim PV" "PV" "sim-pv-001")
PV_ID=$(json_field "$PV_RESP" "CUID")
[[ -n "$PV_ID" ]] || die "CreateCU PV failed: $PV_RESP"
echo "    CUID=$PV_ID ExternalID=sim-pv-001"
create_point "$PV_ID" "electrical.active_power.v1" "POINT_ACCESS_MODE_READ"
create_mapping "sim-pv-001" "$PV_ID"

echo "==> Creating Meter CU + points + mapping"
MTR_RESP=$(create_cu "Sim Meter" "Meter" "sim-meter-001")
MTR_ID=$(json_field "$MTR_RESP" "CUID")
[[ -n "$MTR_ID" ]] || die "CreateCU Meter failed: $MTR_RESP"
echo "    CUID=$MTR_ID ExternalID=sim-meter-001"
create_point "$MTR_ID" "electrical.active_power.v1" "POINT_ACCESS_MODE_READ"
create_mapping "sim-meter-001" "$MTR_ID"

# Persist IDs for the testing guide / later scripts
OUT_FILE="${SEED_OUT:-/tmp/vpp-simulator-seed.env}"
cat > "$OUT_FILE" <<EOF
TENANT_ID=$TENANT_ID
SITE_ID=$SITE_ID
ASSET_ID=$ASSET_ID
BATTERY_CU=$BAT_ID
PCS_CU=$PCS_ID
PV_CU=$PV_ID
METER_CU=$MTR_ID
BATTERY_EXT=sim-battery-001
PCS_EXT=sim-pcs-001
PV_EXT=sim-pv-001
METER_EXT=sim-meter-001
EOF

cat <<EOF

Seed complete. IDs written to $OUT_FILE

  Tenant:  $TENANT_ID
  Site:    $SITE_ID
  Asset:   $ASSET_ID
  Battery: $BAT_ID (sim-battery-001)
  PCS:     $PCS_ID (sim-pcs-001)
  PV:      $PV_ID (sim-pv-001)
  Meter:   $MTR_ID (sim-meter-001)

Next:
  1. Start simulator:  cd internal/simulator && go run ./cmd -c ../../config/simulator.yaml
  2. Inspect runtime:  curl -s http://127.0.0.1:8084/api/v1/runtime | jq .
  3. See testing guide: internal/simulator/TESTING.md

EOF
