# Parameterized Dockerfile shared by all 5 services (resource, telemetry,
# gateway, dispatch, simulator). Build context MUST be the repo root: the
# multi-module monorepo's `replace` directives point at ../../platform and
# ../../api/*/proto/gen, which live outside any single internal/<service> dir.
#
# Usage:
#   docker build -f deploy/docker/Dockerfile --build-arg SERVICE=resource -t vpp-resource:local .

ARG GO_VERSION=1.26
FROM golang:${GO_VERSION}-bookworm AS builder
ARG SERVICE
WORKDIR /src
COPY . .
WORKDIR /src/internal/${SERVICE}
# No CGO deps in any of the 6 modules (Kafka client is pure-Go segmentio/kafka-go),
# so a static binary works and doesn't need a glibc/musl runtime match.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app ./cmd

# alpine (Docker Hub) instead of gcr.io/distroless: gcr.io is unreachable from
# some networks (observed: TLS handshake reset resolving gcr.io metadata),
# while alpine mirrors reliably everywhere. No CA certs installed — nothing in
# config/*.yaml currently calls out over HTTPS (Casdoor/APISIX are plain HTTP
# internally); add `RUN apk add --no-cache ca-certificates` here if that changes.
FROM alpine:3.20
ARG SERVICE
RUN addgroup -g 65532 nonroot && adduser -D -u 65532 -G nonroot nonroot
COPY --from=builder /out/app /app
COPY config/${SERVICE}.yaml /etc/vpp/config.yaml
USER nonroot:nonroot
ENTRYPOINT ["/app", "-c", "/etc/vpp/config.yaml"]
