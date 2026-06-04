# sql-api

A high-performance REST API that acts as a secure bridge for AI Agents to execute SQL queries against a database. Built with Go's standard library, Clean Architecture, and strict AST-level SQL filtering.

---

## Prerequisites

| Tool | Version |
|---|---|
| [Go](https://go.dev/dl/) | 1.22 or later |
| A running database | MySQL, PostgreSQL, or SQL Server |

Verify your Go version:

```bash
go version
```

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

Copy the example environment file and fill in your values:

```bash
cp .env.example .env
```

Edit `.env`:

```dotenv
# Driver: mysql | postgres | sqlserver
DB_DRIVER=mysql
DB_DSN=root:secret@tcp(127.0.0.1:3306)/mydb?parseTime=true

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

## Build

**Development (run directly)**

```bash
go run main.go
```

**Production binary**

```bash
go build -o sql-api .
```

Run the binary:

```bash
./sql-api
```

**Cross-compile (optional)**

```bash
# Linux amd64
GOOS=linux GOARCH=amd64 go build -o sql-api-linux .

# Windows
GOOS=windows GOARCH=amd64 go build -o sql-api.exe .
```

---

## Running

```bash
# With a .env file in the current directory
./sql-api

# Or inject env vars directly (no .env needed)
DB_DRIVER=postgres DB_DSN="postgres://..." SERVER_PORT=9090 ./sql-api
```

Expected startup log:

```json
{"time":"...","level":"INFO","msg":"database connected","driver":"mysql","max_open_conns":25,...}
{"time":"...","level":"INFO","msg":"server listening","addr":":8080"}
```

---

## API Reference

### `GET /health`

Liveness + readiness check. Pings the database and reports its status.

**200 OK — all systems healthy**

```json
{
  "status": "ok",
  "database": "ok"
}
```

**503 Service Unavailable — database unreachable**

```json
{
  "status": "degraded",
  "database": "unreachable: dial tcp 127.0.0.1:3306: connect: connection refused"
}
```

---

### `POST /api/v1/execute`

Execute a SQL statement. The query is parsed at the AST level before reaching the database.

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

**403 Forbidden** — blocked statement (DELETE, DROP, TRUNCATE)

```json
{ "error": "unauthorized SQL statement: DELETE statements are not permitted" }
```

**422 Unprocessable Entity** — SQL syntax cannot be parsed

```json
{ "error": "AST parsing failed: syntax error at position 7 near 'SELEKT'" }
```

**500 Internal Server Error** — database execution error

```json
{ "error": "SQL Execution Error: Table 'mydb.unknown' doesn't exist" }
```

---

### SQL allowlist

| Statement | Allowed |
|---|---|
| `SELECT` / `UNION` | yes |
| `INSERT` | yes |
| `UPDATE` | yes |
| `CREATE` / `ALTER` / `RENAME` | yes |
| `DELETE` | **no** — 403 |
| `DROP` | **no** — 403 |
| `TRUNCATE` | **no** — 403 |

---

## Quick test with curl

```bash
# Health check
curl http://localhost:8080/health

# SELECT
curl -s -X POST http://localhost:8080/api/v1/execute \
  -H "Content-Type: application/json" \
  -d '{"query":"SELECT 1 AS ping"}'

# INSERT
curl -s -X POST http://localhost:8080/api/v1/execute \
  -H "Content-Type: application/json" \
  -d '{"query":"INSERT INTO logs (msg) VALUES (\"hello\")"}'

# Blocked — DELETE
curl -s -X POST http://localhost:8080/api/v1/execute \
  -H "Content-Type: application/json" \
  -d '{"query":"DELETE FROM users"}'
```

---

## Project structure

```
sql-api/
├── .env.example              # Configuration template
├── main.go                   # Entry point — wires DI, registers routes
└── internal/
    ├── config/
    │   └── config.go         # Env loading, *sql.DB factory, connection pool
    ├── domain/
    │   └── query.go          # Shared types, interfaces, sentinel errors
    ├── repository/
    │   └── sql_repository.go # DB execution (QueryContext / ExecContext)
    ├── usecase/
    │   └── query_usecase.go  # AST firewall, timeout, orchestration
    └── delivery/http/
        ├── handler.go        # POST /api/v1/execute
        └── health.go         # GET /health
```
