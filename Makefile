# ── VPP Backend Makefile ───────────────────────────────────────────────────────
#
# Common targets:
#   make help | infra-up | build-all | run-all | stop-all | restart
#   make status | logs | logs SERVICE=gateway
#   make apisix-up | apisix-init | apisix-down | apisix-status
#   make casdoor-up | casdoor-init | casdoor-down | casdoor-status | casdoor-token
#   make tidy | gen | fmt | lint
#   make clean | clean-logs | clean-telemetry | clean-all
#
# ─────────────────────────────────────────────────────────────────────────────

SHELL := /bin/bash

BIN_DIR := bin
# Absolute paths: run-all cds into internal/<svc>; relative LOG_DIR would break redirection.
BIN_ABS := $(abspath $(BIN_DIR))
LOG_DIR := $(abspath data/vpp-logs)
APISIX_COMPOSE := deploy/apisix/docker-compose.apisix.yaml
CASDOOR_COMPOSE := deploy/casdoor/docker-compose.casdoor.yaml
DOCKER_COMPOSE := $(shell command -v docker-compose >/dev/null 2>&1 && echo docker-compose || echo "docker compose")
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
	@echo "    make apisix-up / apisix-down   Start/stop APISIX edge gateway"
	@echo "    make apisix-init               Install Phase 0+1 proxy routes"
	@echo "    make apisix-status             APISIX + backend port health"
	@echo "    make casdoor-up / casdoor-down Start/stop Casdoor IdP"
	@echo "    make casdoor-init             Ensure casdoor DB + verify seed"
	@echo "    make casdoor-status           Casdoor port / OIDC discovery"
	@echo "    make casdoor-token [USER=]    Password Grant access_token"
	@echo "    make casdoor-token USER=admin DECODE=1   decode JWT claims"
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
	@echo "  Codegen / lint / modules"
	@echo "    make tidy                     go mod tidy in internal/{platform,services}"
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

.PHONY: infra-up infra-down grafana-fix-perms
infra-up: grafana-fix-perms
	docker compose up -d

# Grafana image runs as UID 472; host bind-mount must be writable by that user.
# Also normalize ./data/vpp-logs ownership so host processes (make run-all) can write.
grafana-fix-perms:
	@mkdir -p $(LOG_DIR) ./data/grafana
	@docker run --rm -v "$(CURDIR)/data:/data" alpine:3.20 \
		sh -c 'chown -R 472:472 /data/grafana && chown -R $(shell id -u):$(shell id -g) /data/vpp-logs'

infra-down:
	docker compose down

# ── APISIX northbound edge (Phase 0) ──────────────────────────────────────────

.PHONY: apisix-up apisix-down apisix-init apisix-status apisix-logs
apisix-up:
	$(DOCKER_COMPOSE) -f $(APISIX_COMPOSE) up -d
	@echo "APISIX starting. Proxy :9080 | Admin :9181 | Metrics :9091"
	@echo "Next: make run-all && make apisix-init"

apisix-down:
	$(DOCKER_COMPOSE) -f $(APISIX_COMPOSE) down

apisix-init:
	@bash deploy/apisix/init.sh

apisix-logs:
	$(DOCKER_COMPOSE) -f $(APISIX_COMPOSE) logs -f apisix

apisix-status:
	@printf "%-14s %-8s %-s\n" "COMPONENT" "PORT" "STATUS"
	@printf "%-14s %-8s %-s\n" "-----------" "----" "------"
	@check_port() { \
		name=$$1; port=$$2; \
		if ss -ltn 2>/dev/null | grep -qE ":$$port\s"; then \
			printf "%-14s %-8s %-s\n" $$name $$port "UP"; \
		else \
			printf "%-14s %-8s %-s\n" $$name $$port "DOWN"; \
		fi; \
	}; \
	check_port apisix-proxy 9080; \
	check_port apisix-admin 9181; \
	check_port gateway-http 8083; \
	check_port resource-http 8082; \
	if curl --noproxy '*' -sf http://127.0.0.1:9181/apisix/admin/routes \
		-H "X-API-KEY: $$(awk '/key:/{print $$2; exit}' deploy/apisix/conf/config.yaml)" >/dev/null 2>&1; then \
		routes=$$(curl --noproxy '*' -sf http://127.0.0.1:9181/apisix/admin/routes \
			-H "X-API-KEY: $$(awk '/key:/{print $$2; exit}' deploy/apisix/conf/config.yaml)" | \
			grep -o '"total":[0-9]*' | head -1 | cut -d: -f2); \
		echo "routes-configured: $${routes:-unknown}"; \
	else \
		echo "routes-configured: admin-unreachable"; \
	fi

# ── Casdoor IdP (Phase 2 OIDC) ────────────────────────────────────────────────

.PHONY: casdoor-up casdoor-down casdoor-init casdoor-status casdoor-logs casdoor-db casdoor-token
casdoor-db:
	@bash -c ' \
		c=$$(docker ps --format "{{.Names}}" | grep -E "postgres" | head -1); \
		if [ -z "$$c" ]; then echo "ERROR: postgres container not running (make infra-up)" >&2; exit 1; fi; \
		if [ "$$(docker exec $$c psql -U postgres -Atc "SELECT 1 FROM pg_database WHERE datname='"'"'casdoor'"'"'")" = "1" ]; then \
			echo "Database casdoor already exists."; \
		else \
			docker exec $$c psql -U postgres -c "CREATE DATABASE casdoor;"; \
		fi'

casdoor-up: casdoor-db
	$(DOCKER_COMPOSE) -f $(CASDOOR_COMPOSE) up -d
	@echo "Casdoor starting. UI :8000 | OIDC discovery /.well-known/openid-configuration"
	@echo "Next: make casdoor-init"

casdoor-down:
	$(DOCKER_COMPOSE) -f $(CASDOOR_COMPOSE) down

casdoor-init:
	@bash deploy/casdoor/init.sh

casdoor-logs:
	$(DOCKER_COMPOSE) -f $(CASDOOR_COMPOSE) logs -f casdoor

casdoor-status:
	@printf "%-14s %-8s %-s\n" "COMPONENT" "PORT" "STATUS"
	@printf "%-14s %-8s %-s\n" "-----------" "----" "------"
	@check_port() { \
		name=$$1; port=$$2; \
		if ss -ltn 2>/dev/null | grep -qE ":$$port\s"; then \
			printf "%-14s %-8s %-s\n" $$name $$port "UP"; \
		else \
			printf "%-14s %-8s %-s\n" $$name $$port "DOWN"; \
		fi; \
	}; \
	check_port casdoor 8000; \
	if curl --noproxy '*' -sf http://127.0.0.1:8000/.well-known/openid-configuration >/dev/null 2>&1; then \
		echo "oidc-discovery: OK"; \
	else \
		echo "oidc-discovery: DOWN"; \
	fi

# Password Grant → access_token. USER=admin|operator|viewer (default admin).
# DECODE=1 prints JWT payload + C3 claim mapping instead of raw token.
# Example: curl -H "Authorization: Bearer $$(make -s casdoor-token)" ...
casdoor-token:
	@u="$(USER)"; \
	case "$$u" in admin|operator|viewer) ;; *) u=admin ;; esac; \
	CASDOOR_USER="$$u" DECODE="$(DECODE)" bash deploy/casdoor/token.sh \
		$$( [ "$(DECODE)" = "1" ] && echo --decode )

# ── codegen / lint / modules ──────────────────────────────────────────────────

.PHONY: gen genproto gengateway fmt lint tidy
gen: genproto gengateway

genproto:
	@./scripts/genproto.sh

gengateway:
	@./scripts/gengateway.sh

fmt:
	goimports -l -w internal/

lint:
	@./scripts/lint.sh

# Run go mod tidy for the six internal modules (platform + services).
tidy:
	@failed=0; \
	for dir in internal/platform internal/resource internal/telemetry \
		internal/gateway internal/dispatch internal/simulator; do \
		echo "==> go mod tidy ($$dir)"; \
		if ( cd "$$dir" && go mod tidy ); then \
			:; \
		else \
			echo "FAILED: $$dir"; \
			failed=1; \
		fi; \
	done; \
	echo "tidy done (6 modules)"; \
	exit $$failed

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
# - setsid: new session so services survive the make/shell process-group cleanup
# - env -u LOCAL_ENV: background logs stay JSON for Loki (colored text = foreground only)
# - pidfile records the real binary PID (via pgrep), not a shell wrapper
run-all: build-all
	@mkdir -p $(LOG_DIR) $(BIN_ABS)
	@start_svc() { \
		name=$$1; \
		bin="$(BIN_ABS)/$$name"; \
		pidfile="$(LOG_DIR)/$$name.pid"; \
		logfile="$(LOG_DIR)/$$name.log"; \
		if [ -f $$pidfile ] && kill -0 $$(cat $$pidfile) 2>/dev/null; then \
			echo "$$name already running (pid $$(cat $$pidfile)); skip. Use 'make restart' to recycle."; \
			return 0; \
		fi; \
		if pgrep -f "$${bin}$$" >/dev/null 2>&1; then \
			echo "$$name orphan binary still running; run 'make stop-all' first."; \
			return 1; \
		fi; \
		( cd "$(PWD)/internal/$$name" && \
			setsid env -u LOCAL_ENV $$bin > $$logfile 2>&1 < /dev/null & ); \
		sleep 0.5; \
		pid=$$(pgrep -n -f "$${bin}$$" || true); \
		if [ -n "$$pid" ] && kill -0 $$pid 2>/dev/null; then \
			echo $$pid > $$pidfile; \
			echo "started $$name pid=$$pid"; \
			return 0; \
		fi; \
		echo "FAILED $$name (exited immediately — see $$logfile)"; \
		tail -n 8 $$logfile 2>/dev/null || true; \
		rm -f $$pidfile; \
		return 1; \
	}; \
	for name in resource telemetry gateway dispatch; do \
		start_svc $$name || exit 1; \
	done; \
	echo "Waiting for resource (:5002) and gateway (:8083)..."; \
	i=0; \
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
	fi; \
	start_svc simulator || exit 1; \
	echo "Services started. Logs: $(LOG_DIR)/  |  make status | make logs"

# Stop via pidfile, then sweep leftover binaries (stale pidfiles, ./bin vs bin paths).
stop-all:
	@mkdir -p $(LOG_DIR)
	@for name in $(SERVICES); do \
		pidfile=$(LOG_DIR)/$$name.pid; \
		if [ -f $$pidfile ]; then \
			pid=$$(cat $$pidfile); \
			if kill -0 $$pid 2>/dev/null; then \
				echo "Stopping $$name pid=$$pid"; \
				kill $$pid 2>/dev/null || true; \
			else \
				echo "$$name pidfile stale (pid $$pid not running)"; \
			fi; \
			rm -f $$pidfile; \
		fi; \
		orphans=$$(pgrep -f '/$(BIN_DIR)/'"$$name"'( |$$)' 2>/dev/null || true); \
		if [ -n "$$orphans" ]; then \
			echo "Stopping $$name orphan(s): $$orphans"; \
			kill $$orphans 2>/dev/null || true; \
		fi; \
	done
	@for i in 1 2 3 4 5; do \
		left=$$(pgrep -f '/$(BIN_DIR)/(resource|telemetry|gateway|dispatch|simulator)( |$$)' 2>/dev/null || true); \
		[ -z "$$left" ] && break; \
		sleep 1; \
	done
	@left=$$(pgrep -f '/$(BIN_DIR)/(resource|telemetry|gateway|dispatch|simulator)( |$$)' 2>/dev/null || true); \
	if [ -n "$$left" ]; then \
		echo "force kill stragglers: $$left"; \
		kill -9 $$left 2>/dev/null || true; \
	fi
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
		if [ "$$proc" != "UP" ] && [ "$$listen" = "UP" ]; then \
			note="ORPHAN port holder — make stop-all"; \
		fi; \
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

# Logs dir may be root-owned (e.g. created via sudo / prior container). Use Alpine
# as root to wipe + recreate owned by the invoking user.
clean-logs:
	@docker run --rm -v "$(CURDIR)/data:/data" alpine:3.20 \
		sh -c 'rm -rf /data/vpp-logs && mkdir -p /data/vpp-logs && chown $(shell id -u):$(shell id -g) /data/vpp-logs'
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
