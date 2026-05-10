---
name: VPN Platform Project
description: Go-based censorship-resistant VPN platform — architecture, module path, key ports, and status
type: project
---

Go VPN platform at /Users/apple/Projects/go/VPN. Module path: github.com/vpnplatform/core.

**Why:** User wants a personal VPN for use in China with potential to productise later. Prompt specified production-quality, censorship-resistant, multi-protocol VPN.

**Architecture:**
- `cmd/control-plane` — Gin REST API (port 8080) + gRPC server (port 9090); manages users, nodes, transports
- `cmd/node-agent` — lightweight agent that runs on VPN servers, talks to control plane over gRPC
- `internal/` — clean arch: models → repository → service → handler
- `pkg/` — config (Viper), logger (zap), crypto (bcrypt/rand)
- `internal/grpc/proto/` — hand-written stub (replace with `make proto` once protoc available)
- `web/` — Tauri + React + TypeScript desktop client
- `deployments/docker/` — Dockerfile.dev (Go + Air hot-reload), Dockerfile.control-plane/node-agent (distroless)
- `docker-compose.yml` — full dev stack: postgres:16, redis:7, control-plane, node-agent, prometheus, grafana

**Dev stack ports:**
- 8080 → control-plane REST
- 9090 → gRPC
- 5433 → Postgres (5432 was in use on host)
- 6379 → Redis
- 3001 → Grafana
- 9092 → Prometheus

**Phase 1 complete and running** as of 2026-05-10. Both binaries compile cleanly with `go build ./cmd/...` inside Docker.

**How to apply:** When adding features, follow the existing pattern: model → repository → service → handler → router.go. Run `docker compose up -d` to start the dev stack. Run `make proto` to regenerate gRPC stubs.
