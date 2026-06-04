package domain

import (
	"context"
	"errors"
)

// Sentinel errors — used by handler to pick the correct HTTP status code.
var (
	ErrForbiddenStatement = errors.New("unauthorized SQL statement")
	ErrSQLParseFailed     = errors.New("AST parsing failed")
)

// QueryRequest is the incoming JSON payload from the AI Agent.
type QueryRequest struct {
	Query   string `json:"query"`
	Context string `json:"context,omitempty"`
}

// QueryResult is the unified response for any SQL execution.
type QueryResult struct {
	Columns      []string         `json:"columns"`
	Rows         []map[string]any `json:"rows"`
	RowsAffected int64            `json:"rows_affected"`
	LastInsertID int64            `json:"last_insert_id,omitempty"`
}

// ExecuteMode tells the repository which *sql.DB method to use.
type ExecuteMode uint8

const (
	ExecModeQuery ExecuteMode = iota // QueryContext — SELECT / UNION
	ExecModeExec                     // ExecContext  — INSERT / UPDATE / DDL
)

// QueryRepository is the data-access contract.
type QueryRepository interface {
	Execute(ctx context.Context, query string, mode ExecuteMode) (*QueryResult, error)
}

// QueryUsecase is the business-logic contract.
type QueryUsecase interface {
	ProcessQuery(ctx context.Context, req *QueryRequest) (*QueryResult, error)
}
