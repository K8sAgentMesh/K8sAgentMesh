# syntax=docker/dockerfile:1
#
# Dedicated container image for k8sAgentMesh (ISI-6205). Entrypoint: meshgw,
# the AgentMesh gateway server (cmd/meshgw) — the per-pod PID1 process-manager
# that holds a pool of coding-agent runtime processes (goose by default,
# ISI-6197) behind the RuntimeAdapter seam and serves /healthz, /ready and the
# A2A /task surface.
#
# Build-only hardening scope: multi-stage + minimal non-root distroless base +
# a static (CGO-free) binary, digest-pinnable by replacing the `:tag` refs
# below with `@sha256:...`. Multi-arch, provenance/SBOM, image scanning and the
# GHCR publish are the explicit ISI-6103 follow-up, not this image.
#
# Build:  docker build -t k8sagentmesh:dev .
# Run:    docker run --rm -p 8080:8080 k8sagentmesh:dev   # serves /healthz, /ready

# ---- build stage ------------------------------------------------------------
# Pin by digest for reproducibility once ISI-6103 lands its pin set:
#   FROM golang:1.26@sha256:... AS build
FROM golang:1.26 AS build

WORKDIR /src

# Module graph first so `go mod download` is cached across source-only changes.
# This repo is pure stdlib today (no require block), so this is a near-no-op —
# it stays correct and cache-friendly when the first dependency lands.
COPY go.mod ./
RUN go mod download

COPY . .

# Static, stripped, reproducible: CGO off so the binary runs on the distroless
# static base with no libc; -trimpath + stamped version for provenance.
ARG VERSION=dev
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
RUN go build -ldflags="-s -w -X main.version=${VERSION}" -o /out/meshgw ./cmd/meshgw

# ---- runtime stage ----------------------------------------------------------
# distroless static:nonroot — no shell, no package manager, runs as uid 65532.
# Digest-pin for release:  FROM gcr.io/distroless/static:nonroot@sha256:...
FROM gcr.io/distroless/static:nonroot

COPY --from=build /out/meshgw /usr/local/bin/meshgw

# Documented contract (see cmd/meshgw/main.go for the full env list).
EXPOSE 8080
ENV MESHGW_ADDR=:8080

USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/meshgw"]
