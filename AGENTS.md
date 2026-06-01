# HyperFleet API — Agent Instructions

Stateless REST API serving as the pure CRUD data layer for HyperFleet cluster lifecycle management. Persists clusters, node pools, generic resources, and adapter statuses to PostgreSQL — no business logic, no events. Sentinel handles orchestration; adapters execute and report back.

- **Language**: Go 1.24+ with FIPS crypto (`CGO_ENABLED=1 GOEXPERIMENT=boringcrypto`)
- **Database**: PostgreSQL 14.2 with GORM ORM
- **API Spec**: TypeSpec → `hyperfleet-api-spec` Go module → oapi-codegen → Go models
- **Architecture**: Plugin-based route registration, transaction-per-request middleware

**Request flow**: Router → Middleware (logging, auth, transaction) → Handler → Service → DAO → GORM → PostgreSQL

Transaction middleware creates GORM transactions for **write requests only** (POST/PUT/PATCH/DELETE). Read requests (GET) skip transaction creation. Status aggregation: service layer synthesizes `Available` and `Ready` conditions from adapter reports.

## Verification

**Generated code is not checked into git.** Run `make generate-all` before building, testing, or even `go mod download`.

Run `make verify-all` before declaring work done. It runs everything without a database:

| Target | What it does | Requires DB? |
|---|---|---|
| `make verify-all` | `verify` + `lint` + `test` (all-in-one) | No |
| `make verify` | go vet + gofmt check | No |
| `make lint` | golangci-lint (config: `.golangci.yml`) | No |
| `make test` | Unit tests (`HYPERFLEET_ENV=unit_testing`) | No |
| `make test-integration` | Integration tests (testcontainers) | No (auto-creates) |
| `make test-helm` | Helm chart lint + template validation | No |
| `make test-all` | `lint` + `test` + `test-integration` + `test-helm` | Auto-creates |

Setup sequence for a fresh clone:

```
make generate-all     # REQUIRED FIRST — generated code not in git
go mod download
make secrets          # Initialize secrets/ with defaults
make db/setup         # Start local PostgreSQL container
make build            # Build binary (CGO_ENABLED=1 GOEXPERIMENT=boringcrypto)
./bin/hyperfleet-api migrate
make run-no-auth      # Start server without auth
```

## Source of Truth

| Topic | Location |
|---|---|
| Entry point + subcommands (serve, migrate) | `cmd/hyperfleet-api/` |
| Environment configs | `cmd/hyperfleet-api/environments/` |
| HTTP handler pipeline | `pkg/handlers/framework.go` |
| Cluster handlers | `pkg/handlers/cluster.go`, `cluster_nodepools.go`, `cluster_status.go` |
| Node pool handlers | `pkg/handlers/node_pool.go`, `nodepool_status.go` |
| Generic resource handler | `pkg/handlers/resource_handler.go` |
| Request validation | `pkg/handlers/validation.go` |
| Service interfaces + implementations | `pkg/services/` |
| Generic resource service | `pkg/services/resource.go` |
| Status aggregation logic | `pkg/services/aggregation.go` |
| DAO interfaces + implementations | `pkg/dao/` |
| Generic resource DAO | `pkg/dao/resource.go` |
| Generated OpenAPI models + spec | `pkg/api/openapi/` (never edit) |
| Presenters (API response formatting) | `pkg/api/presenters/` |
| ServiceError type + RFC 9457 | `pkg/errors/errors.go` |
| Structured logging | `pkg/logger/logger.go` |
| SessionFactory + transaction middleware | `pkg/db/` |
| Database migrations | `pkg/db/migrations/` (immutable after merge) |
| Schema validation middleware | `pkg/middleware/schema_validation.go`, `pkg/validators/schema_validator.go` |
| Configuration management | `pkg/config/` |
| Plugin registration | `plugins/` — clusters, nodePools, adapterStatus, generic, resources, channels, versions |
| OpenAPI spec import + codegen | `openapi/README.md` |
| Test factories | `test/factories/` |
| Integration tests | `test/integration/` |
| Helm chart | `charts/` |
| Tool versions (Bingo) | `.bingo/` |

## Code Conventions

### Errors

All service methods return `*errors.ServiceError` (not stdlib error). Use constructor functions from `pkg/errors/errors.go`: `NotFound()`, `Validation()`, `GeneralError()`, `Conflict()`, `ValidationWithDetails()`. Error codes: `HYPERFLEET-CAT-NUM` format. Errors convert to RFC 9457 Problem Details via `AsProblemDetails()`.

### Logging

Use `pkg/logger/` — `logger.Info(ctx, "msg")`, `logger.With(ctx, "key", val).WithError(err).Error("msg")`. Never use `fmt.Println` or `log.Print`.

### Handlers

Use `handlerConfig` pipeline from `pkg/handlers/framework.go`:
- `handle(w, r, cfg, status)` — unmarshal → validate → action → respond
- `handleGet` / `handleList` / `handleDelete` — no-body variants
- Validation: `func() *errors.ServiceError`; Action: `func() (interface{}, *errors.ServiceError)`

### Services

Interface + `sql*Service` struct. Constructor injection of DAOs. Return `*errors.ServiceError`. Add `//go:generate mockgen` directive for mocks.

### DAOs

Interface + `sql*Dao` struct. Get session via `sessionFactory.New(ctx)`. Call `db.MarkForRollback(ctx, err)` on write errors. Return stdlib `error`.

### Plugins

Register via `init()`: `registry.RegisterService()`, `server.RegisterRoutes()`, `presenters.RegisterPath()`, `presenters.RegisterKind()`. See `plugins/clusters/plugin.go` as reference. Active plugins: clusters, nodePools, adapterStatus, generic, resources, channels, versions.

### Generic Resources

Descriptor-driven layer for resource types beyond clusters and node pools. `ResourceService` supports configurable delete policies per descriptor. `ResourceDao` and `ResourceHandler` follow the same interface patterns.

### Tests

- **Unit**: Gomega assertions with `RegisterTestingT(t)`. Factories in `test/factories/` create resources via service layer.
- **Integration**: `test.RegisterIntegration(t)` returns `(helper, client)`. Testcontainers auto-creates isolated PostgreSQL.
- **Mocks**: `make generate-mocks` — uses `go generate` directives with `go.uber.org/mock/gomock`. Never write mocks manually.
- **Helm**: `make test-helm` — lints and renders templates with multiple value combinations.

## Gotchas

- Generated code not in git — `make generate-all` MUST run before build/test/lint
- Migration files are immutable after merge — create new migration files for schema changes
- `status.phase` is calculated from adapter conditions — never set it manually
- Tool versions managed by Bingo (`.bingo/`) — don't manually install oapi-codegen or golangci-lint
- OpenAPI spec (`openapi/openapi.yaml`) not tracked in git — generated by `make generate` from `hyperfleet-api-spec` Go module

## Boundaries

### DON'T

- Edit files in `pkg/api/openapi/` — generated by `make generate`
- Edit `*_mock.go` files — regenerate with `make generate-mocks`
- Set `status.phase` manually — calculated from adapter conditions
- Create direct DB connections — use `SessionFactory.New(ctx)` for transaction participation
- Build without FIPS — always use `CGO_ENABLED=1 GOEXPERIMENT=boringcrypto`
- Modify existing migration files — create new ones instead

### DO

- Run `make generate-all` before any build or test
- Run `make verify-all` before declaring work done
- Use `handlerConfig` pipeline for HTTP handlers
- Use `*errors.ServiceError` for all service-layer errors
- Use `logger.Info/Error(ctx, ...)` for structured logging
- Follow plugin registration pattern in `plugins/*/plugin.go`
