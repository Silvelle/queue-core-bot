package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-telegram/bot"

	"github.com/Silvelle/queue-core-bot/internal/config"
	"github.com/Silvelle/queue-core-bot/internal/handler"
	"github.com/Silvelle/queue-core-bot/internal/service"
	"github.com/Silvelle/queue-core-bot/internal/storage/sqlite"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run starts the bot and blocks until it is stopped. Returning an error
// instead of exiting lets every deferred cleanup run, such as closing the
// database.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Local setup comes before contacting Telegram, so a bad DB_PATH fails
	// at once with a clear error.
	store, err := sqlite.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("close storage: %v", err)
		}
	}()

	b, err := bot.New(cfg.Token)
	if err != nil {
		return err
	}
	me, err := b.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("get bot info: %w", err)
	}

	svc := service.New(store)
	handler.New(ctx, b, svc, me.Username).Register(ctx)

	log.Printf("bot @%s started, storage %s", me.Username, cfg.DBPath)
	b.Start(ctx)
	log.Println("bot stopped")
	return nil
}
