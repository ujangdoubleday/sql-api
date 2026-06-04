package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/xwb1989/sqlparser"

	"sql-api/internal/domain"
)

type queryUsecase struct {
	repo            domain.QueryRepository
	queryTimeoutSec int
}

// NewQueryUsecase wires the usecase with its repository dependency.
func NewQueryUsecase(repo domain.QueryRepository, queryTimeoutSec int) domain.QueryUsecase {
	return &queryUsecase{repo: repo, queryTimeoutSec: queryTimeoutSec}
}

func (uc *queryUsecase) ProcessQuery(ctx context.Context, req *domain.QueryRequest) (*domain.QueryResult, error) {
	stmt, err := sqlparser.Parse(req.Query)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", domain.ErrSQLParseFailed, err.Error())
	}

	mode, err := validateAndClassify(stmt)
	if err != nil {
		return nil, err
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(uc.queryTimeoutSec)*time.Second)
	defer cancel()

	return uc.repo.Execute(execCtx, req.Query, mode)
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
