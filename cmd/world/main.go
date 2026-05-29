package main

import (
	"context"
	"log"

	"overmind/internal/platform/app"
	platformconfig "overmind/internal/platform/config"
	"overmind/internal/platform/logging"
	worldapp "overmind/internal/world/app"
)

func main() {
	cfg, err := platformconfig.Load(".")
	if err != nil {
		log.Fatal(err)
	}

	logging.Init(cfg.Services.World.Name, cfg.Log.Level, cfg.Log.Encoding)

	if err := app.RunServer(context.Background(), worldapp.New(cfg)); err != nil {
		log.Fatal(err)
	}
}
