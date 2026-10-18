package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/0xFA7E/aniimo-weather-bot/internal/app"
	"github.com/0xFA7E/aniimo-weather-bot/internal/gui"
)

func main() {
	configPath := flag.String("config", "", "path to config.json (default: ./config.json when present, otherwise platform config directory)")
	once := flag.Bool("once", false, "scan existing screenshots once and exit")
	headless := flag.Bool("headless", false, "run without the desktop interface")
	flag.Parse()

	cfg, path, err := app.LoadConfig(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := app.EnsureConfig(path, cfg); err != nil {
		log.Fatal(err)
	}
	if !*headless && !*once {
		if err := gui.Run(cfg, path); err != nil {
			log.Fatal(err)
		}
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := app.Run(ctx, cfg, *once); err != nil {
		log.Fatal(err)
	}
	fmt.Println("weather-bot stopped")
}
