package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"sql-api/internal/config"
	"sql-api/internal/domain"
	"sql-api/internal/repository"
	"sql-api/internal/usecase"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var (
		queryStr string
		filePath string
		envFile  string
		database string
	)

	flag.StringVar(&queryStr, "q", "", "SQL query to execute")
	flag.StringVar(&filePath, "f", "", "Path to .sql file to execute")
	flag.StringVar(&envFile, "env", "", "path to .env file (default: .env in current dir, then ~/.config/sql-api/.env)")
	flag.StringVar(&database, "database", "", "configured database alias (default: configured default)")
	flag.Parse()

	if queryStr == "" && filePath == "" {
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  sql-cli -q \"SELECT 1\"")
		fmt.Fprintln(os.Stderr, "  sql-cli -f query.sql")
		return fmt.Errorf("query is required")
	}
	if queryStr != "" && filePath != "" {
		return fmt.Errorf("use either -q or -f, not both")
	}

	if filePath != "" {
		b, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}
		queryStr = string(b)
	}

	config.LoadEnv(envFile)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	if database == "" {
		database = cfg.DefaultDatabase
	}
	dbCfg, ok := cfg.Databases[database]
	if !ok {
		return fmt.Errorf("unknown database alias %q", database)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.QueryTimeoutSeconds)*time.Second)
	defer cancel()
	db, err := config.NewDB(ctx, dbCfg)
	if err != nil {
		return fmt.Errorf("database %q: %w", database, err)
	}
	defer db.Close()
	repo := repository.NewSQLRepository(db)
	uc := usecase.NewQueryUsecase(repo, cfg.QueryTimeoutSeconds, dbCfg.Driver)

	result, err := uc.ProcessQuery(context.Background(), &domain.QueryRequest{Query: queryStr})
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	return nil
}
