# ── run ───────────────────────────────────────────────────────────────────────

.PHONY: run-resource
run-resource:
	cd internal/resource  && go run ./cmd/main.go -c ../../config/resource.yaml

.PHONY: run-telemetry
run-telemetry:
	cd internal/telemetry && go run ./cmd/main.go -c ../../config/telemetry.yaml

.PHONY: run-gateway
run-gateway:
	cd internal/gateway && go run ./cmd/main.go -c ../../config/gateway.yaml

# ── infra ─────────────────────────────────────────────────────────────────────

.PHONY: infra-up
infra-up:
	docker compose up -d

.PHONY: infra-down
infra-down:
	docker compose down

# ── codegen ───────────────────────────────────────────────────────────────────

.PHONY: gen
gen: genproto gengateway

.PHONY: genproto
genproto:
	@./scripts/genproto.sh

.PHONY: gengateway
gengateway:
	@./scripts/gengateway.sh

.PHONY:fmt
fmt:
	goimports -l -w internal/

.PHONY:lint
lint:
	@./scripts/lint.sh

# 同时后台启动所有服务（日志写到 /tmp/logs/）
.PHONY: run-all
run-all:
	@mkdir -p /tmp/vpp-logs
	@cd internal/resource  && go run ./cmd/main.go > /tmp/vpp-logs/resource.log  2>&1 & echo $$! > /tmp/vpp-logs/resource.pid
	@cd internal/telemetry && go run ./cmd/main.go > /tmp/vpp-logs/telemetry.log 2>&1 & echo $$! > /tmp/vpp-logs/telemetry.pid
	@echo "Services started. Logs: /tmp/vpp-logs/"

# 停止所有后台服务
.PHONY: stop-all
stop-all:
	@-kill $$(cat /tmp/vpp-logs/resource.pid)  2>/dev/null
	@-kill $$(cat /tmp/vpp-logs/telemetry.pid) 2>/dev/null
	@echo "Services stopped."