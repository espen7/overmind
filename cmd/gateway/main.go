package main

import (
	"context"
	"log"

	gamerepo "overmind/internal/game/repository"
	gameservice "overmind/internal/game/service"
	gametransport "overmind/internal/game/transport"
	gatewaynet "overmind/internal/gateway/net"
	"overmind/internal/platform/app"
	platformconfig "overmind/internal/platform/config"
	"overmind/internal/platform/logging"
	portalrepo "overmind/internal/portal/repository"
	portalservice "overmind/internal/portal/service"
	portaltransport "overmind/internal/portal/transport"
)

func main() {
	cfg, err := platformconfig.Load(".")
	if err != nil {
		log.Fatal(err)
	}

	logging.Init(cfg.Log.Level, cfg.Log.Encoding)

	portalRepository := portalrepo.NewMemoryRepository()
	portalHandler := portaltransport.NewHandler(portalservice.New(portalRepository))

	world := gamerepo.NewMemoryWorld()
	gameHandler := gametransport.NewHandler(
		gameservice.NewScene(world),
		gameservice.NewCombat(world),
		world,
	)

	server := gatewaynet.NewWSServer(cfg.Services.Gateway.Address(), portalHandler, gameHandler)
	logging.L().Info("gateway service starting", logging.String("addr", cfg.Services.Gateway.Address()))
	if err := app.RunServer(context.Background(), server); err != nil {
		log.Fatal(err)
	}
}
