package main

import (
	"context"
	"log"

	gatewaynet "overmind/internal/gateway/net"
	"overmind/internal/platform/app"
	platformconfig "overmind/internal/platform/config"
	"overmind/internal/platform/logging"
	portalrepo "overmind/internal/portal/repository"
	portalservice "overmind/internal/portal/service"
	portaltransport "overmind/internal/portal/transport"
	worldrepo "overmind/internal/world/repository"
	worldservice "overmind/internal/world/service"
	worldtransport "overmind/internal/world/transport"
)

func main() {
	cfg, err := platformconfig.Load(".")
	if err != nil {
		log.Fatal(err)
	}

	logging.Init(cfg.Services.Gateway.Name, cfg.Log.Level, cfg.Log.Encoding)

	portalRepository := portalrepo.NewMemoryRepository()
	portalHandler := portaltransport.NewHandler(portalservice.New(portalRepository))

	// Gateway keeps the transport edge and delegates portal/world logic through handlers.
	world := worldrepo.NewMemoryWorld()
	worldHandler := worldtransport.NewHandler(
		worldservice.NewScene(world),
		worldservice.NewCombat(world),
		world,
	)

	server := gatewaynet.NewWSServer(cfg.Services.Gateway.Address(), portalHandler, worldHandler)
	logging.L().Info("gateway service starting", logging.String("addr", cfg.Services.Gateway.Address()))
	if err := app.RunServer(context.Background(), server); err != nil {
		log.Fatal(err)
	}
}
