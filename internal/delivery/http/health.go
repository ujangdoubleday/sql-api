package http

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"
)

// HealthHandler handles infrastructure-level health checks.
// It intentionally holds *sql.DB directly — health is an infrastructure concern,
// not a business one, so bypassing the usecase layer here is appropriate.
type HealthHandler struct {
	db *sql.DB
}

// NewHealthHandler constructs the HealthHandler.
func NewHealthHandler(db *sql.DB) *HealthHandler {
	return &HealthHandler{db: db}
}

type healthResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

// Health handles GET /health.
// Returns 200 when the server and database are reachable, 503 otherwise.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	resp := healthResponse{
		Status:   "ok",
		Database: "ok",
	}
	httpStatus := http.StatusOK

	if err := h.db.PingContext(ctx); err != nil {
		slog.Error("health check: database unreachable", "error", err)
		resp.Status = "degraded"
		resp.Database = "unreachable: " + err.Error()
		httpStatus = http.StatusServiceUnavailable
	}

	writeJSON(w, httpStatus, resp)
}
