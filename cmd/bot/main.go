package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Silvelle/queue-core-bot/internal/config"
	"github.com/Silvelle/queue-core-bot/internal/handler"
	"github.com/go-telegram/bot"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	b, err := bot.New(cfg.Token)
	if err != nil {
		log.Fatal(err)
	}

	handler.New().Register(b)

	log.Println("bot started")
	b.Start(ctx)
	log.Println("bot stopped")
}
