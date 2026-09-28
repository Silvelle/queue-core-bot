package config

import (
	"errors"
	"os"
)

type Config struct {
	Token string
}

func Load() (*Config, error) {
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		return nil, errors.New("BOT_TOKEN environment variable not set")
	}

	return &Config{token}, nil
}
