package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"sql-api/internal/domain"
)

// Handler holds the HTTP layer dependencies.
type Handler struct {
	queryUCs        map[string]domain.QueryUsecase
	defaultDatabase string
}

// NewHandler constructs the delivery Handler.
func NewHandler(queryUCs map[string]domain.QueryUsecase, defaultDatabase string) *Handler {
	return &Handler{queryUCs: queryUCs, defaultDatabase: defaultDatabase}
}

type errorResponse struct {
	Error string `json:"error"`
}

// Execute handles the default and explicitly named database routes.
func (h *Handler) Execute(w http.ResponseWriter, r *http.Request) {
	database := r.PathValue("database")
	if database == "" {
		database = h.defaultDatabase
	}
	uc, ok := h.queryUCs[database]
	if !ok {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "unknown database alias"})
		return
	}
	var req domain.QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON payload: " + err.Error()})
		return
	}
	defer r.Body.Close()

	if req.Query == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "field 'query' is required"})
		return
	}

	slog.Info("incoming query", "database", database, "query", req.Query, "context", req.Context)

	result, err := uc.ProcessQuery(r.Context(), &req)
	if err != nil {
		slog.Error("query failed", "database", database, "error", err, "query", req.Query)
		statusCode := resolveStatusCode(err)
		writeJSON(w, statusCode, errorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// resolveStatusCode maps domain sentinel errors to HTTP status codes.
func resolveStatusCode(err error) int {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	case errors.Is(err, domain.ErrForbiddenStatement):
		return http.StatusForbidden
	case errors.Is(err, domain.ErrSQLParseFailed):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("response encode error", "error", err)
	}
}
