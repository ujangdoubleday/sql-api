# sql-api

A REST API that acts as a secure bridge for AI Agents to execute SQL queries against configured databases. Built with Go's standard library and Clean Architecture. Supports MySQL, PostgreSQL, and SQL Server (full T-SQL support).

---

## Prerequisites

| Tool | Version |
|---|---|
| [Go](https://go.dev/dl/) | 1.24 or later |
| A running database | MySQL, PostgreSQL, or SQL Server |

---

## Installation

**1. Clone the repository**

```bash
git clone <repository-url>
cd sql-api
```

**2. Install dependencies**

```bash
go mod download
```

---

## Configuration

### Option 1 — Per-project `.env`

Copy the example file and fill in your values:

```bash
cp .env.example .env
```

The server and CLI will automatically pick up `.env` from the current working directory.

### Option 2 — Global config (recommended for global install)

```bash
mkdir -p ~/.config/sql-api
cp .env.example ~/.config/sql-api/.env
# edit ~/.config/sql-api/.env
```

When running from any directory, the tools resolve config in this order:

| Priority | Source |
|---|---|
| 1 | `-env /path/to/.env` flag |
| 2 | `.env` in current working directory |
| 3 | `~/.config/sql-api/.env` |

### `.env` reference

```dotenv
# Driver: mysql | postgres | sqlserver
DB_DRIVER=sqlserver
DB_DSN=sqlserver://user:pass@host:1433?database=dbname

# Connection pool
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5
DB_CONN_MAX_LIFETIME_MINUTES=5

# Server
SERVER_PORT=8080

# Per-request query timeout
QUERY_TIMEOUT_SECONDS=10
```

### DSN formats by driver

| Driver | Example DSN |
|---|---|
| `mysql` | `user:pass@tcp(host:3306)/dbname?parseTime=true` |
| `postgres` | `postgres://user:pass@host:5432/dbname?sslmode=disable` |
| `sqlserver` | `sqlserver://user:pass@host:1433?database=dbname` |

### Multiple databases

Use named environment variables in the same `.env` file or process environment:

```dotenv
DATABASES=main,reporting
DEFAULT_DATABASE=main

DB_MAIN_DRIVER=mysql
DB_MAIN_DSN="app:example@tcp(mysql.internal:3306)/app?parseTime=true"
DB_REPORTING_DRIVER=postgres
DB_REPORTING_DSN="postgres://reporter:example@postgres.internal:5432/reporting?sslmode=require"

DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5
DB_CONN_MAX_LIFETIME_MINUTES=5
DB_REPORTING_MAX_OPEN_CONNS=10
DB_REPORTING_MAX_IDLE_CONNS=2
DB_REPORTING_CONN_MAX_LIFETIME_MINUTES=10

SERVER_PORT=8080
QUERY_TIMEOUT_SECONDS=10
```

- `DATABASES` is a comma-separated list of unique, case-sensitive aliases matching `[a-z][a-z0-9_]*`. Spaces around list entries are ignored. Each alias uses its uppercase form in the `DB_<ALIAS>_` prefix.
- Every named database requires `DRIVER` and `DSN`. `DEFAULT_DATABASE` must name one of them. DSNs contain the host, port, credentials, database name, and driver-specific options.
- Shared pool settings apply independently to each database; `DB_<ALIAS>_MAX_OPEN_CONNS`, `MAX_IDLE_CONNS`, and `CONN_MAX_LIFETIME_MINUTES` override them. Budget connections across all application instances: this example permits up to 35 open connections per instance.
- Maximum open connections and query timeout must be positive. Idle connections and lifetime must be nonnegative; idle cannot exceed open. A zero lifetime disables lifetime-based recycling. Invalid or empty numeric settings fail configuration loading.
- When `DATABASES` is present, legacy `DB_DRIVER`/`DB_DSN` are ignored; an empty list is an error. Without it, legacy settings work unchanged under alias `default`.
- Existing `.env` resolution still applies. Process environment values take precedence over values loaded from a file. Restart after configuration changes.

The server builds a separate pool, repository, and driver-specific usecase for each alias. The HTTP handler selects a usecase from a read-only map; no shared current-database state is changed. All pools must connect at startup, with each connectivity check bounded by `QUERY_TIMEOUT_SECONDS`. A failure closes already-opened pools and stops startup. SIGINT/SIGTERM drains HTTP requests before pool cleanup, bounded by the larger of the query timeout and three seconds.

During operation, a failed database affects its own queries; requests never fall back to another database or automatically replay SQL. Health reports failures across all pools. Connection setup and health errors omit driver details that could reveal credentials.

Aliases select connections, not authorization boundaries. Database credentials still govern access, including cross-database SQL where the database permits it. Existing driver-specific SQL validation is unchanged.

**Migration:** existing users need no changes. To enable named mode, add the named settings and move the existing DSN into the default alias's DSN setting. `/api/v1/execute` continues to use that default. Local `.env.*` files are ignored by Git, except `.env.example`.

---

## Build & Install

`make build` compiles both binaries, installs them to `$GOPATH/bin`, and adds `$GOPATH/bin` to PATH in `~/.bashrc` automatically — so `sql-api` and `sql-cli` become available globally.

```bash
make build
```

After the first build, open a new terminal (or run `source ~/.bashrc`) and the commands are available from anywhere:

```bash
sql-api
sql-cli -q "SELECT 1"
```

**Other Makefile targets**

| Command | Description |
|---|---|
| `make build` | Build + install globally |
| `make build-server` | Build server binary only to `bin/` |
| `make build-cli` | Build CLI binary only to `bin/` |
| `make run` | Build then start the server |
| `make dev` | Run server from source (no build) |
| `make clean` | Remove `bin/` directory |
| `make vet` | Run `go vet ./...` |

**Cross-compile (optional)**

```bash
# Linux amd64
GOOS=linux GOARCH=amd64 go build -o bin/sql-api-linux ./cmd/server

# Windows
GOOS=windows GOARCH=amd64 go build -o bin/sql-api.exe ./cmd/server
```

---

## Running the Server

```bash
# Global (after make build)
sql-api

# With explicit .env path
sql-api -env /path/to/.env

# Inject env vars directly (no .env needed)
DB_DRIVER=sqlserver DB_DSN="sqlserver://..." sql-api
```

Expected startup log:

```json
{"level":"INFO","msg":"database connected","driver":"sqlserver","max_open_conns":25,...}
{"level":"INFO","msg":"server listening","addr":":8080"}
```

If `SERVER_PORT` is already in use, the server automatically tries the next port (`8081`, `8082`, ...) until it finds a free one.

---

## CLI

The CLI connects directly to the selected database using the same config resolution as the server. It opens only the selected pool, so another configured database being offline does not block the command. Output is JSON to stdout; errors go to stderr.

```bash
# Named database
sql-cli -database reporting -q "SELECT 1"

# Inline query
sql-cli -q "SELECT TRY_CAST('123' AS INT) AS val"

# From a .sql file
sql-cli -f my_query.sql

# Explicit .env path
sql-cli -env /path/to/.env -q "SELECT TOP 5 * FROM orders"

# Pipe to jq
sql-cli -q "SELECT TOP 5 * FROM orders" | jq '.rows'
```

| Flag | Description |
|---|---|
| `-q "..."` | SQL query string |
| `-f file.sql` | Path to a `.sql` file |
| `-env file` | Path to `.env` file |
| `-database alias` | Select a configured database; defaults to `DEFAULT_DATABASE` (or `default` in legacy mode) |

Example output:

```json
{
  "columns": ["val"],
  "rows": [
    { "val": 123 }
  ],
  "rows_affected": 1
}
```

---

## API Reference

### `GET /health`

Readiness check. Pings all configured databases concurrently within a shared three-second deadline. Returns 503 when any database is unavailable.

**200 OK**

```json
{ "status": "ok", "database": "ok" }
```

**503 Service Unavailable**

```json
{ "status": "degraded", "database": "unreachable" }
```

Named mode additionally includes per-alias statuses:

```json
{
  "status": "degraded",
  "database": "unreachable",
  "databases": { "main": "ok", "reporting": "unreachable" }
}
```

### `POST /api/v1/{database}/execute`

Execute against an explicit configured alias. The JSON request and response match the default endpoint below. Unknown aliases return `404` with `{"error":"unknown database alias"}`; no SQL is executed.

```bash
curl -s -X POST http://localhost:8080/api/v1/reporting/execute \
  -H 'Content-Type: application/json' \
  -d '{"query":"SELECT 1 AS ping","context":"reporting check"}'
```

### `POST /api/v1/execute`

Execute a SQL statement against the configured default database.

**Request**

```http
POST /api/v1/execute
Content-Type: application/json

{
  "query": "SELECT id, name FROM users WHERE active = 1",
  "context": "optional free-text tag for logging"
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `query` | string | yes | SQL statement to execute |
| `context` | string | no | Arbitrary label (logged, not sent to DB) |

**200 OK — SELECT result**

```json
{
  "columns": ["id", "name"],
  "rows": [
    {"id": 1, "name": "Alice"},
    {"id": 2, "name": "Bob"}
  ],
  "rows_affected": 2
}
```

**200 OK — INSERT / UPDATE / DDL result**

```json
{
  "columns": [],
  "rows": [],
  "rows_affected": 1,
  "last_insert_id": 42
}
```

**400 Bad Request** — missing or malformed JSON body

```json
{ "error": "field 'query' is required" }
```

**403 Forbidden** — blocked statement (MySQL/PostgreSQL only)

```json
{ "error": "unauthorized SQL statement: DELETE statements are not permitted" }
```

**422 Unprocessable Entity** — SQL syntax cannot be parsed (MySQL/PostgreSQL only)

```json
{ "error": "AST parsing failed: syntax error at position 7 near 'SELEKT'" }
```

**500 Internal Server Error** — database execution error

```json
{ "error": "SQL Execution Error: Invalid object name 'unknown_table'" }
```

**504 Gateway Timeout** — the query context deadline expired.

---

### SQL validation by driver

For **MySQL** and **PostgreSQL**, queries are validated at the AST level before reaching the database:

| Statement | Allowed |
|---|---|
| `SELECT` / `UNION` | yes |
| `INSERT` | yes |
| `UPDATE` | yes |
| `CREATE` / `ALTER` / `RENAME` | yes |
| `DELETE` | **no** — 403 |
| `DROP` | **no** — 403 |
| `TRUNCATE` | **no** — 403 |

For **SQL Server**, all T-SQL is forwarded directly to the database — including `TRY_CAST`, `TRY_CONVERT`, `EXEC`, `MERGE`, `FOR JSON`, `FOR XML`, `PIVOT`, stored procedures, CTEs, and any other T-SQL syntax. Validation is handled by SQL Server itself.

---

## Quick test with curl

```bash
# Health check
curl http://localhost:8080/health

# SELECT
curl -s -X POST http://localhost:8080/api/v1/execute \
  -H "Content-Type: application/json" \
  -d '{"query":"SELECT 1 AS ping"}'

# SQL Server — TRY_CAST
curl -s -X POST http://localhost:8080/api/v1/execute \
  -H "Content-Type: application/json" \
  -d '{"query":"SELECT TRY_CAST(123 AS VARCHAR(10)) AS val"}'

# SQL Server — stored procedure
curl -s -X POST http://localhost:8080/api/v1/execute \
  -H "Content-Type: application/json" \
  -d '{"query":"EXEC sp_helptext \"my_view\""}'
```

---

## Project structure

```
sql-api/
├── .env.example              # Configuration template
├── Makefile                  # Build, install, run, clean targets
├── cmd/
│   ├── server/
│   │   └── main.go           # HTTP server entry point
│   └── cli/
│       └── main.go           # CLI entry point
└── internal/
    ├── config/
    │   ├── config.go         # Env loading, *sql.DB factory, connection pool
    │   └── env.go            # .env resolution (explicit → cwd → ~/.config/sql-api)
    ├── domain/
    │   └── query.go          # Shared types, interfaces, sentinel errors
    ├── repository/
    │   └── sql_repository.go # DB execution (QueryContext / ExecContext)
    ├── usecase/
    │   └── query_usecase.go  # SQL validation + T-SQL passthrough, timeout
    └── delivery/http/
        ├── handler.go        # POST /api/v1/execute
        └── health.go         # GET /health
```


## Tests

```bash
go test -race ./...
go vet ./...
go build ./cmd/server ./cmd/cli
```

Unit tests cover config validation and legacy compatibility, concurrent routing, driver-specific SQL classification, deadline mapping, pool startup cleanup, and aggregate health.

The opt-in integration test requires exactly two named **test databases**, configured through process environment variables (it does not load `.env`). In each, provision this table with exactly one row and use different marker values:

```sql
CREATE TABLE sql_api_test_marker (marker VARCHAR(100) NOT NULL);
INSERT INTO sql_api_test_marker (marker) VALUES ('main');
-- Use 'reporting' instead in the second database.
```

```bash
SQL_API_INTEGRATION=1 go test -race ./internal/delivery/http -run TestMultiDatabaseIntegration -v
```

The test only reads fixture data. It verifies both aliases and the default route against the markers, then closes one local pool to simulate unavailability and verifies that the other still works and health becomes degraded. It does not stop or modify database servers.
