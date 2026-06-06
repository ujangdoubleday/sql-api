package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/joho/godotenv"

	"sql-api/internal/config"
	httpdelivery "sql-api/internal/delivery/http"
	"sql-api/internal/repository"
	"sql-api/internal/usecase"
)

func main() {
	// Wire structured JSON logging globally.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if err := godotenv.Load(); err != nil {
		slog.Warn("no .env file found, falling back to environment variables")
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration error", "error", err)
		os.Exit(1)
	}

	db, err := config.NewDB(cfg)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	slog.Info("database connected",
		"driver", cfg.DBDriver,
		"max_open_conns", cfg.DBMaxOpenConns,
		"max_idle_conns", cfg.DBMaxIdleConns,
		"conn_max_lifetime", cfg.DBConnMaxLifetime.String(),
	)

	// Dependency injection — outermost layer wires everything together.
	queryRepo := repository.NewSQLRepository(db)
	queryUC := usecase.NewQueryUsecase(queryRepo, cfg.QueryTimeoutSeconds, cfg.DBDriver)
	handler := httpdelivery.NewHandler(queryUC)
	healthHandler := httpdelivery.NewHealthHandler(db)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler.Health)
	mux.HandleFunc("POST /api/v1/execute", handler.Execute)

	addr := fmt.Sprintf(":%s", cfg.ServerPort)
	slog.Info("server listening", "addr", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
