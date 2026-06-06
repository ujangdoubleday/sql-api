# sql-api

A REST API that acts as a secure bridge for AI Agents to execute SQL queries against a database. Built with Go's standard library and Clean Architecture. Supports MySQL, PostgreSQL, and SQL Server (full T-SQL support).

---

## Prerequisites

| Tool | Version |
|---|---|
| [Go](https://go.dev/dl/) | 1.22 or later |
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

The CLI connects directly to the database using the same config resolution as the server. Output is JSON to stdout; errors go to stderr.

```bash
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

Liveness + readiness check. Pings the database and reports its status.

**200 OK**

```json
{ "status": "ok", "database": "ok" }
```

**503 Service Unavailable**

```json
{ "status": "degraded", "database": "unreachable: ..." }
```

---

### `POST /api/v1/execute`

Execute a SQL statement.

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
