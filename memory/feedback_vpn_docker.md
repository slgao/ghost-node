---
name: VPN dev environment uses Docker
description: Go and all tools are installed inside Docker containers — no local Go installation needed
type: feedback
---

Go is not installed locally on this machine. All Go builds, `go mod tidy`, linting, and tests run inside Docker containers.

**Why:** User explicitly asked for everything to run in Docker.

**How to apply:** Never suggest running `go build`, `go mod tidy`, or `go test` directly on the host. Instead use `docker compose run --rm ...` or `docker run --rm -v $(pwd):/app vpnplatform-dev:latest ...`. The dev image is `vpnplatform-dev:latest` (built from `deployments/docker/Dockerfile.dev`).
