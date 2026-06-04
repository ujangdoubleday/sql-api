package config

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	// Blank-import all supported drivers so the caller only needs to set DB_DRIVER.
	_ "github.com/denisenkom/go-mssqldb"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

// Config holds every tunable the application reads from the environment.
type Config struct {
	DBDriver            string
	DBDSN               string
	DBMaxOpenConns      int
	DBMaxIdleConns      int
	DBConnMaxLifetime   time.Duration
	ServerPort          string
	QueryTimeoutSeconds int
}

// Load reads the environment and returns a validated Config.
func Load() (*Config, error) {
	cfg := &Config{
		DBDriver:            getEnv("DB_DRIVER", "mysql"),
		DBDSN:               getEnv("DB_DSN", ""),
		ServerPort:          getEnv("SERVER_PORT", "8080"),
		DBMaxOpenConns:      getEnvInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns:      getEnvInt("DB_MAX_IDLE_CONNS", 5),
		DBConnMaxLifetime:   time.Duration(getEnvInt("DB_CONN_MAX_LIFETIME_MINUTES", 5)) * time.Minute,
		QueryTimeoutSeconds: getEnvInt("QUERY_TIMEOUT_SECONDS", 10),
	}
	if cfg.DBDSN == "" {
		return nil, fmt.Errorf("DB_DSN environment variable is required")
	}
	return cfg, nil
}

// NewDB opens the database, applies pool settings, and verifies connectivity.
func NewDB(cfg *Config) (*sql.DB, error) {
	db, err := sql.Open(cfg.DBDriver, cfg.DBDSN)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}

	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	db.SetConnMaxLifetime(cfg.DBConnMaxLifetime)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("db.Ping: %w", err)
	}
	return db, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
