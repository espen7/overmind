package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/asynkron/protoactor-go/cluster"

	clusterruntime "overmind/internal/cluster/runtime"
	"overmind/internal/platform/app"
	platformconfig "overmind/internal/platform/config"
	"overmind/internal/platform/logging"
	platformmongo "overmind/internal/platform/mongo"
	playeractor "overmind/internal/player/actor"
	playerrepo "overmind/internal/player/repository"
	playerservice "overmind/internal/player/service"
)

func main() {
	cfg, err := platformconfig.Load(".")
	if err != nil {
		log.Fatal(err)
	}

	logging.Init(cfg.Services.Player.Name, cfg.Log.Level, cfg.Log.Encoding)

	ctx := context.Background()
	runtime := clusterruntime.New(cfg.Actor)
	mongoClient, database, err := platformmongo.Connect(ctx, cfg.Mongo)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = mongoClient.Disconnect(shutdownCtx)
	}()

	playerKind := cluster.NewKind(
		clusterruntime.PlayerKind,
		playeractor.ClusterProps(
			playerservice.NewStaticLoginService(),
			playeractor.WithManagerFactory(
				playeractor.NewMongoManagerFactory(
					playerrepo.NewMongoRepository(database),
				),
			),
		),
	)
	virtualCluster := clusterruntime.StartLocalVirtualCluster(runtime.System(), cfg.Actor, playerKind)
	defer virtualCluster.Shutdown(true)

	server := &http.Server{
		Addr: cfg.Services.Player.Address(),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/healthz" {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("player ok:" + runtime.Address() + " db:" + database.Name()))
		}),
	}

	logging.L().Info("player service starting", logging.String("addr", server.Addr))
	if err := app.RunHTTP(context.Background(), server); err != nil {
		log.Fatal(err)
	}
}
