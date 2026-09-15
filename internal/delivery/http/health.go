package http

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// HealthHandler checks infrastructure directly, outside the query usecase.
type HealthHandler struct {
	databases map[string]*sql.DB
	named     bool
}

// NewHealthHandler uses named to enable per-alias statuses in the response.
func NewHealthHandler(databases map[string]*sql.DB, named bool) *HealthHandler {
	return &HealthHandler{databases: databases, named: named}
}

type healthResponse struct {
	Status    string            `json:"status"`
	Database  string            `json:"database"`
	Databases map[string]string `json:"databases,omitempty"`
}

// Health returns 200 only when all configured databases are reachable.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	statuses := make(map[string]string, len(h.databases))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for name, db := range h.databases {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status := "ok"
			if err := db.PingContext(ctx); err != nil {
				status = "unreachable"
				slog.Error("health check: database unreachable", "database", name)
			}
			mu.Lock()
			statuses[name] = status
			mu.Unlock()
		}()
	}
	wg.Wait()
	resp := healthResponse{Status: "ok", Database: "ok"}
	code := http.StatusOK
	for _, status := range statuses {
		if status != "ok" {
			resp.Status = "degraded"
			resp.Database = "unreachable"
			code = http.StatusServiceUnavailable
		}
	}
	if h.named {
		resp.Databases = statuses
	}
	writeJSON(w, code, resp)
}
