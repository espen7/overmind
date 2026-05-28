package main

import (
	"context"
	"log"
	"net/http"

	"overmind/internal/game/repository"
	"overmind/internal/game/service"
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

	world := repository.NewMemoryWorld()
	_ = service.NewScene(world)
	_ = service.NewCombat(world)

	server := &http.Server{
		Addr: cfg.Services.Game.Address(),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/healthz" {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("game ok"))
		}),
	}

	logging.L().Info("game service starting", logging.String("addr", server.Addr))
	if err := app.RunHTTP(context.Background(), server); err != nil {
		log.Fatal(err)
	}
}
