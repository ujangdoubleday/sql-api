package config

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/denisenkom/go-mssqldb"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

// DatabaseConfig defines one driver's connection and pool settings.
type DatabaseConfig struct {
	Driver          string
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// Config holds the database registry and shared application settings.
type Config struct {
	Databases           map[string]DatabaseConfig
	DefaultDatabase     string
	NamedDatabases      bool
	ServerPort          string
	QueryTimeoutSeconds int
}

// Load validates named database settings or the legacy single-database settings.
func Load() (*Config, error) {
	cfg := &Config{Databases: make(map[string]DatabaseConfig), ServerPort: getEnv("SERVER_PORT", "8080")}
	var err error
	cfg.QueryTimeoutSeconds, err = envInt("QUERY_TIMEOUT_SECONDS", 10)
	if err != nil {
		return nil, err
	}
	if cfg.QueryTimeoutSeconds <= 0 || int64(cfg.QueryTimeoutSeconds) > int64((1<<63-1)/time.Second) {
		return nil, fmt.Errorf("QUERY_TIMEOUT_SECONDS must be a positive duration")
	}
	defaults, err := poolConfig("DB_", DatabaseConfig{MaxOpenConns: 25, MaxIdleConns: 5, ConnMaxLifetime: 5 * time.Minute})
	if err != nil {
		return nil, err
	}
	names, named := os.LookupEnv("DATABASES")
	cfg.NamedDatabases = named
	if !named {
		defaults.Driver = getEnv("DB_DRIVER", "mysql")
		defaults.DSN = os.Getenv("DB_DSN")
		if err := validateDatabase("default", defaults); err != nil {
			return nil, err
		}
		cfg.Databases["default"] = defaults
		cfg.DefaultDatabase = "default"
		return cfg, nil
	}
	validAlias := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	for _, name := range strings.Split(names, ",") {
		name = strings.TrimSpace(name)
		if !validAlias.MatchString(name) {
			return nil, fmt.Errorf("DATABASES contains an invalid alias")
		}
		if _, exists := cfg.Databases[name]; exists {
			return nil, fmt.Errorf("duplicate database alias %q", name)
		}
		prefix := "DB_" + strings.ToUpper(name) + "_"
		db, err := poolConfig(prefix, defaults)
		if err != nil {
			return nil, err
		}
		db.Driver = os.Getenv(prefix + "DRIVER")
		db.DSN = os.Getenv(prefix + "DSN")
		if err := validateDatabase(name, db); err != nil {
			return nil, err
		}
		cfg.Databases[name] = db
	}
	cfg.DefaultDatabase = os.Getenv("DEFAULT_DATABASE")
	if _, ok := cfg.Databases[cfg.DefaultDatabase]; !ok {
		return nil, fmt.Errorf("DEFAULT_DATABASE must name a configured database")
	}
	return cfg, nil
}

func poolConfig(prefix string, defaults DatabaseConfig) (DatabaseConfig, error) {
	var err error
	defaults.MaxOpenConns, err = envInt(prefix+"MAX_OPEN_CONNS", defaults.MaxOpenConns)
	if err != nil {
		return defaults, err
	}
	defaults.MaxIdleConns, err = envInt(prefix+"MAX_IDLE_CONNS", defaults.MaxIdleConns)
	if err != nil {
		return defaults, err
	}
	minutes, err := envInt(prefix+"CONN_MAX_LIFETIME_MINUTES", int(defaults.ConnMaxLifetime/time.Minute))
	if err != nil {
		return defaults, err
	}
	if defaults.MaxOpenConns <= 0 || defaults.MaxIdleConns < 0 || defaults.MaxIdleConns > defaults.MaxOpenConns || minutes < 0 || int64(minutes) > int64((1<<63-1)/time.Minute) {
		return defaults, fmt.Errorf("invalid %spool settings: require open > 0, 0 <= idle <= open, and a nonnegative lifetime", prefix)
	}
	defaults.ConnMaxLifetime = time.Duration(minutes) * time.Minute
	return defaults, nil
}

func validateDatabase(name string, cfg DatabaseConfig) error {
	switch cfg.Driver {
	case "mysql", "postgres", "sqlserver":
	default:
		return fmt.Errorf("database %q has an unsupported or missing driver", name)
	}
	if strings.TrimSpace(cfg.DSN) == "" {
		return fmt.Errorf("database %q requires a DSN", name)
	}
	return nil
}

// NewDB returns a verified pool. Driver errors are withheld because they can contain credentials.
func NewDB(ctx context.Context, cfg DatabaseConfig) (*sql.DB, error) {
	db, err := sql.Open(cfg.Driver, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("cannot open database; check driver and DSN")
	}
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("database connectivity check failed; check connection settings and availability")
	}
	return db, nil
}

// OpenDatabases owns startup cleanup; the caller owns the returned pools.
func OpenDatabases(ctx context.Context, cfg *Config) (map[string]*sql.DB, error) {
	pools := make(map[string]*sql.DB)
	names := make([]string, 0, len(cfg.Databases))
	for name := range cfg.Databases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		pingCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.QueryTimeoutSeconds)*time.Second)
		db, err := NewDB(pingCtx, cfg.Databases[name])
		cancel()
		if err != nil {
			CloseDatabases(pools)
			return nil, fmt.Errorf("database %q: %w", name, err)
		}
		pools[name] = db
	}
	return pools, nil
}

// CloseDatabases releases every pool owned by the caller.
func CloseDatabases(pools map[string]*sql.DB) {
	for _, db := range pools {
		_ = db.Close()
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	v, exists := os.LookupEnv(key)
	if !exists {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	return n, nil
}
