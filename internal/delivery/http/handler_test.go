package http

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"sql-api/internal/domain"
)

type stubUsecase struct {
	name string
	err  error
}

func (uc stubUsecase) ProcessQuery(context.Context, *domain.QueryRequest) (*domain.QueryResult, error) {
	return &domain.QueryResult{Columns: []string{"database"}, Rows: []map[string]any{{"database": uc.name}}, RowsAffected: 1}, uc.err
}
func TestRoutes(t *testing.T) {
	h := NewHandler(map[string]domain.QueryUsecase{"main": stubUsecase{name: "main"}, "reporting": stubUsecase{name: "reporting"}}, "main")
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/execute", h.Execute)
	mux.HandleFunc("POST /api/v1/{database}/execute", h.Execute)
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, tt := range []struct {
				path, want string
				code       int
			}{
				{"/api/v1/execute", "main", 200}, {"/api/v1/main/execute", "main", 200}, {"/api/v1/reporting/execute", "reporting", 200}, {"/api/v1/missing/execute", "", 404},
			} {
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, httptest.NewRequest("POST", tt.path, strings.NewReader(`{"query":"SELECT 1"}`)))
				if w.Code != tt.code {
					t.Errorf("%s status %d", tt.path, w.Code)
					continue
				}
				if tt.code == 200 {
					var result domain.QueryResult
					if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result.Rows) != 1 || result.Rows[0]["database"] != tt.want {
						t.Errorf("%s: %s", tt.path, w.Body.String())
					}
				}
			}
		}()
	}
	wg.Wait()
}
func TestExecuteErrors(t *testing.T) {
	for _, tt := range []struct {
		body string
		err  error
		code int
	}{
		{"{", nil, 400}, {`{}`, nil, 400}, {`{"query":"SELECT 1"}`, domain.ErrForbiddenStatement, 403},
		{`{"query":"SELECT 1"}`, domain.ErrSQLParseFailed, 422}, {`{"query":"SELECT 1"}`, fmt.Errorf("wrapped: %w", context.DeadlineExceeded), 504},
		{`{"query":"SELECT 1"}`, errors.New("database operation failed"), 500},
	} {
		h := NewHandler(map[string]domain.QueryUsecase{"main": stubUsecase{err: tt.err}}, "main")
		w := httptest.NewRecorder()
		h.Execute(w, httptest.NewRequest("POST", "/api/v1/execute", strings.NewReader(tt.body)))
		if w.Code != tt.code {
			t.Errorf("status %d, want %d", w.Code, tt.code)
		}
	}
}

type healthDriver struct{}
type healthConn struct{ mode string }

func (healthDriver) Open(dsn string) (driver.Conn, error) { return healthConn{dsn}, nil }
func (healthConn) Prepare(string) (driver.Stmt, error)    { return nil, errors.New("unused") }
func (healthConn) Begin() (driver.Tx, error)              { return nil, errors.New("unused") }
func (healthConn) Close() error                           { return nil }
func (c healthConn) Ping(ctx context.Context) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("missing deadline")
	}
	if c.mode == "fail" {
		return errors.New("secret")
	}
	if c.mode == "wait" {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}
func init() { sql.Register("health_test", healthDriver{}) }
func TestHealth(t *testing.T) {
	for _, tt := range []struct {
		mode  string
		named bool
		code  int
	}{{"ok", true, 200}, {"fail", true, 503}, {"wait", true, 503}, {"ok", false, 200}} {
		t.Run(fmt.Sprintf("%s-%v", tt.mode, tt.named), func(t *testing.T) {
			a, _ := sql.Open("health_test", "ok")
			defer a.Close()
			b, _ := sql.Open("health_test", tt.mode)
			defer b.Close()
			h := NewHealthHandler(map[string]*sql.DB{"a": a, "b": b}, tt.named)
			req := httptest.NewRequest("GET", "/health", nil)
			if tt.mode == "wait" {
				ctx, cancel := context.WithTimeout(req.Context(), 20*time.Millisecond)
				defer cancel()
				req = req.WithContext(ctx)
			}
			w := httptest.NewRecorder()
			h.Health(w, req)
			if w.Code != tt.code || strings.Contains(w.Body.String(), "secret") {
				t.Fatalf("health: %d %s", w.Code, w.Body.String())
			}
			var resp healthResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if tt.named && len(resp.Databases) != 2 {
				t.Fatal("missing per-database statuses")
			}
			if !tt.named && resp.Databases != nil {
				t.Fatal("legacy health shape changed")
			}
		})
	}
}
