.PHONY: dev build proto lint test clean docker-up docker-down help

MODULE := github.com/vpnplatform/core
CONTROL_PLANE_BIN := bin/control-plane
NODE_AGENT_BIN    := bin/node-agent
PROTO_DIR         := api/proto
PROTO_OUT         := internal/grpc/proto

## ─── Development ────────────────────────────────────────────────────────────

dev: ## Start full dev stack (Docker)
	docker compose -f docker-compose.yml up --build

dev-down: ## Stop dev stack
	docker compose -f docker-compose.yml down -v

logs: ## Tail all service logs
	docker compose -f docker-compose.yml logs -f

## ─── Build ──────────────────────────────────────────────────────────────────

build: proto ## Build both binaries (requires local Go)
	@mkdir -p bin
	go build -ldflags="-s -w" -o $(CONTROL_PLANE_BIN) ./cmd/control-plane
	go build -ldflags="-s -w" -o $(NODE_AGENT_BIN)    ./cmd/node-agent

## ─── Protobuf ───────────────────────────────────────────────────────────────

proto: ## Generate Go code from .proto files (runs inside Docker)
	docker compose -f docker-compose.yml run --rm proto-gen

proto-local: ## Generate proto locally (requires protoc + plugins)
	@mkdir -p $(PROTO_OUT)
	protoc \
		--go_out=$(PROTO_OUT) --go_opt=paths=source_relative \
		--go-grpc_out=$(PROTO_OUT) --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/*.proto

## ─── Quality ─────────────────────────────────────────────────────────────────

lint: ## Run golangci-lint
	docker compose -f docker-compose.yml run --rm lint

test: ## Run all tests
	docker compose -f docker-compose.yml run --rm test

test-local: ## Run tests locally
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

## ─── Database ────────────────────────────────────────────────────────────────

migrate-up: ## Apply migrations
	docker compose -f docker-compose.yml exec control-plane ./bin/control-plane migrate up

migrate-down: ## Rollback last migration
	docker compose -f docker-compose.yml exec control-plane ./bin/control-plane migrate down

## ─── Cleanup ─────────────────────────────────────────────────────────────────

clean: ## Remove build artifacts
	rm -rf bin/ coverage.out coverage.html

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
