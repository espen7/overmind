package main

import (
	"context"
	"log"
	"net/http"

	clusterruntime "overmind/internal/cluster/runtime"
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

	runtime := clusterruntime.New(cfg.Actor)
	server := &http.Server{
		Addr: cfg.Services.Player.Address(),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/healthz" {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("player ok:" + runtime.Address()))
		}),
	}

	logging.L().Info("player service starting", logging.String("addr", server.Addr))
	if err := app.RunHTTP(context.Background(), server); err != nil {
		log.Fatal(err)
	}
}
