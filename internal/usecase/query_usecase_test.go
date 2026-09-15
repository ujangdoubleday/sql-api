package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"sql-api/internal/domain"
)

type recordingRepo struct {
	calls    int
	mode     domain.ExecuteMode
	deadline bool
}

func (r *recordingRepo) Execute(ctx context.Context, query string, mode domain.ExecuteMode) (*domain.QueryResult, error) {
	r.calls++
	r.mode = mode
	_, r.deadline = ctx.Deadline()
	return &domain.QueryResult{}, nil
}
func TestDriverClassification(t *testing.T) {
	for _, tt := range []struct {
		driver, query string
		mode          domain.ExecuteMode
		blocked       bool
	}{
		{"mysql", "SELECT 1", domain.ExecModeQuery, false},
		{"postgres", "SELECT 1", domain.ExecModeQuery, false},
		{"mysql", "DELETE FROM items", domain.ExecModeExec, true},
		{"postgres", "DELETE FROM items", domain.ExecModeExec, true},
		{"sqlserver", "EXEC sample", domain.ExecModeQuery, false},
		{"sqlserver", "DELETE FROM items", domain.ExecModeExec, false},
	} {
		repo := &recordingRepo{}
		uc := NewQueryUsecase(repo, 1, tt.driver)
		_, err := uc.ProcessQuery(context.Background(), &domain.QueryRequest{Query: tt.query})
		if tt.blocked {
			if !errors.Is(err, domain.ErrForbiddenStatement) || repo.calls != 0 {
				t.Fatalf("%s: validation bypassed", tt.driver)
			}
		} else if err != nil || repo.calls != 1 || repo.mode != tt.mode || !repo.deadline {
			t.Fatalf("%s %s: err=%v repo=%+v", tt.driver, tt.query, err, repo)
		}
	}
}

type timeoutRepo struct{}

func (timeoutRepo) Execute(ctx context.Context, query string, mode domain.ExecuteMode) (*domain.QueryResult, error) {
	return nil, errors.New("driver-specific cancellation")
}
func TestDeadlineError(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	uc := NewQueryUsecase(timeoutRepo{}, 1, "sqlserver")
	_, err := uc.ProcessQuery(ctx, &domain.QueryRequest{Query: "SELECT 1"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline lost: %v", err)
	}
}
