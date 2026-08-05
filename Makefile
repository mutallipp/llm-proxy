.PHONY: generate build backend frontend cleanup-db \
	test-backend-all \
	e2e-test e2e-backend-start e2e-backend-stop e2e-backend-status e2e-backend-restart e2e-backend-clean \
	migration-test migration-test-all migration-test-all-dbs \
	sync-faq sync-models filter-logs \
	lint lint-privacy \
	generate-schema \
	start stop restart logs status \
	dev-up dev-down dev-logs dev-restart dev-clean dev-frontend dev

# Generate GraphQL and Ent code
generate:
	@echo "Generating GraphQL and Ent code..."
	cd internal/server/gql && go generate
	@echo "Generation completed!"

generate-openapi:
	@echo "Generating GraphQL and Ent code..."
	cd internal/server/gql/openapi && go generate
	@echo "Generation completed!"

# Build the backend application
build-backend:
	@echo "Building llm-proxy backend..."
	go build -ldflags "-s -w" -tags=nomsgpack -o llm-proxy ./cmd/llm-proxy
	@echo "Backend build completed!"

# Build the frontend application
build-frontend:
	@echo "Building llm-proxy frontend..."
	cd frontend && pnpm vite build
	@echo "Copying frontend dist to server static directory..."
	rm -rf internal/server/static/dist/assets
	mkdir -p internal/server/static/dist
	cp -r frontend/dist/* internal/server/static/dist/
	@echo "Frontend build completed!"

# Build both frontend and backend
build: build-frontend build-backend
	@echo "Full build completed!"

# Cleanup test database - remove all playwright test data
cleanup-db:
	@echo "Cleaning up playwright test data from database..."
	@sqlite3 llm-proxy.db "DELETE FROM user_roles WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'pw-test-%' OR first_name LIKE 'pw-test%');"
	@sqlite3 llm-proxy.db "DELETE FROM user_projects WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'pw-test-%' OR first_name LIKE 'pw-test%');"
	@sqlite3 llm-proxy.db "DELETE FROM user_projects WHERE project_id IN (SELECT id FROM projects WHERE slug LIKE 'pw-test-%' OR name LIKE 'pw-test-%');"
	@sqlite3 llm-proxy.db "DELETE FROM api_keys WHERE name LIKE 'pw-test-%';"
	@sqlite3 llm-proxy.db "DELETE FROM api_keys WHERE user_id IN (SELECT id FROM users WHERE email LIKE 'pw-test-%' OR first_name LIKE 'pw-test%');"
	@sqlite3 llm-proxy.db "DELETE FROM api_keys WHERE project_id IN (SELECT id FROM projects WHERE slug LIKE 'pw-test-%' OR name LIKE 'pw-test-%');"
	@sqlite3 llm-proxy.db "DELETE FROM roles WHERE code LIKE 'pw-test-%' OR name LIKE 'pw-test-%';"
	@sqlite3 llm-proxy.db "DELETE FROM roles WHERE project_id IN (SELECT id FROM projects WHERE slug LIKE 'pw-test-%' OR name LIKE 'pw-test-%');"
	@sqlite3 llm-proxy.db "DELETE FROM usage_logs WHERE project_id IN (SELECT id FROM projects WHERE slug LIKE 'pw-test-%' OR name LIKE 'pw-test-%');"
	@sqlite3 llm-proxy.db "DELETE FROM requests WHERE project_id IN (SELECT id FROM projects WHERE slug LIKE 'pw-test-%' OR name LIKE 'pw-test-%');"
	@sqlite3 llm-proxy.db "DELETE FROM users WHERE email LIKE 'pw-test-%' OR first_name LIKE 'pw-test%';"
	@sqlite3 llm-proxy.db "DELETE FROM projects WHERE slug LIKE 'pw-test-%' OR name LIKE 'pw-test-%';"
	@echo "Cleanup completed!"

# --- Testing ---

# Run all backend tests across all Go modules
test-backend-all:
	@echo "Running all backend tests..."
	@echo ""
	@echo "=== Testing root module ==="
	go test ./...
	@echo "=== Testing llm module ==="
	cd llm && go test ./...
	@echo ""
	@echo "All backend tests completed!"

# --- E2E Testing ---

# Run the full E2E test suite
e2e-test:
	@echo "Running E2E tests..."
	@./scripts/e2e/e2e-test.sh

# Start the E2E backend service
e2e-backend-start:
	@echo "Starting E2E backend..."
	@./scripts/e2e/e2e-backend.sh start

# Stop the E2E backend service
e2e-backend-stop:
	@echo "Stopping E2E backend..."
	@./scripts/e2e/e2e-backend.sh stop

# Check E2E backend status
e2e-backend-status:
	@./scripts/e2e/e2e-backend.sh status

# Restart the E2E backend service
e2e-backend-restart:
	@echo "Restarting E2E backend..."
	@./scripts/e2e/e2e-backend.sh restart

# Clean up E2E test files
e2e-backend-clean:
	@echo "Cleaning up E2E test files..."
	@./scripts/e2e/e2e-backend.sh clean

# --- Migration Testing ---

# Test database migration from a specific tag
# Usage: make migration-test TAG=v0.1.0
migration-test:
	@if [ -z "$(TAG)" ]; then echo "Error: TAG is required. Usage: make migration-test TAG=v0.1.0"; exit 1; fi
	@echo "Running migration test from $(TAG)..."
	@./scripts/migration/migration-test.sh $(TAG)

# Run migration tests for all recent stable versions
migration-test-all:
	@echo "Running migration tests for all versions..."
	@./scripts/migration/migration-test-all.sh

# Test migration across all supported database types
# Usage: make migration-test-all-dbs TAG=v0.1.0
migration-test-all-dbs:
	@if [ -z "$(TAG)" ]; then echo "Error: TAG is required. Usage: make migration-test-all-dbs TAG=v0.1.0"; exit 1; fi
	@echo "Running migration tests across all DBs from $(TAG)..."
	@./scripts/migration/test-migration-all-dbs.sh $(TAG)

# --- Data Syncing ---

# Sync FAQ from GitHub issues
sync-faq:
	@echo "Syncing FAQ from GitHub..."
	@node ./scripts/sync/sync-github-faq.js

# Sync model developers data
sync-models:
	@echo "Syncing model developers..."
	@node ./scripts/sync/sync-model-developers.js

# --- Utilities ---

# Filter and analyze load balance logs
filter-logs:
	@echo "Filtering load balance logs..."
	@./scripts/utils/filter-load-balance-logs.sh

# --- Linting ---

GO_LINT_CMD = golangci-lint run --timeout 10m --max-same-issues 50 --new --fix ./...

GO_MODULES := . llm

lint-all:
	@echo "Running golangci-lint (checking and fixing new code) across all Go modules..."
	@for module in $(GO_MODULES); do \
		echo ""; \
		echo "=== Linting $$module module ==="; \
		if [ -f "$$module/go.mod" ]; then \
			cd $$module && $(GO_LINT_CMD) && cd - > /dev/null; \
		else \
			$(GO_LINT_CMD); \
		fi; \
	done
	@echo ""
	@echo "All lint checks passed!"

# Generate JSON schema for configuration
generate-schema:

# ── Production (docker-compose.yml, port 8090) ─────────────────
start:        ## Stop any running prod, then build + start fresh (port 8090)
	@if docker ps --format '{{.Names}}' | grep -qx 'llm-proxy'; then \
		echo "==> Stopping existing llm-proxy container..."; \
		docker compose stop llm-proxy; \
		docker compose rm -f llm-proxy; \
	fi
	docker compose build llm-proxy
	docker compose up -d llm-proxy

stop:         ## Stop prod container (keeps image)
	docker compose stop llm-proxy

restart:      ## Force-recreate prod container
	docker compose up -d --force-recreate llm-proxy

logs:         ## Tail prod container logs
	docker compose logs -f llm-proxy

status:       ## Show prod + dev container status
	@echo "== prod =="; docker ps -a --filter 'name=llm-proxy$$' --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}' || true
	@echo "== dev ==";  docker ps -a --filter 'name=llm-proxy-dev' --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}' || true

# ── Development (docker-compose.dev.yml, port 18090) ─────────
dev-up:       ## Stop any running dev, then build + start fresh (port 18090)
	@if docker ps --format '{{.Names}}' | grep -qx 'llm-proxy-dev'; then \
		echo "==> Stopping existing llm-proxy-dev container..."; \
		docker compose -f docker-compose.dev.yml stop llm-proxy; \
		docker compose -f docker-compose.dev.yml rm -f llm-proxy; \
	fi
	docker compose -f docker-compose.dev.yml build llm-proxy
	docker compose -f docker-compose.dev.yml up -d llm-proxy

dev-down:     ## Stop + remove dev container
	docker compose -f docker-compose.dev.yml down

dev-logs:     ## Tail dev container logs
	docker compose -f docker-compose.dev.yml logs -f llm-proxy

dev-restart:  ## Force-recreate dev container
	docker compose -f docker-compose.dev.yml up -d --force-recreate llm-proxy

dev-clean:    ## Stop dev + remove dev image (drops dev DB manually: DROP DATABASE "llm-proxy-dev")
	docker compose -f docker-compose.dev.yml down --rmi local

dev-frontend: ## Run frontend Vite dev server (assumes dev backend on 18090)
	cd frontend && VITE_API_URL=http://localhost:18090 pnpm dev --port 15173

dev:          ## Start dev backend, then run Vite dev in foreground (Ctrl+C exits frontend; run dev-down to stop backend)
	@echo "Starting dev backend on :18090 (Ctrl+C exits frontend only, run 'make dev-down' to stop backend)..."
	$(MAKE) dev-up
	$(MAKE) dev-frontend

	@echo "Generating JSON schema for configuration..."
	@cd cmd/schema && go run . > ../../config.schema.json
	@echo "JSON schema generated at config.schema.json"

# Run all lint checks
lint: lint-all lint-privacy
	@echo "All lint checks passed!"

# Check for illegal privacy.DecisionContext(...Allow) usage
lint-privacy:
	@echo "Checking for illegal privacy.DecisionContext(...Allow) usage..."
	@./scripts/lint/check-privacy-allow.sh
