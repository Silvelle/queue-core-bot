package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("BOT_TOKEN", "123:abc")
	t.Setenv("DB_PATH", "/var/lib/bot/queue.db")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "123:abc" || cfg.DBPath != "/var/lib/bot/queue.db" {
		t.Errorf("Load() = %+v", cfg)
	}
}

func TestLoadDefaultDBPath(t *testing.T) {
	t.Setenv("BOT_TOKEN", "123:abc")
	t.Setenv("DB_PATH", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBPath != defaultDBPath {
		t.Errorf("DBPath = %q, want the default %q", cfg.DBPath, defaultDBPath)
	}
}

func TestLoadRequiresToken(t *testing.T) {
	t.Setenv("BOT_TOKEN", "")

	if _, err := Load(); err == nil {
		t.Error("Load() without BOT_TOKEN should fail")
	}
}
