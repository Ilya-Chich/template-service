ENV_FILE ?= $(if $(filter init-prod,$(MAKECMDGOALS)),.env.prod,.env.dev)
-include $(ENV_FILE)
export ENV_FILE
TOOLBIN = go -C tools tool -n
OAPI_CODEGEN = $(shell $(TOOLBIN) oapi-codegen)
MERGED_OAPI_V1=$(CURDIR)/api/openapi/v1/merged.json
OAPI_MERGER = $(shell $(TOOLBIN) oapi-merger)
GOLANGCI = $(shell $(TOOLBIN) golangci-lint)
GOOSE = $(shell $(TOOLBIN) goose)
GOVULNCHECK = $(shell $(TOOLBIN) govulncheck)

COMPOSE_FILE ?= $(if $(filter .env.prod,$(ENV_FILE)),docker-compose.prod.yml,docker-compose.yml)
COMPOSE = docker compose --env-file $(ENV_FILE) -f $(COMPOSE_FILE)

.PHONY: init init-prod up stop docker-up build postgres-init
# Compose interpolation and migration configuration use the same selected file.
init: docs
	$(COMPOSE) up -d --wait postgres kafka
	$(COMPOSE) run --rm kafka-init
	$(MAKE) migrate-up
	$(COMPOSE) up --build api

init-prod: docs
	$(COMPOSE) up -d --wait postgres kafka
	$(COMPOSE) run --rm kafka-init
	$(MAKE) migrate-up
	$(COMPOSE) up -d --build api

up: docker-up

stop:
	$(COMPOSE) stop

docker-up:
	$(COMPOSE) up --build -d

build:
	$(COMPOSE) build

.PHONY: postgres-init
postgres-init:
	$(COMPOSE) up -d --wait postgres

.PHONY: docs
docs: openapi_http

.PHONY: openapi_merge
openapi_merge:
	$(OAPI_MERGER) -wdir api/openapi/v1 -spec openapi.yaml -o $(MERGED_OAPI_V1)

.PHONY: openapi_http
openapi_http: openapi_merge
	mkdir -p internal/adapter/http/view
	$(OAPI_CODEGEN) --old-config-style  -generate types,skip-prune -o ./internal/adapter/http/view/types.gen.go -package view $(MERGED_OAPI_V1)
	$(OAPI_CODEGEN) --old-config-style -generate spec -o ./internal/adapter/http/spec.gen.go -package http $(MERGED_OAPI_V1)
	rm -f $(MERGED_OAPI_V1)

.PHONY: lint
lint:
	$(GOLANGCI) run

.PHONY: test
test:
	go test -race ./...

export MIGRATION_NAME = $(NAME)
.PHONY: migrate-create migrate-up migrate-down migrate-status migrate-version
migrate-create:
	@case "$$MIGRATION_NAME" in ''|*[!a-zA-Z0-9_]*) echo "Usage: make migrate-create NAME=your_migration_name"; exit 1;; esac
	@"$(GOOSE)" -dir database/migrations create "$$MIGRATION_NAME" sql

define migrate
	@dsn="$$(go run ./cmd/pgurl)" || exit $$?; \
	"$(GOOSE)" -dir database/migrations postgres "$$dsn" $(1)
endef

migrate-up:
	$(call migrate,up)

migrate-down:
	$(call migrate,down)

migrate-status:
	$(call migrate,status)

migrate-version:
	$(call migrate,version)