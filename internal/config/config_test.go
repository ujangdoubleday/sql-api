package config

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func cleanEnv(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "DB_") || key == "DATABASES" || key == "DEFAULT_DATABASE" || key == "QUERY_TIMEOUT_SECONDS" {
			t.Setenv(key, "")
			os.Unsetenv(key)
		}
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		invalid bool
	}{
		{"legacy", map[string]string{"DB_DSN": "legacy"}, false},
		{"named", map[string]string{"DATABASES": "main, reporting", "DEFAULT_DATABASE": "main", "DB_MAIN_DRIVER": "mysql", "DB_MAIN_DSN": "main", "DB_REPORTING_DRIVER": "postgres", "DB_REPORTING_DSN": "reporting", "DB_REPORTING_MAX_OPEN_CONNS": "10", "DB_REPORTING_MAX_IDLE_CONNS": "2", "DB_REPORTING_CONN_MAX_LIFETIME_MINUTES": "0", "DB_DRIVER": "invalid", "DB_DSN": "ignored"}, false},
		{"empty list", map[string]string{"DATABASES": ""}, true},
		{"duplicate", map[string]string{"DATABASES": "main,main", "DEFAULT_DATABASE": "main", "DB_MAIN_DRIVER": "mysql", "DB_MAIN_DSN": "main"}, true},
		{"bad alias", map[string]string{"DATABASES": "Main"}, true},
		{"missing default", map[string]string{"DATABASES": "main", "DB_MAIN_DRIVER": "mysql", "DB_MAIN_DSN": "main"}, true},
		{"unknown default", map[string]string{"DATABASES": "main", "DEFAULT_DATABASE": "other", "DB_MAIN_DRIVER": "mysql", "DB_MAIN_DSN": "main"}, true},
		{"missing driver", map[string]string{"DATABASES": "main", "DEFAULT_DATABASE": "main", "DB_MAIN_DSN": "main"}, true},
		{"missing dsn", map[string]string{"DB_DRIVER": "mysql"}, true},
		{"unsupported driver", map[string]string{"DB_DRIVER": "other", "DB_DSN": "x"}, true},
		{"invalid integer", map[string]string{"DB_DSN": "x", "DB_MAX_OPEN_CONNS": "oops"}, true},
		{"empty integer", map[string]string{"DB_DSN": "x", "DB_MAX_OPEN_CONNS": ""}, true},
		{"zero open", map[string]string{"DB_DSN": "x", "DB_MAX_OPEN_CONNS": "0"}, true},
		{"excess idle", map[string]string{"DB_DSN": "x", "DB_MAX_IDLE_CONNS": "26"}, true},
		{"negative lifetime", map[string]string{"DB_DSN": "x", "DB_CONN_MAX_LIFETIME_MINUTES": "-1"}, true},
		{"zero timeout", map[string]string{"DB_DSN": "x", "QUERY_TIMEOUT_SECONDS": "0"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			cfg, err := Load()
			if (err != nil) != tt.invalid {
				t.Fatalf("Load error = %v, invalid = %v", err, tt.invalid)
			}
			if err != nil {
				return
			}
			if tt.name == "legacy" {
				if cfg.NamedDatabases || cfg.DefaultDatabase != "default" || cfg.Databases["default"].DSN != "legacy" || cfg.Databases["default"].MaxOpenConns != 25 {
					t.Fatalf("legacy config: %+v", cfg)
				}
			} else {
				db := cfg.Databases["reporting"]
				if !cfg.NamedDatabases || cfg.DefaultDatabase != "main" || len(cfg.Databases) != 2 || db.Driver != "postgres" || db.MaxOpenConns != 10 || db.MaxIdleConns != 2 || db.ConnMaxLifetime != 0 {
					t.Fatalf("named config: %+v", cfg)
				}
			}
		})
	}
}

type poolDriver struct{}
type poolConn struct{ fail bool }

var closed atomic.Int32

func (poolDriver) Open(dsn string) (driver.Conn, error) { return &poolConn{fail: dsn == "fail"}, nil }
func (c *poolConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unused") }
func (c *poolConn) Begin() (driver.Tx, error)           { return nil, errors.New("unused") }
func (c *poolConn) Close() error                        { closed.Add(1); return nil }
func (c *poolConn) Ping(ctx context.Context) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("missing deadline")
	}
	if c.fail {
		return errors.New("secret connection details")
	}
	return nil
}
func init() { sql.Register("config_test", poolDriver{}) }

func TestPoolLifecycle(t *testing.T) {
	for _, fail := range []bool{false, true} {
		closed.Store(0)
		cfg := &Config{QueryTimeoutSeconds: 1, Databases: map[string]DatabaseConfig{
			"a": {Driver: "config_test", DSN: "ok", MaxOpenConns: 2, MaxIdleConns: 1},
			"b": {Driver: "config_test", DSN: "ok", MaxOpenConns: 2, MaxIdleConns: 1},
		}}
		if fail {
			db := cfg.Databases["b"]
			db.DSN = "fail"
			cfg.Databases["b"] = db
		}
		pools, err := OpenDatabases(context.Background(), cfg)
		if fail {
			if err == nil || !strings.Contains(err.Error(), `database "b"`) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("unsafe or missing error: %v", err)
			}
		} else {
			if err != nil || len(pools) != 2 {
				t.Fatalf("open: %v", err)
			}
			if pools["a"].Stats().MaxOpenConnections != 2 {
				t.Fatal("pool settings not applied")
			}
			CloseDatabases(pools)
			if pools["a"].Ping() == nil {
				t.Fatal("pool remains open")
			}
		}
		if closed.Load() != 2 {
			t.Fatalf("closed %d connections, want 2", closed.Load())
		}
	}
}
