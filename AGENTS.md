# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Commands

```bash
# Run in development
go run main.go

# Build production binary
go build -o sql-api .

# Download dependencies
go mod download

# Vet and check for issues
go vet ./...

# Run tests (none exist yet, but the standard command)
go test ./...
```

## Architecture

This is a Go REST API (no framework — stdlib `net/http` only) built with Clean Architecture. The dependency graph flows strictly inward:

```
delivery/http → usecase → repository → domain
```

- **`domain/query.go`** — the only package with no internal imports. Defines all shared types (`QueryRequest`, `QueryResult`, `ExecuteMode`), the two interface contracts (`QueryRepository`, `QueryUsecase`), and sentinel errors (`ErrForbiddenStatement`, `ErrSQLParseFailed`). The handler maps these sentinels to specific HTTP status codes (403, 422).

- **`usecase/query_usecase.go`** — the security layer. Every query passes through `sqlparser.Parse` (AST-level, via `github.com/xwb1989/sqlparser`) before reaching the database. `validateAndClassify` enforces the allowlist (blocks DELETE, DROP, TRUNCATE; returns 403) and determines `ExecuteMode` (Query vs Exec).

- **`repository/sql_repository.go`** — thin DB wrapper. Routes to `QueryContext` (SELECT/UNION → returns rows) or `ExecContext` (INSERT/UPDATE/DDL → returns rows affected). Scans `[]byte` values to `string` for JSON safety.

- **`config/config.go`** — reads env vars, constructs `*sql.DB` with pool settings, calls `db.Ping()` at startup. All three drivers (MySQL, PostgreSQL, SQL Server) are blank-imported here so only `DB_DRIVER` needs to change.

- **`main.go`** — wires DI manually (no framework), registers two routes, starts the server.

## Configuration

`DB_DSN` is the only required env var. Copy `.env.example` to `.env`. The server reads `.env` automatically at startup via `godotenv`; if no `.env` exists it falls back to real environment variables.

| Env var | Default | Notes |
|---|---|---|
| `DB_DRIVER` | `mysql` | `mysql`, `postgres`, or `sqlserver` |
| `DB_DSN` | — | **Required** |
| `SERVER_PORT` | `8080` | |
| `QUERY_TIMEOUT_SECONDS` | `10` | Per-request context deadline |

## Key design decisions

- **No query parameters / prepared statements**: queries arrive as raw strings and are forwarded verbatim after AST validation. The security boundary is the allowlist, not parameterization.
- **`ExecModeQuery` vs `ExecModeExec`**: the usecase decides which `*sql.DB` method to use based on the parsed statement type; the repository never inspects the SQL string itself.
- **Structured JSON logging** (`log/slog`): all output goes to stdout as JSON. There is no log level configuration; the default is INFO.
