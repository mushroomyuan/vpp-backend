# ── VPP Backend Makefile ───────────────────────────────────────────────────────
#
# Common targets:
#   make help | infra-up | build-all | run-all | stop-all | restart
#   make status | logs | logs SERVICE=gateway
#   make clean | clean-logs | clean-telemetry | clean-all
#
# ─────────────────────────────────────────────────────────────────────────────

SHELL := /bin/bash

BIN_DIR := ./bin
# Absolute path: run-all cds into internal/<svc>; relative LOG_DIR would break redirection.
LOG_DIR := $(abspath data/vpp-logs)
SERVICES := resource telemetry gateway dispatch simulator

# Primary listen port used by `status` (gRPC where available; HTTP for simulator).
PORT_resource  := 5002
PORT_telemetry := 5003
PORT_gateway   := 5005
PORT_dispatch  := 5006
PORT_simulator := 8084

# ── help ──────────────────────────────────────────────────────────────────────

.PHONY: help
help:
	@echo "VPP Backend — common targets"
	@echo ""
	@echo "  Infra"
	@echo "    make infra-up / infra-down     Start/stop docker compose stack"
	@echo ""
	@echo "  Services"
	@echo "    make build-all                Build all binaries → $(BIN_DIR)/"
	@echo "    make run-all                  Build + start all services in background"
	@echo "    make stop-all                 Stop background services (SIGTERM → SIGKILL)"
	@echo "    make restart                  stop-all + run-all"
	@echo "    make status                   PID / process / port health"
	@echo "    make logs                     Tail all logs (Ctrl-C to stop)"
	@echo "    make logs SERVICE=gateway     Tail one service log"
	@echo "    make run-<svc>                Foreground go run (resource|telemetry|gateway|dispatch|simulator)"
	@echo ""
	@echo "  Cleanup"
	@echo "    make clean-logs               Remove $(LOG_DIR)"
	@echo "    make clean-telemetry          Truncate telemetry DB + Redis db=1"
	@echo "    make clean                    stop-all + clean-logs + remove $(BIN_DIR)"
	@echo "    make clean-all                clean + clean-telemetry"
	@echo ""
	@echo "  Codegen / lint"
	@echo "    make gen | fmt | lint"

# ── single-service foreground run ─────────────────────────────────────────────

.PHONY: run-resource run-telemetry run-gateway run-dispatch run-simulator
run-resource:
	cd internal/resource && go run ./cmd/main.go -c ../../config/resource.yaml

run-telemetry:
	cd internal/telemetry && go run ./cmd/main.go -c ../../config/telemetry.yaml

run-gateway:
	cd internal/gateway && go run ./cmd/main.go -c ../../config/gateway.yaml

run-dispatch:
	cd internal/dispatch && go run ./cmd/main.go -c ../../config/dispatch.yaml

run-simulator:
	cd internal/simulator && go run ./cmd/main.go -c ../../config/simulator.yaml

# ── infra ─────────────────────────────────────────────────────────────────────

.PHONY: infra-up infra-down
infra-up:
	@mkdir -p $(LOG_DIR) ./data/grafana
	docker compose up -d

infra-down:
	docker compose down

# ── codegen / lint ────────────────────────────────────────────────────────────

.PHONY: gen genproto gengateway fmt lint
gen: genproto gengateway

genproto:
	@./scripts/genproto.sh

gengateway:
	@./scripts/gengateway.sh

fmt:
	goimports -l -w internal/

lint:
	@./scripts/lint.sh

# ── build / run / stop / restart ──────────────────────────────────────────────

.PHONY: build-all run-all stop-all restart status logs

build-all:
	@mkdir -p $(BIN_DIR)
	@cd internal/resource && go build -o ../../$(BIN_DIR)/resource ./cmd
	@cd internal/telemetry && go build -o ../../$(BIN_DIR)/telemetry ./cmd
	@cd internal/gateway && go build -o ../../$(BIN_DIR)/gateway ./cmd
	@cd internal/dispatch && go build -o ../../$(BIN_DIR)/dispatch ./cmd
	@cd internal/simulator && go build -o ../../$(BIN_DIR)/simulator ./cmd
	@echo "Build completed → $(BIN_DIR)/"

# Start all services in background. Simulator waits for resource+gateway ports.
run-all: build-all
	@mkdir -p $(LOG_DIR)
	@for name in resource telemetry gateway dispatch; do \
		if [ -f $(LOG_DIR)/$$name.pid ] && kill -0 $$(cat $(LOG_DIR)/$$name.pid) 2>/dev/null; then \
			echo "$$name already running (pid $$(cat $(LOG_DIR)/$$name.pid)); skip. Use 'make restart' to recycle."; \
			continue; \
		fi; \
		( cd $(PWD)/internal/$$name && \
			$(PWD)/$(BIN_DIR)/$$name > $(LOG_DIR)/$$name.log 2>&1 & \
			echo $$! > $(LOG_DIR)/$$name.pid; \
			echo "started $$name pid=$$!" ); \
	done

	@echo "Waiting for resource (:5002) and gateway (:8083)..."
	@i=0; \
	while [ $$i -lt 45 ]; do \
		res_ok=0; gw_ok=0; \
		(echo >/dev/tcp/127.0.0.1/5002) >/dev/null 2>&1 && res_ok=1; \
		(echo >/dev/tcp/127.0.0.1/8083) >/dev/null 2>&1 && gw_ok=1; \
		if [ $$res_ok -eq 1 ] && [ $$gw_ok -eq 1 ]; then \
			echo "Dependencies ready."; \
			break; \
		fi; \
		i=$$((i+1)); \
		sleep 1; \
	done; \
	if [ $$i -ge 45 ]; then \
		echo "WARN: timed out waiting for resource/gateway; starting simulator anyway (it will retry)."; \
	fi

	@if [ -f $(LOG_DIR)/simulator.pid ] && kill -0 $$(cat $(LOG_DIR)/simulator.pid) 2>/dev/null; then \
		echo "simulator already running (pid $$(cat $(LOG_DIR)/simulator.pid)); skip."; \
	else \
		( cd $(PWD)/internal/simulator && \
			$(PWD)/$(BIN_DIR)/simulator > $(LOG_DIR)/simulator.log 2>&1 & \
			echo $$! > $(LOG_DIR)/simulator.pid; \
			echo "started simulator pid=$$!" ); \
	fi
	@echo "Services started. Logs: $(LOG_DIR)/  |  make status | make logs"

# Graceful stop, then force-kill stragglers.
stop-all:
	@mkdir -p $(LOG_DIR)
	@for name in $(SERVICES); do \
		pidfile=$(LOG_DIR)/$$name.pid; \
		[ -f $$pidfile ] || continue; \
		pid=$$(cat $$pidfile); \
		if kill -0 $$pid 2>/dev/null; then \
			echo "Stopping $$name pid=$$pid"; \
			kill $$pid 2>/dev/null || true; \
			for i in 1 2 3 4 5; do \
				kill -0 $$pid 2>/dev/null || break; \
				sleep 1; \
			done; \
			if kill -0 $$pid 2>/dev/null; then \
				echo "  force kill $$name pid=$$pid"; \
				kill -9 $$pid 2>/dev/null || true; \
			fi; \
		else \
			echo "$$name pidfile stale (pid $$pid not running)"; \
		fi; \
		rm -f $$pidfile; \
	done
	@echo "Services stopped."

restart: stop-all run-all

# ── status / logs ─────────────────────────────────────────────────────────────

status:
	@printf "%-12s %-8s %-10s %-8s %-s\n" "SERVICE" "PID" "PROCESS" "PORT" "NOTE"
	@printf "%-12s %-8s %-10s %-8s %-s\n" "-------" "---" "-------" "----" "----"
	@check() { \
		name=$$1; port=$$2; note=$$3; \
		pidfile="$(LOG_DIR)/$$name.pid"; \
		pid="-"; proc="DOWN"; listen="DOWN"; \
		if [ -f $$pidfile ]; then \
			pid=$$(cat $$pidfile); \
			if kill -0 $$pid 2>/dev/null; then proc="UP"; else proc="DEAD"; fi; \
		fi; \
		if ss -ltn 2>/dev/null | grep -qE ":$$port\s"; then listen="UP"; fi; \
		printf "%-12s %-8s %-10s %-8s %-s\n" $$name $$pid $$proc $$listen "$$note"; \
	}; \
	check resource  $(PORT_resource)  "gRPC (+HTTP :8082)"; \
	check telemetry $(PORT_telemetry) "gRPC"; \
	check gateway   $(PORT_gateway)   "gRPC (+HTTP :8083)"; \
	check dispatch  $(PORT_dispatch)  "gRPC"; \
	check simulator $(PORT_simulator) "HTTP"

# Tail logs. Usage: make logs   or   make logs SERVICE=gateway
logs:
	@mkdir -p $(LOG_DIR)
	@if [ -n "$(SERVICE)" ]; then \
		f=$(LOG_DIR)/$(SERVICE).log; \
		if [ ! -f $$f ]; then echo "No log: $$f"; exit 1; fi; \
		echo "Tailing $$f (Ctrl-C to stop)"; \
		tail -n 100 -F $$f; \
	else \
		echo "Tailing $(LOG_DIR)/*.log (Ctrl-C to stop)"; \
		touch $(LOG_DIR)/resource.log $(LOG_DIR)/telemetry.log $(LOG_DIR)/gateway.log \
			$(LOG_DIR)/dispatch.log $(LOG_DIR)/simulator.log; \
		tail -n 50 -F $(LOG_DIR)/resource.log $(LOG_DIR)/telemetry.log $(LOG_DIR)/gateway.log \
			$(LOG_DIR)/dispatch.log $(LOG_DIR)/simulator.log; \
	fi

# ── cleanup ───────────────────────────────────────────────────────────────────

.PHONY: clean-logs clean-telemetry clean clean-all

clean-logs:
	@rm -rf $(LOG_DIR)
	@mkdir -p $(LOG_DIR)
	@echo "Logs cleaned: $(LOG_DIR) (empty dir recreated for Alloy mount)"

# Truncate telemetry hypertable + Redis snapshot db. Does NOT touch other DBs.
clean-telemetry:
	@echo "Truncating telemetry_records..."
	@docker exec vpp-backend-postgres-1 psql -U postgres -d telemetry -v ON_ERROR_STOP=1 \
		-c "TRUNCATE TABLE telemetry_records;"
	@echo "Flushing Redis db=1 (telemetry CU snapshots)..."
	@docker exec vpp-backend-redis-1 redis-cli -n 1 FLUSHDB
	@echo "Telemetry data cleaned."

# Stop services, remove logs + binaries. Safe default (keeps DB data).
clean: stop-all clean-logs
	@rm -rf $(BIN_DIR)
	@echo "Clean done (binaries + logs). DB data kept — use clean-telemetry / clean-all if needed."

# Full local reset of process artifacts + telemetry time-series.
clean-all: clean clean-telemetry
	@echo "Clean-all done."
