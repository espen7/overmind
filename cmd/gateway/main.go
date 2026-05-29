package main

import (
	"context"
	"log"

	gatewayapp "overmind/internal/gateway/app"
	"overmind/internal/platform/app"
	platformconfig "overmind/internal/platform/config"
	"overmind/internal/platform/logging"
)

func main() {
	cfg, err := platformconfig.Load(".")
	if err != nil {
		log.Fatal(err)
	}

	logging.Init(cfg.Services.Gateway.Name, cfg.Log.Level, cfg.Log.Encoding)

	if err := app.RunServer(context.Background(), gatewayapp.New(cfg)); err != nil {
		log.Fatal(err)
	}
}
