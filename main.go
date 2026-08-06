package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"github.com/nori-kamiya/xordpress/internal/app"
	"github.com/nori-kamiya/xordpress/internal/config"
)

func main() {
	cfg := config.New()

	flag.StringVar(&cfg.Addr, "addr", cfg.Addr, "address to listen on")
	flag.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "data directory")
	flag.StringVar(&cfg.BaseURL, "base-url", cfg.BaseURL, "base URL of the server")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := app.Run(ctx, *cfg); err != nil {
		log.Fatal(err)
	}
}
