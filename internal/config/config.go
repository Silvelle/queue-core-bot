package config

import (
	"errors"
	"os"
)

// defaultDBPath is inside data/, which .gitignore already excludes.
const defaultDBPath = "data/queue.db"

type Config struct {
	Token string
	// DBPath is the SQLite file where queues are kept.
	DBPath string
}

func Load() (*Config, error) {
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		return nil, errors.New("BOT_TOKEN environment variable not set")
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = defaultDBPath
	}

	return &Config{Token: token, DBPath: dbPath}, nil
}
