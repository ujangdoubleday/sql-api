package repository

import (
	"context"
	"database/sql"
	"fmt"

	"sql-api/internal/domain"
)

type sqlRepository struct {
	db *sql.DB
}

// NewSQLRepository returns a QueryRepository backed by the given *sql.DB.
func NewSQLRepository(db *sql.DB) domain.QueryRepository {
	return &sqlRepository{db: db}
}

func (r *sqlRepository) Execute(ctx context.Context, query string, mode domain.ExecuteMode) (*domain.QueryResult, error) {
	if mode == domain.ExecModeQuery {
		return r.runQuery(ctx, query)
	}
	return r.runExec(ctx, query)
}

// runQuery uses QueryContext and scans the result set into a slice of maps.
func (r *sqlRepository) runQuery(ctx context.Context, query string) (*domain.QueryResult, error) {
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("SQL Execution Error: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve column names: %w", err)
	}

	resultRows := make([]map[string]any, 0)
	for rows.Next() {
		vals := make([]any, len(columns))
		ptrs := make([]any, len(columns))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("row scan error: %w", err)
		}

		row := make(map[string]any, len(columns))
		for i, col := range columns {
			// []byte is the default scan type for strings in many drivers; convert for JSON safety.
			if b, ok := vals[i].([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = vals[i]
			}
		}
		resultRows = append(resultRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return &domain.QueryResult{
		Columns:      columns,
		Rows:         resultRows,
		RowsAffected: int64(len(resultRows)),
	}, nil
}

// runExec uses ExecContext for statements that do not return rows.
func (r *sqlRepository) runExec(ctx context.Context, query string) (*domain.QueryResult, error) {
	res, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("SQL Execution Error: %w", err)
	}

	rowsAffected, _ := res.RowsAffected()
	lastInsertID, _ := res.LastInsertId()

	return &domain.QueryResult{
		Columns:      []string{},
		Rows:         []map[string]any{},
		RowsAffected: rowsAffected,
		LastInsertID: lastInsertID,
	}, nil
}
