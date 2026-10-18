# syntax=docker/dockerfile:1
#
# Dedicated container image for k8sAgentMesh (ISI-6205). Entrypoint: meshgw,
# the AgentMesh gateway server (cmd/meshgw) — the per-pod PID1 process-manager
# that holds a pool of coding-agent runtime processes (goose by default,
# ISI-6197) behind the RuntimeAdapter seam and serves /healthz, /ready and the
# A2A /task surface.
#
# Hardening (ISI-6205 base + ISI-6103 Phase B / ISI-6212): multi-stage, minimal
# non-root distroless base, static (CGO-free) binary, and digest-pinned base
# images below. The CI `docker` job (.github/workflows/ci.yml) builds this
# linux/amd64,linux/arm64 with provenance + SBOM attestation, runs a Grype scan
# of the built image (fail on Critical), and publishes to GHCR on main/release.
#
# Build:  docker build -t k8sagentmesh:dev .
# Run:    docker run --rm -p 8080:8080 k8sagentmesh:dev   # serves /healthz, /ready

# ---- build stage ------------------------------------------------------------
# Digest-pinned for reproducible, tamper-evident builds (ISI-6212). The pin is
# the multi-arch manifest-list digest, so buildx still resolves the right
# per-platform image under `--platform linux/amd64,linux/arm64`. Refreshed by
# Dependabot (see .github/dependabot.yml, `package-ecosystem: docker`), which
# opens a PR bumping both the digest and the tag comment below when the tag
# moves. The tag lives on its own comment line (Docker rejects an inline
# comment after FROM: "FROM requires either one or three arguments").
# tag: golang:1.26
FROM golang:1.27@sha256:820153096fda4de6ac1b7194d12e416b7dd8f4e9a33d17e0345b111b945162d8 AS build

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
# Digest-pinned (multi-arch manifest list); refreshed by Dependabot alongside
# the build-stage pin. Tag on its own comment line (see build-stage note).
# tag: gcr.io/distroless/static:nonroot
FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3

COPY --from=build /out/meshgw /usr/local/bin/meshgw

# Documented contract (see cmd/meshgw/main.go for the full env list).
EXPOSE 8080
ENV MESHGW_ADDR=:8080

USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/meshgw"]
