# Makefile for Kite project
.PHONY: help dev build clean test verify format-check verify-architecture verify-deployment verify-kubernetes-compatibility docker-build docker-run frontend static backend install deps e2e-install e2e-install-browser e2e-kind-up e2e-kind-down e2e-stop-app e2e-setup-dex e2e-run e2e-run-headed e2e-test e2e-test-headed

# Variables
BINARY_NAME=kite
UI_DIR=ui
E2E_DIR=e2e
DOCKER_IMAGE=kite
E2E_KIND_NAME ?= kite-e2e
E2E_NODE_IMAGE ?=
E2E_PORT ?= 38080
E2E_KUBECONFIG ?= $(shell printf '%s' "$${TMPDIR:-/tmp/}kite-e2e.kubeconfig")
E2E_AUTH_NETWORK ?= kite-e2e-auth
E2E_DEX_CONTAINER ?= kite-e2e-dex
E2E_DEX_VOLUME ?= kite-e2e-dex-data
E2E_OAUTH_PORT ?= 5556
E2E_OIDC_CERT_DIR ?= $(shell printf '%s' "$${TMPDIR:-/tmp/}kite-e2e-oidc")
E2E_OIDC_FORWARDER ?= $(E2E_KIND_NAME)-oidc-forwarder
SPEC ?=

# Version information
VERSION=$(shell scripts/get-version.sh)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT_ID ?= $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")

# Build flags
LDFLAGS=-ldflags "-s -w \
	-X 'github.com/zxh326/kite/pkg/version.Version=$(VERSION)' \
	-X 'github.com/zxh326/kite/pkg/version.BuildDate=$(BUILD_DATE)' \
	-X 'github.com/zxh326/kite/pkg/version.CommitID=$(COMMIT_ID)'"

# Default target
.DEFAULT_GOAL := build
DOCKER_TAG=latest

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

# Help target
help: ## Show this help message
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Install dependencies
install: deps ## Install all dependencies
	@echo "📦 Installing dependencies..."

deps: ## Install frontend and backend dependencies
	@echo "📦 Installing frontend dependencies..."
	cd $(UI_DIR) && pnpm install
	@echo "📚 Installing documentation dependencies..."
	cd docs && pnpm install
	@echo "📦 Installing backend dependencies..."
	go mod download

# Build targets
build: frontend backend ## Build both frontend and backend
	@echo "✅ Build completed successfully!"
	@echo "🚀 Run './$(BINARY_NAME)' to start the server"

clean-frontend: ## Clean frontend build artifacts
	cd $(UI_DIR) && rm -rf dist node_modules/.vite
	rm -rf static

clean-backend: ## Clean backend build artifacts
	rm -rf $(BINARY_NAME) bin/

clean: clean-frontend clean-backend ## Clean all build artifacts
	@echo "🧹 All build artifacts cleaned!"

cross-compile: frontend ## Cross-compile for multiple architectures
	@echo "🔄 Cross-compiling for multiple architectures..."
	mkdir -p bin
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath $(LDFLAGS) -o bin/$(BINARY_NAME)-amd64 .
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath $(LDFLAGS) -o bin/$(BINARY_NAME)-arm64 .
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-arm64 .
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath $(LDFLAGS) -o bin/$(BINARY_NAME)-windows-amd64.exe .
	# GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -trimpath $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-amd64 .

package-release:
	@echo "🔄 Packaging..."
	tar -czvf bin/$(BINARY_NAME)-$(shell git describe --tags --match 'v*' | grep -oE 'v[0-9]+\.[0-9][0-9]*(\.[0-9]+)?').tar.gz bin/*

package-binaries: ## Package each kite binary file separately
	@echo "🔄 Packaging kite binaries separately..."
	@VERSION=$$(git describe --tags --match 'v*' | grep -oE 'v[0-9]+\.[0-9][0-9]*(\.[0-9]+)?'); \
	for file in bin/kite-*; do \
		if [ -f "$$file" ]; then \
			filename=$$(basename "$$file"); \
			echo "📦 Packaging $$filename with version $$VERSION..."; \
			tar -czvf "bin/$$filename-$$VERSION.tar.gz" "$$file"; \
		fi; \
	done
	@echo "✅ All kite binaries packaged successfully!"

frontend: static ## Build frontend only

static: ui/src/**/*.tsx ui/src/**/*.ts ui/index.html ui/**/*.css ui/package.json ui/vite.config.ts
	@echo "📦 Ensuring static files are built..."
	cd $(UI_DIR) && pnpm run build

backend: ${BINARY_NAME} ## Build backend only

$(BINARY_NAME): main.go pkg/**/*.go go.mod static *.go **/*.go
	@echo "🏗️ Building backend..."
	CGO_ENABLED=0 go build -trimpath $(LDFLAGS) -o $(BINARY_NAME) .

# Production targets
run: backend ## Run the built application
	@echo "🚀 Starting $(BINARY_NAME) server..."
	./$(BINARY_NAME)

dev: ## Run in development mode
	@echo "🔄 Starting development mode..."
	@echo "🚀 Starting $(BINARY_NAME) server..."
	CGO_ENABLED=0 go build -trimpath $(LDFLAGS) -o $(BINARY_NAME) .
	./$(BINARY_NAME) -v=5 & \
	BACKEND_PID=$$!; \
	echo "Backend PID: $$BACKEND_PID"; \
	trap 'echo "🛑 Stopping backend server..."; kill $$BACKEND_PID 2>/dev/null; exit' INT TERM; \
	echo "🔄 Starting development server..."; \
	cd $(UI_DIR) && pnpm run dev; \
	echo "🛑 Stopping backend server..."; \
	kill $$BACKEND_PID 2>/dev/null

lint: golangci-lint ## Run linters
	@echo "🔍 Running linters..."
	@echo "Backend linting..."
	go vet ./...
	$(GOLANGCI_LINT) run
	@echo "Frontend linting..."
	cd $(UI_DIR) && pnpm run lint

golangci-lint: ## Download golangci-lint locally if necessary.
	test -f $(GOLANGCI_LINT) || curl -sSfL https://golangci-lint.run/install.sh | sh -s v2.12.1

format: ## Format code
	@echo "✨ Formatting code..."
	go fmt ./...
	cd $(UI_DIR) && pnpm run format
	cd $(UI_DIR) && pnpm exec prettier --write '../$(E2E_DIR)/{env.ts,playwright.config.ts,setup/**/*.ts,helpers/**/*.ts,specs/**/*.ts}'

format-check: ## Check formatting without modifying the worktree
	@test -z "$$(gofmt -l $$(rg --files -g '*.go'))" || { gofmt -l $$(rg --files -g '*.go'); exit 1; }
	cd $(UI_DIR) && pnpm run format:check
	cd $(UI_DIR) && pnpm exec prettier --check '../$(E2E_DIR)/{env.ts,playwright.config.ts,setup/**/*.ts,helpers/**/*.ts,specs/**/*.ts}'

verify: format-check lint verify-architecture verify-deployment verify-kubernetes-compatibility ## Run non-mutating release checks
	@echo "✅ Verification completed successfully!"

# Pre-commit checks
pre-commit: format verify ## Format and run pre-commit checks
	@echo "✅ Pre-commit checks completed!"

test: ## Run tests
	@echo "🧪 Running tests..."
	go test -v ./...
	cd $(UI_DIR) && pnpm run test

verify-architecture: ## Verify removed architecture does not return
	./scripts/verify-architecture.sh

verify-deployment: ## Verify production deployment invariants and invalid-value rejection
	./scripts/verify-deployment.sh

verify-kubernetes-compatibility: ## Verify the tested Kubernetes minor window matches client-go
	./scripts/verify-kubernetes-compatibility.sh

e2e-install: ## Install e2e dependencies
	@echo "📦 Installing e2e dependencies..."
	cd $(E2E_DIR) && pnpm install

e2e-install-browser: ## Install Playwright Chromium browser
	@echo "🌐 Installing Playwright Chromium..."
	cd $(E2E_DIR) && pnpm exec playwright install chromium

e2e-kind-up: e2e-setup-dex ## Create or reuse the OIDC-enabled local kind cluster for e2e
	E2E_KIND_NAME="$(E2E_KIND_NAME)" E2E_KUBECONFIG="$(E2E_KUBECONFIG)" \
		E2E_NODE_IMAGE="$(E2E_NODE_IMAGE)" KITE_E2E_OIDC_CERT_DIR="$(E2E_OIDC_CERT_DIR)" ./scripts/e2e-kind-up.sh

e2e-kind-down: ## Delete the local kind cluster used by e2e
	@docker rm -f "$(E2E_OIDC_FORWARDER)" >/dev/null 2>&1 || true
	@if kind get clusters | grep -qx "$(E2E_KIND_NAME)"; then \
		echo "🧹 Deleting kind cluster $(E2E_KIND_NAME)..."; \
		kind delete cluster --name "$(E2E_KIND_NAME)"; \
	else \
		echo "ℹ️ kind cluster $(E2E_KIND_NAME) does not exist"; \
	fi
	rm -f "$(E2E_KUBECONFIG)"

e2e-stop-app: ## Stop any local e2e app process listening on the e2e port
	@PIDS=$$(lsof -tiTCP:$(E2E_PORT) -sTCP:LISTEN 2>/dev/null || true); \
	if [ -n "$$PIDS" ]; then \
		echo "🛑 Stopping local e2e app on port $(E2E_PORT)..."; \
		kill $$PIDS 2>/dev/null || true; \
		sleep 1; \
	fi

e2e-setup-dex: ## Start the standards-based OIDC provider used by e2e
	@KITE_E2E_OIDC_CERT_DIR="$(E2E_OIDC_CERT_DIR)" ./scripts/e2e-generate-oidc-certs.sh >/dev/null
	@docker network inspect "$(E2E_AUTH_NETWORK)" >/dev/null 2>&1 || docker network create "$(E2E_AUTH_NETWORK)" >/dev/null
	@docker volume inspect "$(E2E_DEX_VOLUME)" >/dev/null 2>&1 || docker volume create "$(E2E_DEX_VOLUME)" >/dev/null
	@docker rm -f "$(E2E_DEX_CONTAINER)" >/dev/null 2>&1 || true
	docker run -d --name "$(E2E_DEX_CONTAINER)" \
		--network "$(E2E_AUTH_NETWORK)" \
		-p "$(E2E_OAUTH_PORT):5556" \
		-v "$(CURDIR)/e2e/fixtures/dex/config.yaml:/etc/dex/config.yaml:ro" \
		-v "$(E2E_OIDC_CERT_DIR):/etc/dex/tls:ro" \
		-v "$(E2E_DEX_VOLUME):/var/dex" \
		ghcr.io/dexidp/dex:v2.45.1 dex serve /etc/dex/config.yaml
	@for i in $$(seq 1 60); do \
		if curl --cacert "$(E2E_OIDC_CERT_DIR)/ca.crt" -fsS "https://localhost:$(E2E_OAUTH_PORT)/.well-known/openid-configuration" >/dev/null 2>&1; then \
			break; \
		fi; \
		sleep 1; \
	done
	curl --cacert "$(E2E_OIDC_CERT_DIR)/ca.crt" -fsS "https://localhost:$(E2E_OAUTH_PORT)/.well-known/openid-configuration" >/dev/null

e2e-run: e2e-kind-up e2e-stop-app ## Run e2e tests against the local kind cluster
	@echo "🧪 Running e2e tests..."
	cd $(E2E_DIR) && KUBECONFIG="$(E2E_KUBECONFIG)" KITE_E2E_CLUSTER_NAME="$(E2E_KIND_NAME)" KITE_E2E_PORT="$(E2E_PORT)" pnpm exec playwright test $(SPEC)

e2e-run-headed: e2e-kind-up e2e-stop-app ## Run e2e tests in headed mode against the local kind cluster
	@echo "🧪 Running headed e2e tests..."
	cd $(E2E_DIR) && KUBECONFIG="$(E2E_KUBECONFIG)" KITE_E2E_CLUSTER_NAME="$(E2E_KIND_NAME)" KITE_E2E_PORT="$(E2E_PORT)" pnpm exec playwright test --headed $(SPEC)

e2e-test: e2e-install e2e-install-browser e2e-run ## Run e2e tests against the local kind cluster

e2e-test-headed: e2e-install e2e-install-browser e2e-run-headed ## Run e2e tests in headed mode against the local kind cluster

docs-dev: ## Start documentation server in development mode
	@echo "📚 Starting documentation server..."
	cd docs && pnpm run docs:dev
docs-build: ## Build documentation
	@echo "📚 Building documentation..."
	cd docs && pnpm run docs:build
