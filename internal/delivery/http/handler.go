package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"sql-api/internal/domain"
)

// Handler holds the HTTP layer dependencies.
type Handler struct {
	queryUC domain.QueryUsecase
}

// NewHandler constructs the delivery Handler.
func NewHandler(queryUC domain.QueryUsecase) *Handler {
	return &Handler{queryUC: queryUC}
}

type errorResponse struct {
	Error string `json:"error"`
}

// Execute handles POST /api/v1/execute.
func (h *Handler) Execute(w http.ResponseWriter, r *http.Request) {
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

	slog.Info("incoming query", "query", req.Query, "context", req.Context)

	result, err := h.queryUC.ProcessQuery(r.Context(), &req)
	if err != nil {
		slog.Error("query failed", "error", err, "query", req.Query)
		statusCode := resolveStatusCode(err)
		writeJSON(w, statusCode, errorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// resolveStatusCode maps domain sentinel errors to HTTP status codes.
func resolveStatusCode(err error) int {
	switch {
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
