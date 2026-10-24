# HTTP API template

A Go HTTP API starter with hexagonal architecture, one sample resource (Dummy), Postgres reads, Kafka writes, JWT, and OpenAPI. Writes return `202 Accepted` after Kafka acknowledges publication; reads are eventually consistent.

## 1. Copy and rename

Copy `service/` into your new repository root. Copy source and explicit env examples only: leave out local `.env` variants, certificates, broker state (keep only its `.gitkeep`), binaries, `tmp/`, and coverage output. The ignore files describe these exclusions; copying a directory does not automatically apply them.

Use Go 1.26.10 or a newer compatible release and Make. Both modules require the patched 1.26.10 minimum; CI reads it from `go.mod`, and the container examples use 1.26.10. From the copied root:

```sh
go mod edit -module example.com/your-team/your-api
go -C tools mod edit -module example.com/your-team/your-api/tools
```

Replace `http-api-template/` imports and package paths in `.mockery.yml` with your module path using the IDE, then run `make mocks`. Rename the Compose project `name` in both Compose files, the API service key and any references if desired, and the production image name. If you rename the binary, update both Dockerfiles, `environment/dev/.air.toml`, and its ignore entry together. Keep the `tools/` module separate.

## 2. Replace Dummy across layers

Replace Dummy's domain model and validation in `internal/core/domain`, its ports in `internal/core/port`, and its command/event-processing services and tests in `internal/core/service`. Update the interface allowlist in `.mockery.yml` and regenerate mocks. Then update the Postgres model/repository, Kafka event codec and dispatcher, HTTP mappings, and composition in `cmd/api`.

Replace the split request/response/route YAML under `api/openapi/v1/` and its references in `openapi.yaml`. Change the table and constraints in `database/migrations/`: edit the initial migration only for a fresh, unapplied fork; use a new migration once a database has applied it. Update topic/group examples and tests alongside the resource. Do not migrate an existing application's data by renaming sample types.

## 3. Architecture and ownership

```text
cmd/api                         configuration, dependency wiring, lifecycle
internal/core/domain            resource values, patches, validation, errors
internal/core/port              inbound services and outbound dependencies
internal/core/service           commands, event processing, auth stub
internal/adapter/http           Gin handlers, middleware, generated DTOs/spec
internal/adapter/kafka          publication, event codec, sequential consumer
internal/adapter/postgres       GORM repository and connection handling
internal/adapter/auth            JWT issuance and verification
internal/adapter/logger, uuid   JSON slog and ID generation
```

HTTP handlers call `DummyService`. Reads use `DummyRepository`; writes use `EventPublisher` and never mutate Postgres. The consumer invokes `DummyProcessor`, which performs repository mutations. HTTP DTOs, GORM models, and Kafka envelopes stay in adapters; core imports only the standard library and core packages.

Extend domain rules and services where business invariants belong. Implement outbound ports in adapters and inject them from `cmd/api`; adapters do not open dependencies from handlers or read process environment themselves. Keep request-ID context framework-free. Auth issuance and verification are separate ports, even though one JWT adapter implements both.

## 4. Regenerate OpenAPI

Edit YAML first, then run from the copied root:

```sh
make docs
```

Pinned tools from `tools/go.mod` merge the split spec and generate `internal/adapter/http/view/types.gen.go` and `internal/adapter/http/spec.gen.go`. Commit regenerated output with the YAML change. Never edit `*.gen.go` by hand. Swagger is at `/api/v1/documentation/`; the JSON spec is at `/api/v1/docs.json`.

## 5. Test services before connecting routes

Add repository, publisher, issuer, and ID-generator mock expectations alongside core services before wiring new HTTP handlers. Cover validation, dependency failures, replay/no-op behavior, and explicit empty patch values. Test HTTP mapping/auth with `httptest` and fake services; test Kafka processing and commit retries separately. Normal checks require no database, broker, Docker, TLS certificates, or `.env`.

Run `make mocks` after changing an interface. Mockery v3.5.5 is pinned in `tools/go.mod`; no global installation is needed. `.mockery.yml` explicitly selects core ports (generated into `internal/mocks/port`) and Kafka reader/writer interfaces (generated into adapter-local `mock_*_test.go` files). Commit regenerated output with interface changes and never edit generated mocks by hand. If an interface is removed or renamed, remove its obsolete generated file as well. Generation runs independently of tests, OpenAPI, and infrastructure.

Use a fresh mock per test or subtest. Constructors register expectation checks with test cleanup; unexpected calls fail even when no expectations were registered, which checks validation short circuits. For example:

```go
issuer := portmocks.NewMockTokenIssuer(t) // http-api-template/internal/mocks/port
issuer.EXPECT().Issue(ctx, email).Return(wantToken, nil).Once()
token, err := NewAuth(issuer).Login(ctx, email, password)
// Check token and err as usual.
```

Variadic arguments are expanded: expect each Kafka message or structured log argument separately. Stateful Kafka consumer fakes remain for retry/cancellation scenarios; Postgres tests retain SQLMock.

```sh
make test                         # go test -race ./...
make lint                         # pinned golangci-lint
govulncheck="$(go -C tools tool -n govulncheck)"
"$govulncheck" ./...
go build -o /dev/null ./cmd/api
```

In workspaces where Go cannot obtain VCS status, use `go build -buildvcs=false -o /dev/null ./cmd/api`. CI builds normally in a Git checkout. CI lives under `.github/workflows/ci.yml` and becomes active when this folder is the repository root. Its race-test, lint, vulnerability, and build jobs run independently on Ubuntu with Go selected from `go.mod`; caches cover both modules. Vulnerability findings fail the check and require assessment/fixes before publication.

## Configuration

Like the sibling templater, the application optionally reads `.env.dev` by default, or `.env.prod` when the process starts with `DEBUG=false`. `ENV_FILE` explicitly selects a file and takes precedence over that selector. Application and migration configuration use the same selection. Existing environment variables override file values without modifying the process environment. Missing files are allowed; malformed present files or invalid configuration fail. The old `.env` is no longer loaded. Ports must be integers from 1 to 65535. Never commit or paste local env contents or secrets.

| Variable | Requirement/default |
| --- | --- |
| `ENV_FILE` | Optional dotenv path; otherwise `.env.dev`, or `.env.prod` when process `DEBUG=false` |
| `HTTP_PORT` | Required HTTP/TLS listen port; example `8081` |
| `DEBUG` | Boolean; defaults `false`. `true` selects HTTP, `false` selects TLS |
| `SSL_CERT_PATH`, `SSL_KEY_PATH` | Required when `DEBUG=false`; readable certificate/key paths |
| `POSTGRES_HOST`, `POSTGRES_PORT` | Required; example host processes use `localhost:5432` |
| `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | Required database credentials/name |
| `POSTGRES_SSLMODE` | Defaults `disable`; allowed: `disable`, `allow`, `prefer`, `require`, `verify-ca`, `verify-full` |
| `POSTGRES_URL` | Optional migration-only DSN override; not an application DB override |
| `KAFKA_BROKER` | Required single broker address; example host processes use `localhost:9092` |
| `KAFKA_EXTERNAL_PORT` | Compose-only host broker port; development `9092`, production example `9094` |
| `KAFKA_TOPIC` | Required; examples `dummy-events-dev` / `dummy-events-prod` |
| `KAFKA_GROUP_ID` | Defaults `dummy-service` |
| `JWT_SECRET` | Required nonblank HS256 signing secret; replace the example placeholder |

The API requires all application values even when a migration-only DSN is set. Migration commands need only Postgres settings or `POSTGRES_URL`; HTTP, Kafka, JWT, and TLS settings are unnecessary for them.

## Local startup

Install Docker and Compose v2 (`docker compose`) for these optional local examples. Run from the copied root:

```sh
cp .env.dev.example .env.dev
cp .env.prod.example .env.prod
# Set independent Postgres passwords and JWT secrets in each file.
make init
```

`make init` regenerates OpenAPI, waits for Postgres/Kafka, creates the topic, runs migrations, and starts the development API. The individual commands are:

```sh
docker compose --env-file .env.dev up -d --wait postgres kafka
docker compose --env-file .env.dev run --rm kafka-init
make migrate-up
make up
```

For host execution, run `go run ./cmd/api` instead of `make up` after the same infrastructure and migration commands. The example env uses localhost; Compose overrides API connections to `postgres:5432` and `kafka:9092`. Kafka advertises separate host/internal listeners. `kafka-init` creates the configured topic with three partitions and replication factor one if absent; it does not change an existing topic's partition count.

Use `make up` on later starts and apply new migrations before starting the API. `make stop` stops development services without deleting data. HTTP documentation and health are at `http://localhost:8081/api/v1/documentation/` and `http://localhost:8081/api/v1/health`. Development uses Air/Delve with a localhost debugger on port 2345; the mounted workspace and `tmp/` must be writable by UID 1000.

The standalone TLS example uses `make init-prod`, which selects `.env.prod` for Compose interpolation, API env_file, and host migrations. First provide `certs/server.crt` and `certs/server.key`, readable by runtime UID 65532, with a certificate valid for localhost. Compose forces `DEBUG=false` and container certificate paths; the production example serves `https://localhost:8443`, publishes Postgres on `5433` and Kafka on `9094`, and uses separate database/user/topic/group names and secrets. On subsequent production starts use `make ENV_FILE=.env.prod up`; stop with `make ENV_FILE=.env.prod stop`. Production migrations use `make ENV_FILE=.env.prod migrate-up`. For host API execution use `DEBUG=false go run ./cmd/api` or `ENV_FILE=.env.prod go run ./cmd/api`. This is a single-node local example, without production credential verification or broker security.

Compose projects are `http-api-template-dev` and `http-api-template-prod` so container networks and named volumes are separate. Postgres uses named volumes; development Kafka keeps `environment/dev/kafka-data`, while production uses a named Kafka volume. These new project names select different Postgres volumes from the former shared project; existing volumes and the legacy `.env` are not migrated or deleted. Review existing state before switching. Storage must match the pinned Postgres 16/Kafka 3.9.1 formats. Ignore files exclude runtime env files, certs, broker state, build output, and coverage, retaining only explicit env examples and `.gitkeep`.

## Database migrations

Goose is pinned in `tools/go.mod`. Startup does not migrate automatically. The one reversible initialization migration creates `dummies` with UUID primary key, unique nonblank name, and non-null note defaulting to `""`; it inserts no sample rows.

```sh
make migrate-create NAME=add_resource  # add goose Up and Down SQL
make migrate-up
make migrate-status
make migrate-version
make migrate-down                     # rolls back one migration; can remove data
```

For host commands, use a host-resolvable Postgres address and its published port. `POSTGRES_URL` overrides individual Postgres settings for migration commands only. Individual credentials are URL-escaped by `cmd/pgurl`; do not print the resulting credential-bearing URL. Configure `POSTGRES_SSLMODE` for remote databases. Review both forward and rollback SQL before applying changes.

## HTTP contract

| Method/path | Access | Successful response |
| --- | --- | --- |
| `GET /api/v1/health` | Public | `200 {"status":200}`; no DB/Kafka readiness check |
| `POST /api/v1/auth/login` | Public | `200` token response |
| `GET /api/v1/dummies/{id}` | Public | `200` with `id`, `name`, `note` |
| `POST /api/v1/dummies` | JWT | `202 {"id":"<uuid>"}` |
| `PATCH /api/v1/dummies/{id}` | JWT | `202` with empty body |
| `DELETE /api/v1/dummies/{id}` | JWT | `202` with empty body |
| `GET /api/v1/documentation/`, `GET /api/v1/docs.json` | Public | Swagger UI / JSON spec |

Dummy has a UUID string ID, required unique case-sensitive `name`, and optional `note` represented as `""` when absent. POST accepts `{"name":"Example","note":"optional"}`. PATCH accepts `name` and/or `note`; omitted fields stay unchanged and `{"note":""}` clears the note. Invalid UUIDs, blank names, empty patches, explicit null fields, and malformed input return `400`.

Writes return success only after Kafka acknowledges publication. `202` does not confirm persistence: a following GET can return stale data or `404`. Create requests check Postgres first and return `409` without an ID or publication when the name already exists. This check cannot detect concurrent pending creates before persistence. Duplicate create events by ID or name are successful no-ops; updates/deletes against missing IDs are also no-ops. An update name conflict is logged and skipped by the consumer, not returned synchronously as `409`. Missing GET resources return `404`; unexpected service/publication errors return `500`; error bodies are empty.

Login is a demonstration stub: `{"email":"developer@example.com","password":"example-password"}` validates an email and a nonempty password, but checks no credentials and stores no users. It issues an HS256 token valid for one hour (`access_token`, `token_type`, `expires_in`). Replace this stub before exposing authenticated business operations. Send protected requests with the exact header syntax:

```http
Authorization: Bearer <access_token>
```

Verification enforces HS256, signature validity, and expiry; missing/invalid tokens return `401`.

## Kafka delivery and correlation

One sequential dispatcher per instance uses the configured group. Each event contains `event_type` (`create`, `update`, `delete`), `dummy_id`, and a create `dummy` or update `patch`; delete has no payload. Create's nested ID must match the envelope ID. Validation rejects invalid UUIDs, blank names, null/unknown fields, and malformed payloads.

The synchronous writer requires acknowledgments from all in-sync replicas and hashes the lowercase UUID key. Keep topic partition counts stable to preserve per-ID ordering; external producers must use the same key/partitioning policy. Delivery is at least once: a crash after processing and before commit can replay an event. Duplicate creates and missing-target mutations are idempotent no-ops.

Offsets commit explicitly after successful processing. Transient read/process/commit errors retry with cancellable exponential backoff from 100ms to 5s; commit retries do not repeat processing that already succeeded in that run. A failing message blocks this instance. Malformed, invalid, unknown, or conflicting events are logged and committed; there is no dead-letter topic or automatic recovery of skipped events.

HTTP accepts a single `X-Request-ID` of 1–128 ASCII letters, digits, dots, underscores, or hyphens; otherwise it generates a UUID. Responses echo the ID, which travels through context and the Kafka `request_id` header. Consumers generate missing/invalid IDs once per message and retain them across retries. Logs use structured JSON slog with request correlation and redact sensitive attributes; do not add passwords, tokens, authorization headers, or full event payloads to logs.

## Shutdown

SIGINT/SIGTERM drain HTTP first, then cancel/wait for the consumer and reader close, close the Kafka writer, and close the SQL pool. Each phase has a fresh 10-second bound and later cleanup still runs after failure. A drain timeout force-closes HTTP; blocking context-free Close calls are bounded by waiting, not by cancelling the call itself. Runtime/cleanup errors and timeouts produce a nonzero exit. Expected cancellation and `http.ErrServerClosed` are normal. Compose allows 45 seconds before killing the process. Pending events may remain for restart; shutdown does not drain the entire topic.

See [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md), and the [MIT license](LICENSE).
