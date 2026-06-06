package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/xwb1989/sqlparser"

	"sql-api/internal/domain"
)

type queryUsecase struct {
	repo            domain.QueryRepository
	queryTimeoutSec int
	driver          string
}

// NewQueryUsecase wires the usecase with its repository dependency.
func NewQueryUsecase(repo domain.QueryRepository, queryTimeoutSec int, driver string) domain.QueryUsecase {
	return &queryUsecase{repo: repo, queryTimeoutSec: queryTimeoutSec, driver: driver}
}

func (uc *queryUsecase) ProcessQuery(ctx context.Context, req *domain.QueryRequest) (*domain.QueryResult, error) {
	var (
		mode domain.ExecuteMode
		err  error
	)

	if uc.driver == "sqlserver" {
		mode = classifyForSQLServer(req.Query)
	} else {
		var stmt sqlparser.Statement
		stmt, err = sqlparser.Parse(req.Query)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", domain.ErrSQLParseFailed, err.Error())
		}
		mode, err = validateAndClassify(stmt)
		if err != nil {
			return nil, err
		}
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(uc.queryTimeoutSec)*time.Second)
	defer cancel()

	return uc.repo.Execute(execCtx, req.Query, mode)
}

func classifyForSQLServer(query string) domain.ExecuteMode {
	switch firstKeyword(query) {
	case "SELECT", "WITH", "EXEC", "EXECUTE":
		return domain.ExecModeQuery
	default:
		return domain.ExecModeExec
	}
}

// firstKeyword returns the uppercased first whitespace-delimited token of a query.
func firstKeyword(query string) string {
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return ""
	}
	return strings.ToUpper(fields[0])
}

// validateAndClassify enforces the statement allowlist and returns the execute mode.
// Blocked:  DELETE, DROP TABLE, TRUNCATE TABLE
// Allowed:  SELECT, INSERT, UPDATE, CREATE, ALTER, RENAME, UNION, and others (SHOW/SET/USE).
func validateAndClassify(stmt sqlparser.Statement) (domain.ExecuteMode, error) {
	switch s := stmt.(type) {

	case *sqlparser.Select:
		return domain.ExecModeQuery, nil

	case *sqlparser.Union:
		return domain.ExecModeQuery, nil

	case *sqlparser.Insert:
		return domain.ExecModeExec, nil

	case *sqlparser.Update:
		return domain.ExecModeExec, nil

	case *sqlparser.Delete:
		return 0, fmt.Errorf("%w: DELETE statements are not permitted",
			domain.ErrForbiddenStatement)

	case *sqlparser.DDL:
		switch s.Action {
		case sqlparser.DropStr:
			return 0, fmt.Errorf("%w: DROP statements are not permitted",
				domain.ErrForbiddenStatement)
		case sqlparser.TruncateStr:
			return 0, fmt.Errorf("%w: TRUNCATE statements are not permitted",
				domain.ErrForbiddenStatement)
		default:
			// CREATE, ALTER, RENAME — allowed
			return domain.ExecModeExec, nil
		}

	default:
		// SHOW, SET, USE, EXPLAIN, etc. — forward as exec; DB driver will validate.
		return domain.ExecModeExec, nil
	}
}
