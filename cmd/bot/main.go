package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Silvelle/queue-core-bot/internal/config"
	"github.com/Silvelle/queue-core-bot/internal/handler"
	"github.com/Silvelle/queue-core-bot/internal/service"
	"github.com/Silvelle/queue-core-bot/internal/storage/memory"
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

	me, err := b.GetMe(ctx)
	if err != nil {
		log.Fatalf("get bot info: %v", err)
	}

	svc := service.New(memory.New())
	handler.New(ctx, b, svc, me.Username).Register(ctx)

	log.Printf("bot @%s started", me.Username)
	b.Start(ctx)
	log.Println("bot stopped")
}
