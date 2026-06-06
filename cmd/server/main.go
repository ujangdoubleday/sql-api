package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"syscall"

	"sql-api/internal/config"
	httpdelivery "sql-api/internal/delivery/http"
	"sql-api/internal/repository"
	"sql-api/internal/usecase"
)

func main() {
	envFile := flag.String("env", "", "path to .env file (default: .env in current dir, then ~/.config/sql-api/.env)")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	config.LoadEnv(*envFile)

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

	queryRepo := repository.NewSQLRepository(db)
	queryUC := usecase.NewQueryUsecase(queryRepo, cfg.QueryTimeoutSeconds, cfg.DBDriver)
	handler := httpdelivery.NewHandler(queryUC)
	healthHandler := httpdelivery.NewHealthHandler(db)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler.Health)
	mux.HandleFunc("POST /api/v1/execute", handler.Execute)

	port, err := strconv.Atoi(cfg.ServerPort)
	if err != nil {
		slog.Error("invalid SERVER_PORT", "error", err)
		os.Exit(1)
	}

	ln, port := listenWithFallback(port)
	slog.Info("server listening", "addr", fmt.Sprintf(":%d", port))

	if err := http.Serve(ln, mux); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// listenWithFallback tries to bind starting at port, incrementing until it finds a free one.
func listenWithFallback(port int) (net.Listener, int) {
	for {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			return ln, port
		}
		if isPortInUse(err) {
			port++
			continue
		}
		slog.Error("failed to bind port", "error", err)
		os.Exit(1)
	}
}

func isPortInUse(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return errors.Is(opErr.Err, syscall.EADDRINUSE)
	}
	return false
}
