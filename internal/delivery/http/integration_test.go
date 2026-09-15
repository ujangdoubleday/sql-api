package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"sql-api/internal/config"
	"sql-api/internal/domain"
	"sql-api/internal/repository"
	"sql-api/internal/usecase"
)

// Opt-in: each test database must have one distinct marker in sql_api_test_marker.
// This test reads fixture data only; an outage is simulated by closing one local pool.
func TestMultiDatabaseIntegration(t *testing.T) {
	if os.Getenv("SQL_API_INTEGRATION") != "1" {
		t.Skip("set SQL_API_INTEGRATION=1 and named database environment variables")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.NamedDatabases || len(cfg.Databases) != 2 {
		t.Fatal("integration test requires exactly two named test databases")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pools, err := config.OpenDatabases(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer config.CloseDatabases(pools)
	ucs := make(map[string]domain.QueryUsecase)
	markers := make(map[string]string)
	other := ""
	for name, db := range pools {
		ucs[name] = usecase.NewQueryUsecase(repository.NewSQLRepository(db), cfg.QueryTimeoutSeconds, cfg.Databases[name].Driver)
		var marker string
		if err := db.QueryRowContext(ctx, "SELECT marker FROM sql_api_test_marker").Scan(&marker); err != nil {
			t.Fatalf("%s fixture: %v", name, err)
		}
		if marker == "" {
			t.Fatal("fixture marker must be nonempty")
		}
		markers[name] = marker
		if name != cfg.DefaultDatabase {
			other = name
		}
	}
	if markers[other] == markers[cfg.DefaultDatabase] {
		t.Fatal("fixture markers must differ")
	}
	h := NewHandler(ucs, cfg.DefaultDatabase)
	health := NewHealthHandler(pools, true)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/execute", h.Execute)
	mux.HandleFunc("POST /api/v1/{database}/execute", h.Execute)
	mux.HandleFunc("GET /health", health.Health)
	request := func(method, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(`{"query":"SELECT marker FROM sql_api_test_marker"}`)).WithContext(ctx)
		mux.ServeHTTP(w, r)
		return w
	}
	checkMarker := func(path, want string) {
		t.Helper()
		w := request("POST", path)
		var result domain.QueryResult
		if w.Code != 200 {
			t.Fatalf("%s status %d: %s", path, w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || len(result.Rows) != 1 || result.Rows[0]["marker"] != want {
			t.Fatalf("%s: %s", path, w.Body.String())
		}
	}
	for name, marker := range markers {
		checkMarker("/api/v1/"+name+"/execute", marker)
	}
	checkMarker("/api/v1/execute", markers[cfg.DefaultDatabase])
	if w := request("GET", "/health"); w.Code != 200 {
		t.Fatalf("health: %s", w.Body.String())
	}
	pools[other].Close()
	if w := request("POST", "/api/v1/"+other+"/execute"); w.Code != 500 {
		t.Fatalf("closed database status %d", w.Code)
	}
	checkMarker("/api/v1/execute", markers[cfg.DefaultDatabase])
	if w := request("GET", "/health"); w.Code != 503 {
		t.Fatalf("degraded health: %s", w.Body.String())
	}
}
