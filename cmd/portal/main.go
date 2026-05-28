package main

import (
	"context"
	"log"
	"net/http"

	"overmind/internal/platform/app"
	platformconfig "overmind/internal/platform/config"
	"overmind/internal/platform/logging"
	"overmind/internal/portal/repository"
	"overmind/internal/portal/service"
)

func main() {
	cfg, err := platformconfig.Load(".")
	if err != nil {
		log.Fatal(err)
	}

	logging.Init(cfg.Log.Level, cfg.Log.Encoding)

	repo := repository.NewMemoryRepository()
	_ = service.New(repo)

	server := &http.Server{
		Addr: cfg.Services.Portal.Address(),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/healthz" {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("portal ok"))
		}),
	}

	logging.L().Info("portal service starting", logging.String("addr", server.Addr))
	if err := app.RunHTTP(context.Background(), server); err != nil {
		log.Fatal(err)
	}
}
