package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"sql-api/internal/config"
	httpdelivery "sql-api/internal/delivery/http"
	"sql-api/internal/domain"
	"sql-api/internal/repository"
	"sql-api/internal/usecase"
)

func main() {
	envFile := flag.String("env", "", "path to .env file (default: .env in current dir, then ~/.config/sql-api/.env)")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	config.LoadEnv(*envFile)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	port, err := strconv.Atoi(cfg.ServerPort)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid SERVER_PORT")
	}
	pools, err := config.OpenDatabases(ctx, cfg)
	if err != nil {
		return err
	}
	defer config.CloseDatabases(pools)
	queryUCs := make(map[string]domain.QueryUsecase, len(pools))
	for name, db := range pools {
		dbCfg := cfg.Databases[name]
		queryUCs[name] = usecase.NewQueryUsecase(repository.NewSQLRepository(db), cfg.QueryTimeoutSeconds, dbCfg.Driver)
		slog.Info("database connected", "database", name, "driver", dbCfg.Driver,
			"max_open_conns", dbCfg.MaxOpenConns, "max_idle_conns", dbCfg.MaxIdleConns,
			"conn_max_lifetime", dbCfg.ConnMaxLifetime.String())
	}
	handler := httpdelivery.NewHandler(queryUCs, cfg.DefaultDatabase)
	healthHandler := httpdelivery.NewHealthHandler(pools, cfg.NamedDatabases)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler.Health)
	mux.HandleFunc("POST /api/v1/execute", handler.Execute)
	mux.HandleFunc("POST /api/v1/{database}/execute", handler.Execute)
	ln, port, err := listenWithFallback(port)
	if err != nil {
		return err
	}
	slog.Info("server listening", "addr", fmt.Sprintf(":%d", port))
	server := &http.Server{Handler: mux}
	served := make(chan error, 1)
	go func() { served <- server.Serve(ln) }()
	select {
	case err := <-served:
		_ = server.Close()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), max(time.Duration(cfg.QueryTimeoutSeconds)*time.Second, 3*time.Second))
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("HTTP shutdown: %w", err)
		}
		return nil
	}
}

// listenWithFallback tries to bind starting at port, incrementing until it finds a free one.
func listenWithFallback(port int) (net.Listener, int, error) {
	for port <= 65535 {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			return ln, port, nil
		}
		if isPortInUse(err) {
			port++
			continue
		}
		return nil, 0, fmt.Errorf("listen: %w", err)
	}
	return nil, 0, fmt.Errorf("no available port")
}

func isPortInUse(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return errors.Is(opErr.Err, syscall.EADDRINUSE)
	}
	return false
}
