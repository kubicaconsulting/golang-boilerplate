GO ?= go
GOLANGCI_VERSION := v2.15.0
GOVULNCHECK_VERSION := v1.9.0
TASK_VERSION := v3.54.0
AIR_VERSION := v1.67.4
MOCKERY_VERSION := v3.9.0
.PHONY: test-coverage coverage-check mocks mocks-check hooks-install lint-config-check lint-fix dev-postgres version release-image dev docker-up docker-dev docker-down docker-config deps-check deps-inventory run build fmt format-check vet lint test test-unit test-race test-integration test-e2e openapi contract-check check vuln migrate task clean
mocks:
	$(GO) run github.com/vektra/mockery/v3@$(MOCKERY_VERSION) --config .mockery.yml
mocks-check:
	GO="$(GO)" MOCKERY_VERSION="$(MOCKERY_VERSION)" ./scripts/check-mocks.sh
hooks-install:
	git config --local core.hooksPath .githooks
dev-postgres:
	GO="$(GO)" ./scripts/dev-postgres.sh
dev:
	$(GO) run github.com/air-verse/air@$(AIR_VERSION) -c .air.toml
docker-up:
	docker compose up --build
docker-dev:
	docker compose -f compose.yaml -f compose.dev.yaml up --build
docker-down:
	docker compose -f compose.yaml -f compose.dev.yaml down
docker-config:
	docker compose config --quiet
	docker compose -f compose.yaml -f compose.dev.yaml config --quiet
deps-check:
	$(GO) run ./cmd/dependencies
deps-inventory:
	$(GO) run ./cmd/dependencies -write
run:
	$(GO) run ./cmd/service
build:
	GO="$(GO)" ./scripts/build.sh
version:
	$(GO) run ./cmd/service --version
release-image:
	./scripts/release-image.sh
fmt:
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) fmt
format-check:
	@tmp=$$(mktemp); $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) fmt --diff > "$$tmp"; status=$$?; if [ "$$status" -eq 0 ] && [ -s "$$tmp" ]; then status=1; fi; cat "$$tmp"; rm -f "$$tmp"; exit "$$status"
lint-config-check:
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) config verify
lint-fix:
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) run --fix
vet:
	$(GO) vet ./...
lint:
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION) run
test: test-unit
test-unit:
	$(GO) test ./...
test-race:
	$(GO) test -race ./...
test-coverage:
	@mkdir -p coverage
	@rm -f coverage/coverage.out coverage/functions.txt coverage/index.html
	$(GO) test -race -count=1 -covermode=atomic -coverpkg=./internal/... -coverprofile=coverage/coverage.out ./...
	$(GO) tool cover -func=coverage/coverage.out > coverage/functions.txt
	$(GO) tool cover -html=coverage/coverage.out -o coverage/index.html
	$(MAKE) coverage-check
coverage-check:
	./scripts/check-coverage.sh
test-integration:
	$(GO) test -tags=integration ./tests/integration -count=1 -parallel=4
test-e2e:
	$(GO) test -tags=e2e ./tests/e2e -count=1 -parallel=4
openapi:
	@mkdir -p api
	@tmp=$$(mktemp); $(GO) run ./cmd/openapi > "$$tmp" && mv "$$tmp" api/openapi.json || { rm -f "$$tmp"; exit 1; }
contract-check:
	@tmp=$$(mktemp); $(GO) run ./cmd/openapi > "$$tmp" && diff -u api/openapi.json "$$tmp"; status=$$?; rm -f "$$tmp"; exit $$status
check: lint-config-check format-check mocks-check deps-check vet lint test-unit contract-check
vuln:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...
migrate:
	$(GO) run ./cmd/service --migrate
task:
	$(GO) run github.com/go-task/task/v3/cmd/task@$(TASK_VERSION) $(ARGS)
clean:
	rm -rf bin .air
