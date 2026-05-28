package main

import (
	"context"
	"log"

	authrepo "overmind/internal/auth/repository"
	authservice "overmind/internal/auth/service"
	authtransport "overmind/internal/auth/transport"
	gamerepo "overmind/internal/game/repository"
	gameservice "overmind/internal/game/service"
	gametransport "overmind/internal/game/transport"
	gatewaynet "overmind/internal/gateway/net"
	"overmind/internal/platform/app"
	platformconfig "overmind/internal/platform/config"
	"overmind/internal/platform/logging"
)

func main() {
	cfg, err := platformconfig.Load(".")
	if err != nil {
		log.Fatal(err)
	}

	logging.Init(cfg.Log.Level, cfg.Log.Encoding)

	authRepository := authrepo.NewMemoryRepository()
	authHandler := authtransport.NewHandler(authservice.New(authRepository))

	world := gamerepo.NewMemoryWorld()
	gameHandler := gametransport.NewHandler(
		gameservice.NewScene(world),
		gameservice.NewCombat(world),
		world,
	)

	server := gatewaynet.NewWSServer(cfg.Services.Gateway.Address(), authHandler, gameHandler)
	logging.L().Info("gateway service starting", logging.String("addr", cfg.Services.Gateway.Address()))
	if err := app.RunServer(context.Background(), server); err != nil {
		log.Fatal(err)
	}
}
