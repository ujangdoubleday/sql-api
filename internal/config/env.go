package config

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

func LoadEnv(explicit string) {
	if explicit != "" {
		_ = godotenv.Load(explicit)
		return
	}

	if err := godotenv.Load(); err == nil {
		return
	}

	home, err := os.UserHomeDir()
	if err == nil {
		_ = godotenv.Load(filepath.Join(home, ".config", "sql-api", ".env"))
	}
}
