package main

import (
	"context"
	"log"
	"net/http"

	"overmind/internal/platform/app"
	platformconfig "overmind/internal/platform/config"
	"overmind/internal/platform/logging"
	"overmind/internal/world/repository"
	"overmind/internal/world/service"
)

func main() {
	cfg, err := platformconfig.Load(".")
	if err != nil {
		log.Fatal(err)
	}

	logging.Init(cfg.Log.Level, cfg.Log.Encoding)

	// The world service owns scene state, AOI membership, and combat simulation.
	world := repository.NewMemoryWorld()
	_ = service.NewScene(world)
	_ = service.NewCombat(world)

	server := &http.Server{
		Addr: cfg.Services.World.Address(),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/healthz" {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("world ok"))
		}),
	}

	logging.L().Info("world service starting", logging.String("addr", server.Addr))
	if err := app.RunHTTP(context.Background(), server); err != nil {
		log.Fatal(err)
	}
}
