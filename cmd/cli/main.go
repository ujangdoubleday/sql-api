package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"sql-api/internal/config"
	"sql-api/internal/domain"
	"sql-api/internal/repository"
	"sql-api/internal/usecase"
)

func main() {
	var (
		queryStr string
		filePath string
		envFile  string
	)

	flag.StringVar(&queryStr, "q", "", "SQL query to execute")
	flag.StringVar(&filePath, "f", "", "Path to .sql file to execute")
	flag.StringVar(&envFile, "env", "", "path to .env file (default: .env in current dir, then ~/.config/sql-api/.env)")
	flag.Parse()

	if queryStr == "" && filePath == "" {
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  sql-cli -q \"SELECT 1\"")
		fmt.Fprintln(os.Stderr, "  sql-cli -f query.sql")
		os.Exit(1)
	}
	if queryStr != "" && filePath != "" {
		fmt.Fprintln(os.Stderr, "error: use either -q or -f, not both")
		os.Exit(1)
	}

	if filePath != "" {
		b, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading file: %v\n", err)
			os.Exit(1)
		}
		queryStr = string(b)
	}

	config.LoadEnv(envFile)

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	db, err := config.NewDB(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "database connection failed: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	repo := repository.NewSQLRepository(db)
	uc := usecase.NewQueryUsecase(repo, cfg.QueryTimeoutSeconds, cfg.DBDriver)

	result, err := uc.ProcessQuery(context.Background(), &domain.QueryRequest{Query: queryStr})
	if err != nil {
		fmt.Fprintf(os.Stderr, "query error: %v\n", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "encode error: %v\n", err)
		os.Exit(1)
	}
}
